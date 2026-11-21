package medcare

import (
    "encoding/json"
    "testing"
)

func TestFlexNumericAndListParsing(t *testing.T) {
    payload := `{
      "doc_type": "receipt",
      "hospital": "市第一人民医院",
      "items": [
        {
          "raw_name": "苯磺酸氨氯地平片(5mg*28片)",
          "name": "苯磺酸氨氯地平片",
          "spec": "5mg×28片",
          "dose_amount": "5mg",
          "dose_unit": "片",
          "quantity": "2 盒",
          "unit": "盒",
          "unit_price": "￥32.50",
          "amount": "65.00元",
          "times": "08:00, 20:00",
          "weekdays": [1, 3, 5],
          "interval_days": "2天",
          "duration_days": 7,
          "confidence": "0.88",
          "needs_review": "true",
          "low_stock_threshold": "10",
          "expiry_warn_days": "30天"
        },
        {
          "name": "阿托伐他汀钙片",
          "quantity": 3,
          "times": ["每日一次"],
          "needs_review": false
        }
      ]
    }`
    var draft prescriptionDraft
    if err := json.Unmarshal([]byte(payload), &draft); err != nil {
        t.Fatalf("unmarshal failed: %v", err)
    }
    if len(draft.Items) != 2 {
        t.Fatalf("items = %d, want 2", len(draft.Items))
    }
    a := draft.Items[0]
    if a.Quantity != 2 {
        t.Fatalf("quantity = %v, want 2", a.Quantity)
    }
    if a.UnitPrice != 32.5 || a.Amount != 65 {
        t.Fatalf("unit_price/amount = %v/%v", a.UnitPrice, a.Amount)
    }
    if a.DoseAmount != 5 {
        t.Fatalf("dose_amount = %v, want 5", a.DoseAmount)
    }
    if len(a.Times) != 2 || a.Times[0] != "08:00" || a.Times[1] != "20:00" {
        t.Fatalf("times = %v", a.Times)
    }
    if len(a.Weekdays) != 3 || a.Weekdays[0] != 1 || a.Weekdays[2] != 5 {
        t.Fatalf("weekdays = %v", a.Weekdays)
    }
    if a.IntervalDays != 2 || a.DurationDays != 7 {
        t.Fatalf("interval/duration = %v/%v", a.IntervalDays, a.DurationDays)
    }
    if !a.NeedsReview {
        t.Fatalf("needs_review should be true")
    }
    if a.LowStockThreshold != 10 || a.ExpiryWarnDays != 30 {
        t.Fatalf("threshold/warn = %v/%v", a.LowStockThreshold, a.ExpiryWarnDays)
    }
    if a.Unit != "盒" || a.Spec != "5mg×28片" {
        t.Fatalf("unit/spec = %q/%q", a.Unit, a.Spec)
    }
    b := draft.Items[1]
    if b.Quantity != 3 || len(b.Times) != 1 || b.Times[0] != "每日一次" {
        t.Fatalf("second item = %+v", b)
    }
}

func TestFlexParsingNullAndGarbage(t *testing.T) {
    var draft prescriptionDraft
    payload := `{"items":[{"name":"x","quantity":null,"unit_price":"","dose_amount":{"a":1},"times":null}]}`
    if err := json.Unmarshal([]byte(payload), &draft); err != nil {
        t.Fatalf("unmarshal failed: %v", err)
    }
    it := draft.Items[0]
    if it.Quantity != 0 || it.UnitPrice != 0 || it.DoseAmount != 0 || it.Times != nil {
        t.Fatalf("null/garbage not zeroed: %+v", it)
    }
}

func TestParseFlexNumber(t *testing.T) {
    cases := map[string]float64{
        "2 盒":     2,
        "￥32.50":  32.5,
        "1,234.5": 1234.5,
        "28片":     28,
        "0.5mg":   0.5,
        "":        0,
        "abc":     0,
        "-3":      -3,
    }
    for in, want := range cases {
        if got := parseFlexNumber(in); got != want {
            t.Fatalf("parseFlexNumber(%q) = %v, want %v", in, got, want)
        }
    }
}
