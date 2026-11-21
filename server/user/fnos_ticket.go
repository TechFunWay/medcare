package user

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"smallgo/server/response"
)

// fnOSLoginTicket carries a gateway-verified NAS identity to the plain TCP
// listener, where the gateway identity headers are absent. The fnOS desktop
// entry is served from the gateway origin, which intercepts the application's
// own Authorization credentials, so authenticated API calls fail there; the SPA
// must therefore finish login on the direct listener. Tickets are short lived
// and single use so a leaked value cannot be replayed.
const fnOSTicketTTL = 2 * time.Minute

type fnOSLoginTicket struct {
	Identity  FnOSIdentity
	ExpiresAt time.Time
}

var (
	fnOSTicketsMu sync.Mutex
	fnOSTickets   = make(map[string]fnOSLoginTicket)
)

// storeFnOSTicket mints a ticket for one gateway-verified identity, dropping
// expired entries along the way.
func storeFnOSTicket(identity FnOSIdentity) (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	ticket := hex.EncodeToString(raw)
	now := time.Now()
	fnOSTicketsMu.Lock()
	for key, item := range fnOSTickets {
		if !item.ExpiresAt.After(now) {
			delete(fnOSTickets, key)
		}
	}
	fnOSTickets[ticket] = fnOSLoginTicket{Identity: identity, ExpiresAt: now.Add(fnOSTicketTTL)}
	fnOSTicketsMu.Unlock()
	return ticket, nil
}

// lookupFnOSTicket resolves a ticket without consuming it: the binding flow
// first calls login (which may report binding_required) and then bind with the
// same ticket, so only the request that finishes authentication consumes it.
func lookupFnOSTicket(ticket string) (FnOSIdentity, bool) {
	fnOSTicketsMu.Lock()
	defer fnOSTicketsMu.Unlock()
	item, ok := fnOSTickets[ticket]
	if !ok {
		return FnOSIdentity{}, false
	}
	if !item.ExpiresAt.After(time.Now()) {
		delete(fnOSTickets, ticket)
		return FnOSIdentity{}, false
	}
	return item.Identity, true
}

func consumeFnOSTicket(ticket string) {
	if ticket == "" {
		return
	}
	fnOSTicketsMu.Lock()
	delete(fnOSTickets, ticket)
	fnOSTicketsMu.Unlock()
}

// handleFnOSTicket mints a one-time login ticket. It is only reachable from the
// gateway Unix socket, where fnOS has already verified the browser session and
// the injected identity headers cannot be forged by TCP clients.
func handleFnOSTicket() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Context().Value(fnOSGatewayContextKey{}) != true {
			response.ErrorUnauthorized(c, "请从飞牛桌面中的应用入口使用一键登录")
			return
		}
		identity, ok := fnOSIdentity(c)
		if !ok {
			return
		}
		ticket, err := storeFnOSTicket(identity)
		if err != nil {
			response.ErrorInternal(c, "生成飞牛登录凭证失败")
			return
		}
		response.Success(c, map[string]interface{}{
			"ticket":        ticket,
			"fnos_username": identity.Username,
		})
	}
}
