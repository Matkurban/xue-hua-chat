// Package statsday holds the calendar-day helpers shared by the dashboard statistics.
package statsday

import (
	"time"

	"gitee.com/matkurban/openimsdk-tools/errs"
)

// Location is Asia/Shanghai (fixed +08:00, no DST, no tzdata dependency). It must match the
// "+08:00" timezone used in the Mongo $dateToString of the statistics aggregations.
var Location = time.FixedZone("Asia/Shanghai", 8*3600)

// MaxDays bounds the zero-filled range so a bogus request cannot allocate unbounded maps.
const MaxDays = 3660

// FillDays returns a map with every Asia/Shanghai calendar day in [startMs, endMs) as "YYYY-MM-DD",
// using values from src and 0 for days without data. Days outside the range present in src are dropped.
func FillDays(startMs, endMs int64, src map[string]int64) (map[string]int64, error) {
	out := make(map[string]int64)
	if endMs <= startMs {
		return out, nil
	}
	first := time.UnixMilli(startMs).In(Location)
	last := time.UnixMilli(endMs - 1).In(Location)
	day := time.Date(first.Year(), first.Month(), first.Day(), 0, 0, 0, 0, Location)
	lastDay := time.Date(last.Year(), last.Month(), last.Day(), 0, 0, 0, 0, Location)
	for n := 0; !day.After(lastDay); n++ {
		if n >= MaxDays {
			return nil, errs.ErrArgs.WrapMsg("time range too large")
		}
		k := day.Format("2006-01-02")
		out[k] = src[k]
		day = day.AddDate(0, 0, 1)
	}
	return out, nil
}
