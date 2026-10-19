package statsday

import "testing"

func TestFillDays(t *testing.T) {
	start := int64(1791820800000) // 2026-10-13 00:00 +08:00
	got, err := FillDays(start, start+3*86400000, map[string]int64{"2026-10-14": 4, "2026-01-01": 9})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got["2026-10-13"] != 0 || got["2026-10-14"] != 4 || got["2026-10-15"] != 0 {
		t.Fatalf("bad %v", got)
	}
	// UTC-midnight start covers two local days
	got, _ = FillDays(start-8*3600000, start+86400000-8*3600000, nil)
	if len(got) != 2 {
		t.Fatalf("got %v", got)
	}
	if _, err := FillDays(0, 5000*86400000, nil); err == nil {
		t.Fatal("expected limit error")
	}
}
