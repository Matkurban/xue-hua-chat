package chat

import "testing"

func TestFillDays(t *testing.T) {
	// 2026-10-13 00:00 +08:00 .. 2026-10-20 00:00 +08:00 (7 days)
	start := int64(1791820800000)
	end := start + 7*86400000
	got, err := fillDays(start, end, map[string]int64{"2026-10-15": 3, "2026-09-01": 9})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 7 {
		t.Fatalf("want 7 days, got %d: %v", len(got), got)
	}
	if got["2026-10-13"] != 0 || got["2026-10-15"] != 3 || got["2026-10-19"] != 0 {
		t.Fatalf("bad fill %v", got)
	}
	if _, ok := got["2026-10-20"]; ok {
		t.Fatal("end is exclusive")
	}
	// UTC-midnight start (16:00 prev day Shanghai... start=08:00 local) still covers local days correctly
	got, _ = fillDays(start-8*3600000, start+86400000-8*3600000, nil)
	if len(got) != 2 {
		t.Fatalf("utc-aligned range spans 2 local days, got %v", got)
	}
	if _, err := fillDays(0, 5000*86400000, nil); err == nil {
		t.Fatal("expected range limit error")
	}
}
