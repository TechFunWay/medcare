package medcare

import "testing"

func TestDefaultDoseUnit(t *testing.T) {
    cases := []struct{ form, cat, want string }{
        {"注射液", "injection", "支"},
        {"粉针剂", "injection", "支"},
        {"输液", "infusion", "瓶"},
        {"颗粒剂", "oral", "袋"},
        {"分散片", "oral", "盒"},
        {"软膏", "topical", "支"},
        {"喷雾剂", "other", "瓶"},
        {"", "injection", "支"},
        {"", "infusion", "瓶"},
        {"", "other", "盒"},
    }
    for _, c := range cases {
        if got := defaultDoseUnit(c.form, c.cat); got != c.want {
            t.Errorf("defaultDoseUnit(%q,%q)=%q want %q", c.form, c.cat, got, c.want)
        }
    }
}
