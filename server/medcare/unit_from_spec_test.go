package medcare

import "testing"

func TestUnitFromSpec(t *testing.T) {
    cases := []struct{ spec, want string; ok bool }{
        {"2mg*28片", "片", true},
        {"0.2g×10片", "片", true},
        {"10mg/支", "支", true},
        {"20片/盒", "片", true},
        {"3g*10袋", "袋", true},
        {"100ml/瓶", "瓶", true},
        {"20% 10g", "", false},
        {"", "", false},
        {"10mg", "", false},
    }
    for _, c := range cases {
        got, ok := unitFromSpec(c.spec)
        if ok != c.ok || got != c.want {
            t.Errorf("unitFromSpec(%q)=(%q,%v) want (%q,%v)", c.spec, got, ok, c.want, c.ok)
        }
    }
}
