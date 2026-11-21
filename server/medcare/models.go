package medcare

import "time"

const (
    // 权限层级：owner（管理员，创建人，可转交）/ manager（监护人）/ member（成员）。
    // 「用药人」是成员身上的独立标记（IsCareRecipient），与权限层级无关：
    // 用药人可以是管理员，也可以是成员。
    RoleOwner   = "owner"
    RoleManager = "manager"
    RoleMember  = "member"

    BookActive   = "active"
    BookArchived = "archived"

    CatOral      = "oral"
    CatInjection = "injection"
    CatInfusion  = "infusion"
    CatTopical   = "topical"
    CatOther     = "other"

    PlanActive = "active"
    PlanPaused = "paused"
    PlanEnded  = "ended"
    // PlanCompleted 疗程内所有事件都已完成/跳过/漏服后自动置为已完成。
    PlanCompleted = "completed"
    PlanArchived  = "archived"

    FreqDaily         = "daily"
    FreqEveryNDays    = "every_n_days"
    FreqWeekly        = "weekly"
    FreqIntervalHours = "interval_hours"
    // FreqCustomDates 不规律日期：每天的时间点单独指定，适合今天一针、明天两针这种。
    FreqCustomDates = "custom_dates"

    EventPending = "pending"
    EventDone    = "done"
    EventSkipped = "skipped"
    EventMissed  = "missed"

    SkipDoctor = "doctor_adjusted"
    SkipSelf   = "self_adjusted"
    SkipUnwell = "unwell"
    SkipOther  = "other"

    AdjDoctor = "doctor"
    AdjSelf   = "self"

    ScopeEvent   = "event"
    ScopeFromNow = "from_now"
    ScopePause   = "pause"
    ScopeEnd     = "end"

    ChannelInApp    = "inapp"
    ChannelEmail    = "email"
    ChannelDingTalk = "dingtalk"
    ChannelFeishu   = "feishu"

    JobPending  = "pending"
    JobSending  = "sending"
    JobRetry    = "retry"
    JobSent     = "sent"
    JobFailed   = "failed"
    JobCanceled = "canceled"

    JobLead     = "lead"
    JobDue      = "due"
    JobOverdue1 = "overdue_1"
    JobOverdue2 = "overdue_2"
    // JobOverdueRepeatPrefix 是「没按时吃药就按频率反复提醒、直到点完成」的动态任务，
    // 完整 kind 形如 overdue_repeat_3（第 3 次重复）。
    JobOverdueRepeatPrefix = "overdue_repeat"
    // JobLowStock 是库存低于阈值 / 缺货的提醒（无用药事件）。
    JobLowStock = "low_stock"
    JobSummary  = "daily_summary"
    // JobRemind 由成员手动触发的「提醒用药」，立即发送给用药人与监护人。
    JobRemind = "remind"
    JobTest   = "test"

    PrescriptionRecognizing = "recognizing"
    PrescriptionRecognized  = "recognized"
    PrescriptionConfirming  = "confirming"
    PrescriptionConfirmed   = "confirmed"
    PrescriptionFailed      = "failed"
    PrescriptionDiscarded   = "discarded"

    // 识别单据类型：处方笺、药品包装盒、收费缴费单。
    // 药盒与缴费单识别完成后直接用于「建药品 / 批量入库」，不生成用药计划。
    RecognizeKindPrescription = "prescription"
    RecognizeKindBox          = "box"
    RecognizeKindReceipt      = "receipt"

    // 入库渠道：这批药从哪来（药店购买 / 医院开具 / 其他）。
    StockChannelPharmacy = "pharmacy"
    StockChannelHospital = "hospital"
    StockChannelOther    = "other"

    // 录入来源：这条出入库记录是哪种方式产生的。
    StockEntryManual       = "manual"
    StockEntryBox          = "box"
    StockEntryReceipt      = "receipt"
    StockEntryPrescription = "prescription"
    StockEntryCompletion   = "completion"
    StockEntryReturn       = "return"
    StockEntryAdjust       = "adjust"
    StockEntryWaste        = "waste"
    StockEntryStocktake    = "stocktake"
)

// MedicationBook 是用药本，所有业务数据的边界。
type MedicationBook struct {
    ID                uint       `gorm:"primarykey" json:"id"`
    Name              string     `gorm:"size:60;not null" json:"name"`
    CareRecipientName string     `gorm:"size:40;not null" json:"care_recipient_name"`
    Relation          string     `gorm:"size:20" json:"relation"`
    Color             string     `gorm:"size:20" json:"color"`
    Timezone          string     `gorm:"size:64;not null;default:Asia/Shanghai" json:"timezone"`
    Notes             string     `gorm:"type:text" json:"notes"`
    OwnerUserID       uint       `gorm:"not null;index" json:"owner_user_id"`
    Status            string     `gorm:"size:16;not null;default:active;index" json:"status"`
    CreatedBy         uint       `gorm:"not null" json:"created_by"`
    UpdatedBy         uint       `json:"updated_by"`
    CreatedAt         time.Time  `json:"created_at"`
    UpdatedAt         time.Time  `json:"updated_at"`
    ArchivedAt        *time.Time `json:"archived_at,omitempty"`
    DeletedAt         *time.Time `gorm:"index" json:"-"`
}

type BookMember struct {
    ID     uint   `gorm:"primarykey" json:"id"`
    BookID uint   `gorm:"not null;uniqueIndex:uniq_book_member;index" json:"book_id"`
    UserID uint   `gorm:"not null;uniqueIndex:uniq_book_member;index" json:"user_id"`
    Role   string `gorm:"size:16;not null" json:"role"`
    // IsCareRecipient 标记「用药人」：一个用药本有且只有一位。
    IsCareRecipient bool `gorm:"not null;default:false;index" json:"is_care_recipient"`
    // Permissions 是管理员为该成员单独勾选的能力（JSON 数组文本，见 permissions.go）。
    // 空串表示「跟随角色默认」，不限制；"[]" 表示显式一项都不给。
    Permissions          string    `gorm:"type:text" json:"permissions"`
    NotificationsEnabled bool      `gorm:"not null;default:true" json:"notifications_enabled"`
    JoinedAt             time.Time `json:"joined_at"`
    InvitedBy            *uint     `json:"invited_by,omitempty"`
    CreatedAt            time.Time `json:"created_at"`
    UpdatedAt            time.Time `json:"updated_at"`
}

type BookInvite struct {
    ID        uint   `gorm:"primarykey" json:"id"`
    BookID    uint   `gorm:"not null;index" json:"book_id"`
    TokenHash string `gorm:"size:64;not null;uniqueIndex" json:"-"`
    CodeHash  string `gorm:"size:64;not null;uniqueIndex" json:"-"`
    Role      string `gorm:"size:16;not null" json:"role"`
    MaxUses   int    `gorm:"not null;default:10" json:"max_uses"`
    // AsCareRecipient 表示受邀人加入后即为用药人。
    AsCareRecipient bool       `gorm:"not null;default:false" json:"as_care_recipient"`
    UsedCount       int        `gorm:"not null;default:0" json:"used_count"`
    ExpiresAt       time.Time  `gorm:"not null;index" json:"expires_at"`
    CreatedBy       uint       `gorm:"not null" json:"created_by"`
    RevokedAt       *time.Time `json:"revoked_at,omitempty"`
    CreatedAt       time.Time  `json:"created_at"`
}

type Medicine struct {
    ID                uint       `gorm:"primarykey" json:"id"`
    BookID            uint       `gorm:"not null;index:idx_med_book" json:"book_id"`
    Name              string     `gorm:"size:100;not null;index" json:"name"`
    GenericName       string     `gorm:"size:100" json:"generic_name"`
    Aliases           string     `gorm:"type:text" json:"aliases"`
    Category          string     `gorm:"size:20;not null;index" json:"category"`
    Form              string     `gorm:"size:30" json:"form"`
    Spec              string     `gorm:"size:100" json:"spec"`
    Unit              string     `gorm:"size:20;not null" json:"unit"`
    PackageSpec       string     `gorm:"size:100" json:"package_spec"`
    Manufacturer      string     `gorm:"size:100" json:"manufacturer"`
    Color             string     `gorm:"size:20" json:"color"`
    // CoverImageID 指向 book_images 里的一张图片，作为列表缩略图；0 表示未设置。
    CoverImageID      uint       `gorm:"not null;default:0" json:"cover_image_id"`
    Notes             string     `gorm:"type:text" json:"notes"`
    StockQuantity     float64    `gorm:"not null;default:0" json:"stock_quantity"`
    LowStockThreshold float64    `gorm:"not null;default:0" json:"low_stock_threshold"`
    // LowStockNotifyDisabled 为 true 时，该药品低于阈值或缺货都不发提醒；
    // 存「关闭」而非「开启」，是为了让默认值（false）在 GORM 插入零值时也自然生效。
    LowStockNotifyDisabled bool    `gorm:"not null;default:false" json:"low_stock_notify_disabled"`
    ExpiryWarnDays    int        `gorm:"not null;default:30" json:"expiry_warn_days"`
    // UnitPrice 药品参考单价（元 / 库存单位），人工维护或从收费单识别带入，仅作参考。
    UnitPrice          float64    `gorm:"not null;default:0" json:"unit_price"`
    ArchivedAt        *time.Time `gorm:"index" json:"archived_at,omitempty"`
    Version           uint       `gorm:"not null;default:1" json:"version"`
    CreatedBy         uint       `gorm:"not null" json:"created_by"`
    UpdatedBy         uint       `json:"updated_by"`
    CreatedAt         time.Time  `json:"created_at"`
    UpdatedAt         time.Time  `json:"updated_at"`
}

type MedicineBatch struct {
    ID                uint      `gorm:"primarykey" json:"id"`
    BookID            uint      `gorm:"not null;index" json:"book_id"`
    MedicineID        uint      `gorm:"not null;index" json:"medicine_id"`
    BatchNo           string    `gorm:"size:80" json:"batch_no"`
    QuantityInitial   float64   `gorm:"not null" json:"quantity_initial"`
    QuantityRemaining float64   `gorm:"not null" json:"quantity_remaining"`
    ExpiryDate        string    `gorm:"size:10;index" json:"expiry_date"`
    ReceivedAt        time.Time `json:"received_at"`
    Supplier          string    `gorm:"size:100" json:"supplier"`
    Note              string    `gorm:"type:text" json:"note"`
    CreatedBy         uint      `json:"created_by"`
    CreatedAt         time.Time `json:"created_at"`
    UpdatedAt         time.Time `json:"updated_at"`
}

type StockMovement struct {
    ID            uint    `gorm:"primarykey" json:"id"`
    BookID        uint    `gorm:"not null;index" json:"book_id"`
    MedicineID    uint    `gorm:"not null;index" json:"medicine_id"`
    BatchID       *uint   `gorm:"index" json:"batch_id,omitempty"`
    Type          string  `gorm:"size:16;not null" json:"type"`
    QuantityDelta float64 `gorm:"not null" json:"quantity_delta"`
    QuantityAfter float64 `gorm:"not null" json:"quantity_after"`
    EventID       *uint   `gorm:"index" json:"event_id,omitempty"`
    Reason        string  `gorm:"type:text" json:"reason"`
    // Channel 入库渠道（仅入库有值）：药店购买 / 医院开具 / 其他。
    Channel string `gorm:"size:20;index" json:"channel"`
    // EntrySource 录入来源：手动 / 药盒 / 缴费单 / 处方 / 完成用药 / 调整等。
    EntrySource string    `gorm:"size:20;index" json:"entry_source"`
    ActorUserID uint      `json:"actor_user_id"`
    CreatedAt   time.Time `gorm:"index" json:"created_at"`
}

type StockAllocation struct {
    ID         uint       `gorm:"primarykey" json:"id"`
    BookID     uint       `gorm:"not null;index" json:"book_id"`
    EventID    uint       `gorm:"not null;index" json:"event_id"`
    BatchID    uint       `gorm:"not null;index" json:"batch_id"`
    Quantity   float64    `gorm:"not null" json:"quantity"`
    MovementID uint       `gorm:"not null" json:"movement_id"`
    ReversedAt *time.Time `json:"reversed_at,omitempty"`
    CreatedAt  time.Time  `json:"created_at"`
}

type CareLocation struct {
    ID           uint       `gorm:"primarykey" json:"id"`
    BookID       uint       `gorm:"not null;index" json:"book_id"`
    Name         string     `gorm:"size:80;not null" json:"name"`
    Type         string     `gorm:"size:20;not null" json:"type"`
    Address      string     `gorm:"size:300" json:"address"`
    ContactName  string     `gorm:"size:60" json:"contact_name"`
    ContactPhone string     `gorm:"size:40" json:"contact_phone"`
    Latitude     *float64   `json:"latitude,omitempty"`
    Longitude    *float64   `json:"longitude,omitempty"`
    Notes        string     `gorm:"type:text" json:"notes"`
    ArchivedAt   *time.Time `gorm:"index" json:"archived_at,omitempty"`
    CreatedBy    uint       `json:"created_by"`
    UpdatedBy    uint       `json:"updated_by"`
    CreatedAt    time.Time  `json:"created_at"`
    UpdatedAt    time.Time  `json:"updated_at"`
}

type Plan struct {
    ID            uint    `gorm:"primarykey" json:"id"`
    BookID        uint    `gorm:"not null;index" json:"book_id"`
    MedicineID    uint    `gorm:"not null;index" json:"medicine_id"`
    LocationID    *uint   `gorm:"index" json:"location_id,omitempty"`
    LocationName  string  `gorm:"size:80" json:"location_name"`
    Address       string  `gorm:"size:300" json:"address"`
    Title         string  `gorm:"size:100" json:"title"`
    Type          string  `gorm:"size:20;not null" json:"type"`
    DoseAmount    float64 `gorm:"not null" json:"dose_amount"`
    DoseUnit      string  `gorm:"size:20;not null" json:"dose_unit"`
    FrequencyType string  `gorm:"size:20;not null" json:"frequency_type"`
    IntervalDays  int     `json:"interval_days"`
    Weekdays      string  `gorm:"type:text" json:"weekdays"`
    IntervalHours int     `json:"interval_hours"`
    Times         string  `gorm:"type:text" json:"times"`
    // CustomSchedule 是「指定日期」频率的 JSON：[{date:"2026-11-12",times:["08:00","20:00"]}]。
    CustomSchedule string     `gorm:"type:text" json:"custom_schedule"`
    AnchorDate     string     `gorm:"size:10" json:"anchor_date"`
    StartAt        *time.Time `json:"start_at,omitempty"`
    StartDate      string     `gorm:"size:10;not null" json:"start_date"`
    EndDate        string     `gorm:"size:10" json:"end_date"`
    Timezone       string     `gorm:"size:64;not null;default:Asia/Shanghai" json:"timezone"`
    Status         string     `gorm:"size:16;not null;default:active;index" json:"status"`
    Doctor         string     `gorm:"size:80" json:"doctor"`
    Hospital       string     `gorm:"size:120" json:"hospital"`
    // 打针/输液的执行人：谁给用药人打针或输液（自己/家人/护士/诊所医生）。
    ExecutorType string `gorm:"size:20" json:"executor_type"`
    ExecutorName string `gorm:"size:80" json:"executor_name"`
    // 输液参数：滴速（滴/分钟）与预计时长（分钟），仅输液计划使用。
    DripRate             int       `gorm:"not null;default:0" json:"drip_rate"`
    DurationMinutes      int       `gorm:"not null;default:0" json:"duration_minutes"`
    Notes                string    `gorm:"type:text" json:"notes"`
    ReminderLeadMinutes  *int      `json:"reminder_lead_minutes,omitempty"`
    NotificationsEnabled bool      `gorm:"not null;default:true" json:"notifications_enabled"`
    Version              uint      `gorm:"not null;default:1" json:"version"`
    CreatedBy            uint      `json:"created_by"`
    UpdatedBy            uint      `json:"updated_by"`
    CreatedAt            time.Time `json:"created_at"`
    UpdatedAt            time.Time `json:"updated_at"`
}

type PlanAdjustment struct {
    ID             uint      `gorm:"primarykey" json:"id"`
    BookID         uint      `gorm:"not null;index" json:"book_id"`
    PlanID         uint      `gorm:"not null;index" json:"plan_id"`
    EventID        *uint     `gorm:"index" json:"event_id,omitempty"`
    ActorUserID    uint      `gorm:"not null" json:"actor_user_id"`
    AdjustmentType string    `gorm:"size:16;not null" json:"adjustment_type"`
    Scope          string    `gorm:"size:16;not null" json:"scope"`
    BeforeJSON     string    `gorm:"type:text" json:"before_json"`
    AfterJSON      string    `gorm:"type:text" json:"after_json"`
    Note           string    `gorm:"type:text" json:"note"`
    DoctorName     string    `gorm:"size:80" json:"doctor_name"`
    Hospital       string    `gorm:"size:120" json:"hospital"`
    EffectiveAt    time.Time `json:"effective_at"`
    CreatedAt      time.Time `json:"created_at"`
}

type DoseEvent struct {
    ID           uint    `gorm:"primarykey" json:"id"`
    BookID       uint    `gorm:"not null;index" json:"book_id"`
    PlanID       uint    `gorm:"not null;index;uniqueIndex:uniq_event_plan_time" json:"plan_id"`
    MedicineID   uint    `gorm:"not null;index" json:"medicine_id"`
    MedicineName string  `gorm:"size:100;not null" json:"medicine_name"`
    PlanTitle    string  `gorm:"size:100" json:"plan_title"`
    Type         string  `gorm:"size:20;not null" json:"type"`
    DoseAmount   float64 `gorm:"not null" json:"dose_amount"`
    DoseUnit     string  `gorm:"size:20;not null" json:"dose_unit"`
    LocationID   *uint   `gorm:"index" json:"location_id,omitempty"`
    LocationName string  `gorm:"size:80" json:"location_name"`
    Address      string  `gorm:"size:300" json:"address"`
    // 打针/输液执行人快照，避免计划修改影响历史记录。
    ExecutorType    string `gorm:"size:20" json:"executor_type"`
    ExecutorName    string `gorm:"size:80" json:"executor_name"`
    DripRate        int    `gorm:"not null;default:0" json:"drip_rate"`
    DurationMinutes int    `gorm:"not null;default:0" json:"duration_minutes"`
    // 输液实际开始 / 结束时间（只对输液事件记录）。
    ActualStartAt *time.Time `json:"actual_start_at,omitempty"`
    ActualEndAt   *time.Time `json:"actual_end_at,omitempty"`
    ScheduledAt   time.Time  `gorm:"not null;index;uniqueIndex:uniq_event_plan_time" json:"scheduled_at"`
    LocalDate     string     `gorm:"size:10;not null;index" json:"local_date"`
    LocalTime     string     `gorm:"size:5;not null" json:"local_time"`
    Status        string     `gorm:"size:16;not null;default:pending;index" json:"status"`
    CompletedAt   *time.Time `json:"completed_at,omitempty"`
    CompletedBy   *uint      `json:"completed_by,omitempty"`
    MissedAt      *time.Time `json:"missed_at,omitempty"`
    MarkedBy      *uint      `json:"marked_by,omitempty"`
    SkippedAt     *time.Time `json:"skipped_at,omitempty"`
    SkipReason    string     `gorm:"size:24" json:"skip_reason"`
    AdjustmentID  *uint      `gorm:"index" json:"adjustment_id,omitempty"`
    Note          string     `gorm:"type:text" json:"note"`
    SnoozeUntil   *time.Time `json:"snooze_until,omitempty"`
    WasMissed     bool       `gorm:"not null;default:false" json:"was_missed"`
    StockShortage bool       `gorm:"not null;default:false" json:"stock_shortage"`
    Version       uint       `gorm:"not null;default:1" json:"version"`
    CreatedAt     time.Time  `json:"created_at"`
    UpdatedAt     time.Time  `json:"updated_at"`
}

type PrescriptionImport struct {
    ID             uint       `gorm:"primarykey" json:"id"`
    BookID         uint       `gorm:"not null;index" json:"book_id"`
    CreatedBy      uint       `gorm:"not null" json:"created_by"`
    Status         string     `gorm:"size:20;not null;index" json:"status"`
    Kind           string     `gorm:"size:20;not null;default:prescription;index" json:"kind"`
    // Intent 这条识别是拿来做什么的：archive 只建档 / stock 入库 / plan 生成用药计划（空=未标注，老数据）。
    Intent         string     `gorm:"size:20" json:"intent"`
    ProviderType   string     `gorm:"size:30" json:"provider_type"`
    Model          string     `gorm:"size:80" json:"model"`
    ImageEncrypted []byte     `gorm:"type:blob" json:"-"`
    ImageMIME      string     `gorm:"size:40" json:"-"`
    ImageSize      int        `json:"-"`
    ImageSHA256    string     `gorm:"size:64" json:"-"`
    OCRText        string     `gorm:"type:text" json:"ocr_text,omitempty"`
    // OCRProvider 本次识别实际用到的 OCR 服务商（空表示直接走视觉模型）；用于在记录里
    // 说明「这次是 OCR 提速还是直读图片」（2026-11-17 用户要求可见）。
    OCRProvider string `gorm:"size:20" json:"ocr_provider,omitempty"`
    // 以下 token 用量仅作展示与成本参考：AI 整理清单的输入/输出，以及 OCR 服务返回的 token。
    // 显式列名：GORM 默认命名会把 AIPromptTokens 拆成 a_iprompt_tokens，与 Update 的键不一致。
    AIPromptTokens     int `gorm:"column:ai_prompt_tokens" json:"ai_prompt_tokens"`
    AICompletionTokens int `gorm:"column:ai_completion_tokens" json:"ai_completion_tokens"`
    OCRTokens          int `gorm:"column:ocr_tokens" json:"ocr_tokens"`
    ParsedJSON     string     `gorm:"type:text" json:"parsed_json,omitempty"`
    ConfirmedJSON  string     `gorm:"type:text" json:"confirmed_json,omitempty"`
    Confidence     float64    `json:"confidence"`
    ErrorCode      string     `gorm:"size:40" json:"error_code,omitempty"`
    ErrorMessage   string     `gorm:"size:300" json:"error_message,omitempty"`
    ConfirmedAt    *time.Time `json:"confirmed_at,omitempty"`
    CreatedAt      time.Time  `json:"created_at"`
    UpdatedAt      time.Time  `json:"updated_at"`
}

// BookImage 是用药本里的图片资料：药品照片、处方单、治疗单、检查报告等。
// 图片内容 AES-GCM 加密后存数据库，随数据库备份一起走，不落在公开目录。
type BookImage struct {
    ID             uint   `gorm:"primarykey" json:"id"`
    BookID         uint   `gorm:"not null;index" json:"book_id"`
    Title          string `gorm:"size:120;not null" json:"title"`
    Category       string `gorm:"size:24;not null;default:other;index" json:"category"`
    // MedicineID 非 0 表示这张图片挂在某个药品下（药品照片 / 封面）。
    MedicineID     uint   `gorm:"not null;default:0;index" json:"medicine_id"`
    Tags           string `gorm:"type:text" json:"tags"`
    Note           string `gorm:"type:text" json:"note"`
    ImageEncrypted []byte `gorm:"type:blob" json:"-"`
    Mime           string `gorm:"size:60" json:"mime"`
    Size           int64  `json:"size"`
    // ThumbEncrypted 是列表用缩略图（长边约 420px JPEG），避免每次列表加载原图。
    ThumbEncrypted []byte    `gorm:"type:blob" json:"-"`
    ThumbMime      string    `gorm:"size:60" json:"thumb_mime"`
    ThumbSize      int64     `json:"thumb_size"`
    CreatedBy      uint      `gorm:"not null" json:"created_by"`
    CreatedAt      time.Time `json:"created_at"`
    UpdatedAt      time.Time `json:"updated_at"`
}

type NotificationChannel struct {
    ID              uint       `gorm:"primarykey" json:"id"`
    UserID          uint       `gorm:"not null;index" json:"user_id"`
    Type            string     `gorm:"size:20;not null" json:"type"`
    Name            string     `gorm:"size:40;not null" json:"name"`
    Enabled         bool       `gorm:"not null;default:true" json:"enabled"`
    ConfigEncrypted string     `gorm:"type:text;not null" json:"-"`
    TargetMasked    string     `gorm:"size:120" json:"target_masked"`
    Status          string     `gorm:"size:16;not null;default:active" json:"status"`
    VerifiedAt      *time.Time `json:"verified_at,omitempty"`
    LastTestedAt    *time.Time `json:"last_tested_at,omitempty"`
    LastErrorCode   string     `gorm:"size:40" json:"last_error_code,omitempty"`
    LastErrorAt     *time.Time `json:"last_error_at,omitempty"`
    ArchivedAt      *time.Time `gorm:"index" json:"-"`
    Version         uint       `gorm:"not null;default:1" json:"version"`
    CreatedAt       time.Time  `json:"created_at"`
    UpdatedAt       time.Time  `json:"updated_at"`
}

type NotificationJob struct {
    ID               uint       `gorm:"primarykey" json:"id"`
    BookID           uint       `gorm:"not null;index" json:"book_id"`
    RecipientUserID  uint       `gorm:"not null;index" json:"recipient_user_id"`
    ChannelID        uint       `gorm:"not null;default:0;index" json:"channel_id"`
    ChannelType      string     `gorm:"size:20;not null" json:"channel_type"`
    EventID          *uint      `gorm:"index" json:"event_id,omitempty"`
    Kind             string     `gorm:"size:20;not null" json:"kind"`
    ScheduledAt      time.Time  `json:"scheduled_at"`
    RunAt            time.Time  `gorm:"not null;index" json:"run_at"`
    Status           string     `gorm:"size:16;not null;default:pending;index" json:"status"`
    Attempts         int        `gorm:"not null;default:0" json:"attempts"`
    NextRetryAt      *time.Time `json:"next_retry_at,omitempty"`
    LockedAt         *time.Time `json:"locked_at,omitempty"`
    WorkerID         string     `gorm:"size:64" json:"worker_id,omitempty"`
    DedupeKey        string     `gorm:"size:160;not null;uniqueIndex" json:"-"`
    PayloadJSON      string     `gorm:"type:text" json:"payload_json"`
    LastErrorCode    string     `gorm:"size:40" json:"last_error_code,omitempty"`
    LastErrorMessage string     `gorm:"size:300" json:"last_error_message,omitempty"`
    ExternalID       string     `gorm:"size:200" json:"external_id,omitempty"`
    CreatedAt        time.Time  `json:"created_at"`
    UpdatedAt        time.Time  `json:"updated_at"`
}

type NotificationAttempt struct {
    ID              uint      `gorm:"primarykey" json:"id"`
    JobID           uint      `gorm:"not null;index;uniqueIndex:uniq_attempt_job_no" json:"job_id"`
    AttemptNo       int       `gorm:"not null;uniqueIndex:uniq_attempt_job_no" json:"attempt_no"`
    StartedAt       time.Time `json:"started_at"`
    FinishedAt      time.Time `json:"finished_at"`
    Result          string    `gorm:"size:16;not null" json:"result"`
    ProviderCode    string    `gorm:"size:100" json:"provider_code,omitempty"`
    LatencyMS       int64     `json:"latency_ms"`
    ResponseSummary string    `gorm:"size:300" json:"response_summary,omitempty"`
}

type InAppNotification struct {
    ID        uint       `gorm:"primarykey" json:"id"`
    BookID    uint       `gorm:"not null;index" json:"book_id"`
    UserID    uint       `gorm:"not null;index" json:"user_id"`
    EventID   *uint      `gorm:"index" json:"event_id,omitempty"`
    Type      string     `gorm:"size:30;not null" json:"type"`
    Title     string     `gorm:"size:200;not null" json:"title"`
    Body      string     `gorm:"size:1000" json:"body"`
    ReadAt    *time.Time `json:"read_at,omitempty"`
    CreatedAt time.Time  `gorm:"index" json:"created_at"`
}

type UserPreference struct {
    UserID                         uint   `gorm:"primarykey" json:"user_id"`
    Timezone                       string `gorm:"size:64;not null;default:Asia/Shanghai" json:"timezone"`
    LeadMinutes                    int    `gorm:"not null;default:10" json:"lead_minutes"`
    OverdueFirstMinutes            int    `gorm:"not null;default:15" json:"overdue_first_minutes"`
    OverdueRepeatMinutes           int    `gorm:"not null;default:60" json:"overdue_repeat_minutes"`
    OverdueMaxRepeats              int    `gorm:"not null;default:2" json:"overdue_max_repeats"`
    DailySummaryEnabled            bool   `gorm:"not null;default:true" json:"daily_summary_enabled"`
    DailySummaryTime               string `gorm:"size:5;not null;default:21:00" json:"daily_summary_time"`
    IncludeLocationInNotifications bool   `gorm:"not null;default:true" json:"include_location_in_notifications"`
    IncludeNotesInNotifications    bool   `gorm:"not null;default:false" json:"include_notes_in_notifications"`
    // LowStockNotifyEnabled 库存低于阈值或缺货时是否经消息渠道提醒。
    LowStockNotifyEnabled bool      `gorm:"not null;default:true" json:"low_stock_notify_enabled"`
    ThemeMode             string    `gorm:"size:16;not null;default:system" json:"theme_mode"`
    ThemeBrand            string    `gorm:"size:16;not null;default:a" json:"theme_brand"`
    LastBookID            *uint     `json:"last_book_id,omitempty"`
    OnboardingCompleted   bool      `gorm:"not null;default:false" json:"onboarding_completed"`
    // DonateSupportedVersion 账号级「已支持」状态：点过「已支持」时记下当时的应用版本，
    // 同账号在手机 / 电脑 / 飞牛入口共享，不再各设备各记一份（2026-11-16 用户要求改到后端）。
    DonateSupportedVersion string   `gorm:"size:32;not null;default:''" json:"donate_supported_version"`
    CreatedAt             time.Time `json:"created_at"`
    UpdatedAt             time.Time `json:"updated_at"`
}

// OperationLog 是用药本级的操作审计：每次成功的写操作都记一条，
// 谁在什么时候对哪个对象做了什么，便于家人之间追溯。
type OperationLog struct {
    ID          uint      `gorm:"primarykey" json:"id"`
    BookID      uint      `gorm:"not null;index" json:"book_id"`
    ActorUserID uint      `gorm:"index" json:"actor_user_id"`
    ActorName   string    `gorm:"size:60" json:"actor_name"`
    Action      string    `gorm:"size:120;index" json:"action"`
    Method      string    `gorm:"size:10" json:"method"`
    TargetType  string    `gorm:"size:40;index" json:"target_type"`
    TargetID    uint      `json:"target_id"`
    Detail      string    `gorm:"type:text" json:"detail"`
    IP          string    `gorm:"size:45" json:"ip"`
    CreatedAt   time.Time `gorm:"index" json:"created_at"`
}

type SystemNotificationConfig struct {
    ID                    uint      `gorm:"primarykey" json:"id"`
    SMTPHost              string    `gorm:"size:120" json:"smtp_host"`
    SMTPPort              int       `json:"smtp_port"`
    SMTPSecurity          string    `gorm:"size:20" json:"smtp_security"`
    SMTPUsername          string    `gorm:"size:120" json:"smtp_username"`
    SMTPPasswordEncrypted string    `gorm:"type:text" json:"-"`
    FromAddress           string    `gorm:"size:160" json:"from_address"`
    FromName              string    `gorm:"size:80" json:"from_name"`
    PublicBaseURL         string    `gorm:"size:300" json:"public_base_url"`
    Enabled               bool      `gorm:"not null;default:true" json:"enabled"`
    UpdatedAt             time.Time `json:"updated_at"`
}

type SystemAIConfig struct {
    ID              uint      `gorm:"primarykey" json:"id"`
    Enabled         bool      `gorm:"not null;default:false" json:"enabled"`
    ProviderType    string    `gorm:"size:30;not null;default:openai_compatible" json:"provider_type"`
    // ProviderName 自定义服务商的显示名称（选「自定义」时用户自填），列表服务商留空。
    ProviderName    string    `gorm:"size:60;not null;default:''" json:"provider_name"`
    BaseURL         string    `gorm:"size:300" json:"base_url"`
    APIKeyEncrypted string    `gorm:"type:text" json:"-"`
    Model           string    `gorm:"size:100" json:"model"`
    TimeoutSeconds  int       `gorm:"not null;default:60" json:"timeout_seconds"`
    MaxImageMB      int       `gorm:"not null;default:10" json:"max_image_mb"`
    PromptVersion   string    `gorm:"size:20" json:"prompt_version"`
    UpdatedAt       time.Time `json:"updated_at"`
}

// SystemOCRConfig 可选的 OCR 辅助：先读字再让文本模型整理清单，
// 比视觉模型直接读图快一个数量级，也不耗视觉额度（2026-11-16 定调）。
type SystemOCRConfig struct {
    ID                 uint      `gorm:"primarykey" json:"id"`
    Enabled            bool      `gorm:"not null;default:false" json:"enabled"`
    // Provider 服务商标识：baidu / tencent / aliyun / youdao / custom。
    // custom 走 OpenAI 兼容的多模态模型（发图让模型直接吐文字），任何视觉模型都能用。
    Provider           string    `gorm:"size:20;not null;default:baidu" json:"provider"`
    // BaseURL 仅自定义服务商使用；预设服务商留空走内置端点。
    BaseURL            string    `gorm:"size:300" json:"base_url"`
    APIKey             string    `gorm:"size:200" json:"api_key"`
    SecretKeyEncrypted string    `gorm:"type:text" json:"-"`
    // AppID 有道智云的「应用 ID」（其余服务商留空）。
    AppID              string    `gorm:"size:60" json:"app_id"`
    // Model 仅自定义服务商使用（视觉模型名）。
    Model              string    `gorm:"size:100" json:"model"`
    UpdatedAt          time.Time `json:"updated_at"`
}

// 辅助：会话中的 SSE 客户端修订号与广播。
type BookRevision struct {
    BookID   uint   `gorm:"primarykey"`
    Revision uint64 `gorm:"not null;default:0"`
}
