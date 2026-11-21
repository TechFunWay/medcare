package medcare

import (
    "encoding/json"
    "strconv"
    "strings"
)

// prescriptionItemRaw 与 prescriptionItem 字段一一对应，但用宽松类型接收，
// 再转换成业务结构，避免模型把数字写成字符串时整条记录解析失败。
type prescriptionItemRaw struct {
    RawName           flexString     `json:"raw_name"`
    Name              flexString     `json:"name"`
    GenericName       flexString     `json:"generic_name"`
    Spec              flexString     `json:"spec"`
    Form              flexString     `json:"form"`
    Route             flexString     `json:"route"`
    DoseAmount        flexFloat      `json:"dose_amount"`
    DoseUnit          flexString     `json:"dose_unit"`
    FrequencyText     flexString     `json:"frequency_text"`
    FrequencyType     flexString     `json:"frequency_type"`
    Times             flexStringList `json:"times"`
    CustomSchedule    []ScheduleDay `json:"custom_schedule"`
    IntervalDays      flexInt        `json:"interval_days"`
    Weekdays          flexStringList `json:"weekdays"`
    IntervalHours     flexInt        `json:"interval_hours"`
    DurationDays      flexInt        `json:"duration_days"`
    Quantity          flexFloat      `json:"quantity"`
    UnitPrice         flexFloat      `json:"unit_price"`
    Amount            flexFloat      `json:"amount"`
    Instructions      flexString     `json:"instructions"`
    Confidence        flexFloat      `json:"confidence"`
    NeedsReview       flexBool       `json:"needs_review"`
    Category          flexString     `json:"category"`
    Unit              flexString     `json:"unit"`
    PackageSpec       flexString     `json:"package_spec"`
    Manufacturer      flexString     `json:"manufacturer"`
    ApprovalNo        flexString     `json:"approval_no"`
    ExpiryDate        flexString     `json:"expiry_date"`
    LowStockThreshold flexFloat      `json:"low_stock_threshold"`
    ExpiryWarnDays    flexInt        `json:"expiry_warn_days"`
}

// UnmarshalJSON 让模型返回的「数字写成字符串 / 带单位」也能顺利解析。
func (it *prescriptionItem) UnmarshalJSON(data []byte) error {
    var raw prescriptionItemRaw
    if err := json.Unmarshal(data, &raw); err != nil {
        return err
    }
    weekdays := make([]int, 0, len(raw.Weekdays))
    for _, w := range raw.Weekdays {
        n, err := strconv.Atoi(strings.TrimSpace(w))
        if err == nil && n > 0 && n <= 7 {
            weekdays = append(weekdays, n)
        }
    }
    *it = prescriptionItem{
        RawName:           strings.TrimSpace(string(raw.RawName)),
        Name:              strings.TrimSpace(string(raw.Name)),
        GenericName:       strings.TrimSpace(string(raw.GenericName)),
        Spec:              strings.TrimSpace(string(raw.Spec)),
        Form:              strings.TrimSpace(string(raw.Form)),
        Route:             strings.TrimSpace(string(raw.Route)),
        DoseAmount:        float64(raw.DoseAmount),
        DoseUnit:          strings.TrimSpace(string(raw.DoseUnit)),
        FrequencyText:     strings.TrimSpace(string(raw.FrequencyText)),
        FrequencyType:     strings.TrimSpace(string(raw.FrequencyType)),
        Times:             raw.Times,
        CustomSchedule:    raw.CustomSchedule,
        IntervalDays:      int(raw.IntervalDays),
        Weekdays:          weekdays,
        IntervalHours:     int(raw.IntervalHours),
        DurationDays:      int(raw.DurationDays),
        Quantity:          float64(raw.Quantity),
        UnitPrice:         float64(raw.UnitPrice),
        Amount:            float64(raw.Amount),
        Instructions:      strings.TrimSpace(string(raw.Instructions)),
        Confidence:        float64(raw.Confidence),
        NeedsReview:       bool(raw.NeedsReview),
        Category:          strings.TrimSpace(string(raw.Category)),
        Unit:              strings.TrimSpace(string(raw.Unit)),
        PackageSpec:       strings.TrimSpace(string(raw.PackageSpec)),
        Manufacturer:      strings.TrimSpace(string(raw.Manufacturer)),
        ApprovalNo:        strings.TrimSpace(string(raw.ApprovalNo)),
        ExpiryDate:        strings.TrimSpace(string(raw.ExpiryDate)),
        LowStockThreshold: float64(raw.LowStockThreshold),
        ExpiryWarnDays:    int(raw.ExpiryWarnDays),
    }
    return nil
}
