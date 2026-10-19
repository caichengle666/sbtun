//go:build !darwin

package app

import "testing"

func TestLimitTrayTooltip(t *testing.T) {
	long := ""
	for range 200 {
		long += "节"
	}
	if got := []rune(limitTrayTooltip(long)); len(got) != 127 {
		t.Fatalf("len=%d", len(got))
	}
	if limitTrayTooltip("sbtun") != "sbtun" {
		t.Fatal("short tooltip changed")
	}
}
