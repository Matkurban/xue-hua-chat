package rtc

import (
	"testing"
	"time"

	chatdb "chat/pkg/common/db/table/chat"
)

func TestExpandOnce(t *testing.T) {
	start := time.Date(2026, 7, 26, 10, 0, 0, 0, time.UTC).UnixMilli()
	out, err := ExpandOccurrenceStarts(start, "UTC", chatdb.ScheduleRecurrence{Freq: "none"})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || out[0] != start {
		t.Fatalf("got %#v", out)
	}
}

func TestExpandDaily(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Shanghai")
	first := time.Date(2026, 7, 1, 10, 0, 0, 0, loc)
	out, err := ExpandOccurrenceStarts(first.UnixMilli(), "Asia/Shanghai", chatdb.ScheduleRecurrence{
		Freq: "daily", Interval: 1, Count: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 3 {
		t.Fatalf("len=%d", len(out))
	}
	for i, ms := range out {
		got := time.UnixMilli(ms).In(loc)
		want := first.AddDate(0, 0, i)
		if !got.Equal(want) {
			t.Fatalf("i=%d got %v want %v", i, got, want)
		}
	}
}

func TestExpandWeekly(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Shanghai")
	// 2026-07-01 is Wednesday
	first := time.Date(2026, 7, 1, 15, 0, 0, 0, loc)
	out, err := ExpandOccurrenceStarts(first.UnixMilli(), "Asia/Shanghai", chatdb.ScheduleRecurrence{
		Freq: "weekly", Interval: 1, ByWeekdays: []int{3}, Count: 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 4 {
		t.Fatalf("len=%d out=%v", len(out), out)
	}
}

func TestExpandWeeklyIncludesFirstEvenIfNotInByWeekdays(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Shanghai")
	// Wednesday
	first := time.Date(2026, 7, 1, 15, 0, 0, 0, loc)
	out, err := ExpandOccurrenceStarts(first.UnixMilli(), "Asia/Shanghai", chatdb.ScheduleRecurrence{
		Freq: "weekly", Interval: 1, ByWeekdays: []int{1}, Count: 4, // Monday only, but first is Wed
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) == 0 || out[0] != first.UnixMilli() {
		t.Fatalf("first occurrence must be preserved, got %#v", out)
	}
}

func TestParseSiConversationUserIDs(t *testing.T) {
	got := parseSiConversationUserIDs("si_host_peer_x", "host")
	if len(got) != 2 || got[0] != "host" || got[1] != "peer_x" {
		t.Fatalf("got %#v", got)
	}
	got = parseSiConversationUserIDs("si_peer_x_host", "host")
	if len(got) != 2 || got[0] != "peer_x" || got[1] != "host" {
		t.Fatalf("got %#v", got)
	}
}

func TestExpandMonthlySkipShortMonth(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Shanghai")
	first := time.Date(2026, 1, 31, 10, 0, 0, 0, loc)
	out, err := ExpandOccurrenceStarts(first.UnixMilli(), "Asia/Shanghai", chatdb.ScheduleRecurrence{
		Freq: "monthly", Interval: 1, ByMonthDay: 31, Count: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	// Jan 31, skip Feb, Mar 31, ...
	if len(out) < 2 {
		t.Fatalf("len=%d", len(out))
	}
	d0 := time.UnixMilli(out[0]).In(loc)
	d1 := time.UnixMilli(out[1]).In(loc)
	if d0.Month() != time.January || d0.Day() != 31 {
		t.Fatalf("first %v", d0)
	}
	if d1.Month() != time.March || d1.Day() != 31 {
		t.Fatalf("second should skip Feb, got %v", d1)
	}
}
