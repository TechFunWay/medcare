package medcare

import "testing"

func TestIsProcedureItem(t *testing.T) {
	procedures := []string{"静脉滴注", "肌肉注射", "连续静脉滴注", "皮下注射", "静滴"}
	notProcedures := []string{"黄体酮注射液", "0.9%氯化钠注射液 100ml", "人免疫球蛋白", "连续静脉滴注用生理盐水", ""}
	for _, n := range procedures {
		if !isProcedureItem(n) {
			t.Errorf("isProcedureItem(%q) = false, want true", n)
		}
	}
	for _, n := range notProcedures {
		if isProcedureItem(n) {
			t.Errorf("isProcedureItem(%q) = true, want false", n)
		}
	}
}
