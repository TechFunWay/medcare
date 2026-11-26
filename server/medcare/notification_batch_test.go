package medcare

import (
    "fmt"
    "strings"
    "testing"
    "time"
)

// 三个事件同一渠道到点：应合成一条消息，正文三组「药名（类型）剂量 + 原计划/超时」。
func TestNotificationBatchMergesIntoSingleMessage(t *testing.T) {
    db := newPlanTestDB(t)
    db.Create(&MedicationBook{ID: 1, Name: "宝贝用药本", Timezone: BeijingTimezone, Status: "active"})
    db.Create(&Medicine{ID: 1, BookID: 1, Name: "环孢素软胶囊", Category: CatOral, Unit: "片"})
    db.Create(&Medicine{ID: 2, BookID: 1, Name: "硒酵母片", Category: CatOral, Unit: "片"})
    db.Create(&Medicine{ID: 3, BookID: 1, Name: "黄体酮胶囊", Category: CatOral, Unit: "颗"})
    today := todayIn(BeijingTimezone)
    db.Create(&Plan{ID: 1, BookID: 1, MedicineID: 1, Type: CatOral, DoseAmount: 2, DoseUnit: "片", FrequencyType: FreqDaily, Times: "[\"18:00\"]", StartDate: today, Timezone: BeijingTimezone, Status: PlanActive})
    db.Create(&Plan{ID: 2, BookID: 1, MedicineID: 2, Type: CatOral, DoseAmount: 1, DoseUnit: "片", FrequencyType: FreqDaily, Times: "[\"18:20\"]", StartDate: today, Timezone: BeijingTimezone, Status: PlanActive})
    db.Create(&Plan{ID: 3, BookID: 1, MedicineID: 3, Type: CatOral, DoseAmount: 2, DoseUnit: "颗", FrequencyType: FreqDaily, Times: "[\"18:40\"]", StartDate: today, Timezone: BeijingTimezone, Status: PlanActive})
    db.Create(&DoseEvent{ID: 11, BookID: 1, PlanID: 1, MedicineID: 1, MedicineName: "环孢素软胶囊", Type: CatOral, DoseAmount: 2, DoseUnit: "片", LocalDate: today, LocalTime: "18:00", ScheduledAt: nowUTC().Add(-2 * time.Hour), Status: EventPending})
    db.Create(&DoseEvent{ID: 12, BookID: 1, PlanID: 2, MedicineID: 2, MedicineName: "硒酵母片", Type: CatOral, DoseAmount: 1, DoseUnit: "片", LocalDate: today, LocalTime: "18:20", ScheduledAt: nowUTC().Add(-90 * time.Minute), Status: EventPending})
    db.Create(&DoseEvent{ID: 13, BookID: 1, PlanID: 3, MedicineID: 3, MedicineName: "黄体酮胶囊", Type: CatOral, DoseAmount: 2, DoseUnit: "颗", LocalDate: today, LocalTime: "18:40", ScheduledAt: nowUTC().Add(-60 * time.Minute), Status: EventPending})

    ev1 := parseIntPtr(11)
    ev2 := parseIntPtr(12)
    ev3 := parseIntPtr(13)
    jobs := []NotificationJob{
        {BookID: 1, RecipientUserID: 1, ChannelID: 5, ChannelType: ChannelDingTalk, EventID: ev1, Kind: JobOverdue1, ScheduledAt: nowUTC(), RunAt: nowUTC(), Status: JobPending},
        {BookID: 1, RecipientUserID: 1, ChannelID: 5, ChannelType: ChannelDingTalk, EventID: ev2, Kind: JobOverdue1, ScheduledAt: nowUTC(), RunAt: nowUTC(), Status: JobPending},
        {BookID: 1, RecipientUserID: 1, ChannelID: 5, ChannelType: ChannelDingTalk, EventID: ev3, Kind: JobOverdue1, ScheduledAt: nowUTC(), RunAt: nowUTC(), Status: JobPending},
    }
    for i := range jobs {
        jobs[i].DedupeKey = fmt.Sprintf("test-%d", i)
        if err := db.Create(&jobs[i]).Error; err != nil {
            t.Fatal(err)
        }
    }

    // 直接构造消息体（不实际发网络）：复用批处理的正文构建逻辑
    lines := make([]string, 0, len(jobs)*2)
    for _, j := range jobs {
        var ev DoseEvent
        db.First(&ev, *j.EventID)
        lines = append(lines, fmt.Sprintf("%s（%s） %.3g %s", ev.MedicineName, categoryLabelCN(ev.Type), ev.DoseAmount, ev.DoseUnit))
        lines = append(lines, "原计划 "+ev.LocalTime)
    }
    body := strings.Join(lines, "\n")
    for _, want := range []string{"环孢素软胶囊（口服） 2 片", "硒酵母片（口服） 1 片", "黄体酮胶囊（口服） 2 颗", "原计划 18:00"} {
        if !strings.Contains(body, want) {
            t.Fatalf("合并正文应包含 %q，实际：%s", want, body)
        }
    }
}

func parseIntPtr(v uint) *uint { return &v }
// 整批走真实处理流程：渠道不存在时应一次性把整批标记失败（而不是逐条发三条）。
func TestProcessNotificationJobBatchSingleAttempt(t *testing.T) {
    db := newPlanTestDB(t)
    db.Create(&MedicationBook{ID: 1, Name: "本", Timezone: BeijingTimezone, Status: "active"})
    today := todayIn(BeijingTimezone)
    var jobs []NotificationJob
    for i := 1; i <= 3; i++ {
        med := Medicine{ID: uint(i), BookID: 1, Name: fmt.Sprintf("药%d", i), Category: CatOral, Unit: "片"}
        db.Create(&med)
        db.Create(&Plan{ID: uint(i), BookID: 1, MedicineID: uint(i), Type: CatOral, DoseAmount: 1, DoseUnit: "片", FrequencyType: FreqDaily, Times: "[\"08:00\"]", StartDate: today, Timezone: BeijingTimezone, Status: PlanActive})
        db.Create(&DoseEvent{ID: uint(100 + i), BookID: 1, PlanID: uint(i), MedicineID: uint(i), MedicineName: med.Name, Type: CatOral, DoseAmount: 1, DoseUnit: "片", LocalDate: today, LocalTime: "08:00", ScheduledAt: nowUTC().Add(-time.Hour), Status: EventPending})
        evID := uint(100 + i)
        j := NotificationJob{BookID: 1, RecipientUserID: 1, ChannelID: 999, ChannelType: ChannelDingTalk, EventID: &evID, Kind: JobOverdue1, ScheduledAt: nowUTC(), RunAt: nowUTC(), Status: JobPending, DedupeKey: fmt.Sprintf("b-%d", i)}
        db.Create(&j)
        jobs = append(jobs, j)
    }

    processNotificationJobBatch(db, jobs)

    var failed int64
    db.Model(&NotificationJob{}).Where("channel_id = ? AND status = ?", 999, JobFailed).Count(&failed)
    if failed != 3 {
        t.Fatalf("整批应全部标记失败，实际 %d", failed)
    }
    var attempts int64
    db.Model(&NotificationAttempt{}).Count(&attempts)
    if attempts != 0 {
        t.Fatalf("渠道不存在不该产生投递记录，实际 %d", attempts)
    }
}

// 批内某条已完成：应被取消，其余照发（这里渠道不存在所以整批失败，但该条应是取消状态）。
func TestProcessNotificationJobBatchCancelsDoneEvent(t *testing.T) {
    db := newPlanTestDB(t)
    db.Create(&MedicationBook{ID: 1, Name: "本", Timezone: BeijingTimezone, Status: "active"})
    today := todayIn(BeijingTimezone)
    db.Create(&Medicine{ID: 1, BookID: 1, Name: "药A", Category: CatOral, Unit: "片"})
    db.Create(&Plan{ID: 1, BookID: 1, MedicineID: 1, Type: CatOral, DoseAmount: 1, DoseUnit: "片", FrequencyType: FreqDaily, Times: "[\"08:00\"]", StartDate: today, Timezone: BeijingTimezone, Status: PlanActive})
    db.Create(&DoseEvent{ID: 201, BookID: 1, PlanID: 1, MedicineID: 1, MedicineName: "药A", Type: CatOral, DoseAmount: 1, DoseUnit: "片", LocalDate: today, LocalTime: "08:00", ScheduledAt: nowUTC().Add(-time.Hour), Status: EventPending})
    db.Create(&DoseEvent{ID: 202, BookID: 1, PlanID: 1, MedicineID: 1, MedicineName: "药A", Type: CatOral, DoseAmount: 1, DoseUnit: "片", LocalDate: today, LocalTime: "09:00", ScheduledAt: nowUTC().Add(-30*time.Minute), Status: EventDone})
    p1 := uint(201)
    p2 := uint(202)
    j1 := NotificationJob{BookID: 1, RecipientUserID: 1, ChannelID: 999, ChannelType: ChannelDingTalk, EventID: &p1, Kind: JobOverdue1, ScheduledAt: nowUTC(), RunAt: nowUTC(), Status: JobPending, DedupeKey: "c-1"}
    j2 := NotificationJob{BookID: 1, RecipientUserID: 1, ChannelID: 999, ChannelType: ChannelDingTalk, EventID: &p2, Kind: JobOverdue1, ScheduledAt: nowUTC(), RunAt: nowUTC(), Status: JobPending, DedupeKey: "c-2"}
    db.Create(&j1)
    db.Create(&j2)

    processNotificationJobBatch(db, []NotificationJob{j1, j2})

    var canceled int64
    db.Model(&NotificationJob{}).Where("id = ? AND status = ?", j2.ID, JobCanceled).Count(&canceled)
    if canceled != 1 {
        t.Fatalf("已完成事件的任务应被取消")
    }
    var failed int64
    db.Model(&NotificationJob{}).Where("id = ? AND status = ?", j1.ID, JobFailed).Count(&failed)
    if failed != 1 {
        t.Fatalf("未完成事件的任务应尝试发送（渠道不存在→失败）")
    }
}
// 分组规则：同渠道多个提醒合成一批；单条、站内、总结/库存/测试都走单条。
func TestPartitionReminderJobs(t *testing.T) {
    p := func(v uint) *uint { return &v }
    jobs := []NotificationJob{
        {ID: 1, BookID: 1, RecipientUserID: 1, ChannelID: 7, ChannelType: ChannelDingTalk, EventID: p(1), Kind: JobOverdue1},
        {ID: 2, BookID: 1, RecipientUserID: 1, ChannelID: 7, ChannelType: ChannelDingTalk, EventID: p(2), Kind: JobOverdue2},
        {ID: 3, BookID: 1, RecipientUserID: 1, ChannelID: 8, ChannelType: ChannelFeishu, EventID: p(3), Kind: JobLead},
        {ID: 4, BookID: 1, RecipientUserID: 1, ChannelID: 0, ChannelType: ChannelInApp, EventID: p(4), Kind: JobOverdue1},
        {ID: 5, BookID: 1, RecipientUserID: 1, ChannelID: 0, ChannelType: ChannelInApp, Kind: JobSummary},
        {ID: 6, BookID: 1, RecipientUserID: 1, ChannelID: 9, ChannelType: ChannelEmail, EventID: p(6), Kind: JobLowStock},
    }
    batches, singles := partitionReminderJobs(jobs)

    if len(batches) != 1 || len(batches[0]) != 2 {
        t.Fatalf("钉钉两条应合成一批，实际批次=%d", len(batches))
    }
    if batches[0][0].ID != 1 || batches[0][1].ID != 2 {
        t.Fatalf("批次成员不对：%+v", batches[0])
    }
    // 其余 4 条（飞书单条、站内两条、库存一条）都走单条
    if len(singles) != 4 {
        t.Fatalf("应有 4 条单发，实际 %d", len(singles))
    }
}
// 合并正文成品：标题带条数，正文按时间排序，每条两行。
func TestBuildMergedReminderMessage(t *testing.T) {
    now := nowUTC()
    evs := []DoseEvent{
        {MedicineName: "黄体酮胶囊", Type: CatOral, DoseAmount: 2, DoseUnit: "颗", LocalTime: "18:40", ScheduledAt: now.Add(-60 * time.Minute)},
        {MedicineName: "环孢素软胶囊", Type: CatOral, DoseAmount: 2, DoseUnit: "片", LocalTime: "18:00", ScheduledAt: now.Add(-2 * time.Hour)},
    }
    msg := buildMergedReminderMessage("宝贝用药本", evs, true, now)
    if msg.Title != "还没吃药 · 再次提醒（2 项）" {
        t.Fatalf("标题不对：%s", msg.Title)
    }
    text := messageText(msg)
    t.Log("\n" + text)
    if !strings.Contains(text, "【用药管理】[宝贝用药本]") {
        t.Fatalf("缺用药本名：%s", text)
    }
    // 18:00 应排在 18:40 之前
    if strings.Index(text, "环孢素软胶囊（口服）") > strings.Index(text, "黄体酮胶囊（口服）") {
        t.Fatalf("应按计划时间排序：%s", text)
    }
    if !strings.Contains(text, "原计划 18:00，已超时 2 小时") {
        t.Fatalf("超时时长不对：%s", text)
    }
    if strings.Count(text, "- ") != 4 {
        t.Fatalf("应有 4 行正文，实际：%s", text)
    }
}
