package medcare

import (
    "encoding/json"
    "fmt"
    "net/http"
    "sync"
    "time"

    "smallgo/server/response"

    "github.com/gin-gonic/gin"
    "gorm.io/gorm"
)

type realtimeEvent struct {
    Type         string             `json:"type"`
    Scope        string             `json:"scope,omitempty"`
    Action       string             `json:"action,omitempty"`
    TargetID     uint               `json:"target_id,omitempty"`
    BookID       uint               `json:"book_id,omitempty"`
    Revision     uint64             `json:"revision,omitempty"`
    Notification *InAppNotification `json:"notification,omitempty"`
    SentAt       time.Time          `json:"sent_at"`
}

type realtimeClient struct {
    events chan realtimeEvent
}

type realtimeBroker struct {
    mu      sync.RWMutex
    clients map[uint]map[*realtimeClient]struct{}
}

var (
    medRealtime = realtimeBroker{clients: map[uint]map[*realtimeClient]struct{}{}}
    realtimeDB  *gorm.DB
    revisionMu  sync.Mutex
    revisions   = map[uint]uint64{}
)

func (b *realtimeBroker) subscribe(userID uint) (<-chan realtimeEvent, func()) {
    c := &realtimeClient{events: make(chan realtimeEvent, 8)}
    b.mu.Lock()
    if b.clients[userID] == nil {
        b.clients[userID] = map[*realtimeClient]struct{}{}
    }
    b.clients[userID][c] = struct{}{}
    b.mu.Unlock()
    return c.events, func() {
        b.mu.Lock()
        delete(b.clients[userID], c)
        if len(b.clients[userID]) == 0 {
            delete(b.clients, userID)
        }
        b.mu.Unlock()
    }
}

func (b *realtimeBroker) publish(userID uint, event realtimeEvent) {
    if event.SentAt.IsZero() {
        event.SentAt = nowUTC()
    }
    b.mu.RLock()
    defer b.mu.RUnlock()
    for c := range b.clients[userID] {
        select {
        case c.events <- event:
        default:
            select {
            case <-c.events:
            default:
            }
            select {
            case c.events <- event:
            default:
            }
        }
    }
}

func bumpRevision(userID uint) uint64 {
    revisionMu.Lock()
    defer revisionMu.Unlock()
    revisions[userID]++
    return revisions[userID]
}

func publishEventChanged(bookID uint, scope, action string, targetID uint, origin string) {
    if realtimeDB == nil {
        return
    }
    var members []BookMember
    realtimeDB.Where("book_id = ?", bookID).Find(&members)
    for _, m := range members {
        rev := bumpRevision(m.UserID)
        medRealtime.publish(m.UserID, realtimeEvent{
            Type: "data.changed", Scope: scope, Action: action, TargetID: targetID,
            BookID: bookID, Revision: rev,
        })
    }
}

func publishNotification(n InAppNotification) {
    rev := bumpRevision(n.UserID)
    medRealtime.publish(n.UserID, realtimeEvent{
        Type: "notification.created", BookID: n.BookID, Revision: rev, Notification: &n,
    })
}

func handleRevision(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        revisionMu.Lock()
        rev := revisions[currentUserID(c)]
        revisionMu.Unlock()
        response.Success(c, gin.H{"revision": rev})
    }
}

func handleRealtimeStream(c *gin.Context) {
    flusher, ok := c.Writer.(http.Flusher)
    if !ok {
        c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"message": "当前连接不支持实时推送"})
        return
    }
    c.Header("Content-Type", "text/event-stream; charset=utf-8")
    c.Header("Cache-Control", "no-cache, no-transform")
    c.Header("Connection", "keep-alive")
    c.Header("X-Accel-Buffering", "no")
    c.Status(http.StatusOK)

    events, unsubscribe := medRealtime.subscribe(currentUserID(c))
    defer unsubscribe()
    if err := writeSSE(c.Writer, realtimeEvent{Type: "connected", SentAt: nowUTC()}); err != nil {
        return
    }
    flusher.Flush()
    ticker := time.NewTicker(20 * time.Second)
    defer ticker.Stop()
    for {
        select {
        case <-c.Request.Context().Done():
            return
        case ev := <-events:
            if err := writeSSE(c.Writer, ev); err != nil {
                return
            }
            flusher.Flush()
        case <-ticker.C:
            if _, err := fmt.Fprint(c.Writer, ": heartbeat\n\n"); err != nil {
                return
            }
            flusher.Flush()
        }
    }
}

func writeSSE(w http.ResponseWriter, event realtimeEvent) error {
    data, err := json.Marshal(event)
    if err != nil {
        return err
    }
    _, err = fmt.Fprintf(w, "event: message\ndata: %s\n\n", data)
    return err
}
