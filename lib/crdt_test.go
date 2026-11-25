package lib

import "testing"

func TestCompareAndNextHLC(t *testing.T) {
	a := FormatHLC(1700000000000, 0, "pc-1")
	b := NextHLC(1700000000000, a, "", "phone-1")
	ha, err := ParseHLC(a)
	if err != nil {
		t.Fatalf("ParseHLC(a): %v", err)
	}
	hb, err := ParseHLC(b)
	if err != nil {
		t.Fatalf("ParseHLC(b): %v", err)
	}
	if CompareHLC(hb, ha) <= 0 {
		t.Fatalf("expected b (%s) > a (%s)", b, a)
	}
}

func TestFieldConflictDecision(t *testing.T) {
	cur := FormatHLC(1700000000000, 1, "pc-1")
	next := FormatHLC(1700000001000, 0, "phone-1")
	older := FormatHLC(1699999999000, 5, "phone-1")

	// Causal update matching base_hlc applies cleanly
	d, err := FieldConflictDecision(cur, cur, next, false)
	if err != nil || d != "applied" {
		t.Fatalf("causal: got (%q, %v), want applied", d, err)
	}

	// Concurrent offline edit with stale base_hlc reports conflict
	d, err = FieldConflictDecision(cur, older, next, false)
	if err != nil || d != "conflict" {
		t.Fatalf("concurrent: got (%q, %v), want conflict", d, err)
	}

	// Forced resolution wins
	d, err = FieldConflictDecision(cur, older, next, true)
	if err != nil || d != "applied" {
		t.Fatalf("force: got (%q, %v), want applied", d, err)
	}

	// Older HLC is ignored as stale
	d, err = FieldConflictDecision(cur, cur, older, false)
	if err != nil || d != "stale" {
		t.Fatalf("stale: got (%q, %v), want stale", d, err)
	}
}
