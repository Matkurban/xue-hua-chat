package rtc

import (
	"fmt"
	"time"

	chatdb "chat/pkg/common/db/table/chat"
)

const (
	scheduleMaxOccurrences = 100
	scheduleMaxHorizonDays = 180
)

// ExpandOccurrenceStarts expands recurrence into UTC start timestamps (ms).
// firstStartUTC is the absolute start of the first occurrence.
// timezone is IANA; wall-clock recurrence is computed in that location.
func ExpandOccurrenceStarts(firstStartUTC int64, timezone string, rec chatdb.ScheduleRecurrence) ([]int64, error) {
	if firstStartUTC <= 0 {
		return nil, fmt.Errorf("startTime invalid")
	}
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return nil, fmt.Errorf("invalid timezone: %w", err)
	}
	freq := rec.Freq
	if freq == "" || freq == "none" {
		return []int64{firstStartUTC}, nil
	}
	interval := rec.Interval
	if interval <= 0 {
		interval = 1
	}

	first := time.UnixMilli(firstStartUTC).In(loc)
	horizon := first.AddDate(0, 0, scheduleMaxHorizonDays)
	var until time.Time
	if rec.Until > 0 {
		until = time.UnixMilli(rec.Until).In(loc)
		// include until day end in local tz
		until = time.Date(until.Year(), until.Month(), until.Day(), 23, 59, 59, 999000000, loc)
	}
	maxCount := rec.Count
	if maxCount <= 0 || maxCount > scheduleMaxOccurrences {
		maxCount = scheduleMaxOccurrences
	}

	out := make([]int64, 0, 16)
	switch freq {
	case "daily":
		for i := 0; len(out) < maxCount; i++ {
			t := first.AddDate(0, 0, i*interval)
			if t.After(horizon) {
				break
			}
			if rec.Until > 0 && t.After(until) {
				break
			}
			out = append(out, t.UTC().UnixMilli())
		}
	case "weekly":
		weekdays := rec.ByWeekdays
		firstWD := int(first.Weekday())
		if firstWD == 0 {
			firstWD = 7
		}
		if len(weekdays) == 0 {
			weekdays = []int{firstWD}
		}
		wdSet := map[int]struct{}{}
		for _, d := range weekdays {
			if d < 1 || d > 7 {
				return nil, fmt.Errorf("byWeekdays must be 1..7")
			}
			wdSet[d] = struct{}{}
		}
		// Always include the first occurrence's weekday so startTime is never dropped.
		wdSet[firstWD] = struct{}{}
		// Day-walk with AddDate avoids DST skew from Hours()/24.
		firstDate := time.Date(first.Year(), first.Month(), first.Day(), 0, 0, 0, 0, loc)
		cursor := firstDate
		for len(out) < maxCount {
			dayWD := int(cursor.Weekday())
			if dayWD == 0 {
				dayWD = 7
			}
			daysFromFirst := calendarDaysBetween(firstDate, cursor)
			if daysFromFirst < 0 {
				cursor = cursor.AddDate(0, 0, 1)
				continue
			}
			weekIndex := daysFromFirst / 7
			if weekIndex%interval == 0 {
				if _, ok := wdSet[dayWD]; ok {
					cand := time.Date(cursor.Year(), cursor.Month(), cursor.Day(), first.Hour(), first.Minute(), first.Second(), first.Nanosecond(), loc)
					if !cand.Before(first) {
						if cand.After(horizon) {
							break
						}
						if rec.Until > 0 && cand.After(until) {
							break
						}
						out = append(out, cand.UTC().UnixMilli())
					}
				}
			}
			cursor = cursor.AddDate(0, 0, 1)
			if cursor.After(horizon.AddDate(0, 0, 7)) {
				break
			}
		}
	case "monthly":
		monthDay := rec.ByMonthDay
		if monthDay <= 0 {
			monthDay = first.Day()
		}
		if monthDay > 31 {
			return nil, fmt.Errorf("byMonthDay invalid")
		}
		for i := 0; len(out) < maxCount; i++ {
			y, m := first.Year(), int(first.Month())+i*interval
			for m > 12 {
				y++
				m -= 12
			}
			// skip months without this day
			lastDay := daysInMonth(y, time.Month(m))
			if monthDay > lastDay {
				continue
			}
			cand := time.Date(y, time.Month(m), monthDay, first.Hour(), first.Minute(), first.Second(), first.Nanosecond(), loc)
			if cand.Before(first) {
				continue
			}
			if cand.After(horizon) {
				break
			}
			if rec.Until > 0 && cand.After(until) {
				break
			}
			out = append(out, cand.UTC().UnixMilli())
		}
	default:
		return nil, fmt.Errorf("unsupported recurrence freq: %s", freq)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("recurrence produced zero occurrences")
	}
	return out, nil
}

func daysInMonth(year int, month time.Month) int {
	return time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

// calendarDaysBetween counts whole calendar days from a to b in the same location (DST-safe).
func calendarDaysBetween(a, b time.Time) int {
	a = time.Date(a.Year(), a.Month(), a.Day(), 0, 0, 0, 0, a.Location())
	b = time.Date(b.Year(), b.Month(), b.Day(), 0, 0, 0, 0, b.Location())
	if b.Before(a) {
		return -1
	}
	n := 0
	for a.Before(b) {
		a = a.AddDate(0, 0, 1)
		n++
		if n > scheduleMaxHorizonDays+366 {
			break
		}
	}
	return n
}
