package admin

import (
	"context"
	"testing"
	"time"
)

type fakeNewUsers struct {
	m          map[string]int64
	gotS, gotE time.Time
	called     bool
}

func (f *fakeNewUsers) CountNewUsersEveryday(_ context.Context, s, e time.Time) (map[string]int64, int64, error) {
	f.called, f.gotS, f.gotE = true, s, e
	var t int64
	for _, v := range f.m {
		t += v
	}
	return f.m, t, nil
}

func TestComputeNewUserCount(t *testing.T) {
	start := int64(1791820800000) // 2026-10-13 00:00 +08:00
	f := &fakeNewUsers{m: map[string]int64{"2026-10-14": 2, "2026-10-16": 5}}
	resp, err := computeNewUserCount(context.Background(), f, &NewUserCountReq{Start: start, End: start + 4*86400000})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.DateCount) != 4 || resp.DateCount["2026-10-13"] != 0 || resp.DateCount["2026-10-14"] != 2 ||
		resp.DateCount["2026-10-15"] != 0 || resp.DateCount["2026-10-16"] != 5 {
		t.Fatalf("bad zero fill: %v", resp.DateCount)
	}
	if resp.Total != 7 {
		t.Fatalf("total want 7 got %d", resp.Total)
	}
	if f.gotS.UnixMilli() != start || f.gotE.UnixMilli() != start+4*86400000 {
		t.Fatal("range not passed through")
	}
}

func TestComputeNewUserCountBadRange(t *testing.T) {
	f := &fakeNewUsers{}
	for _, r := range []*NewUserCountReq{{Start: 10, End: 5}, {Start: -1, End: 5}, {Start: 0, End: 5000 * 86400000}} {
		if _, err := computeNewUserCount(context.Background(), f, r); err == nil {
			t.Fatalf("expected error for %+v", r)
		}
	}
	if f.called {
		t.Fatal("store must not be queried for invalid ranges")
	}
	resp, err := computeNewUserCount(context.Background(), f, &NewUserCountReq{Start: 5, End: 5})
	if err != nil || resp.Total != 0 || len(resp.DateCount) != 0 {
		t.Fatalf("empty range: %v %v", resp, err)
	}
}

// imLike mimics the IM /statistics/user/register response: IM's total is ALL users, and an older IM may
// omit empty days or return keys outside the range. Only in-range days must be counted.
type imLike struct{}

func (imLike) CountNewUsersEveryday(_ context.Context, _, _ time.Time) (map[string]int64, int64, error) {
	return map[string]int64{"2026-10-12": 9, "2026-10-14": 2, "2026-10-17": 4}, 1000, nil
}

func TestComputeNewUserCountIgnoresIMTotalAndOutOfRange(t *testing.T) {
	start := int64(1791820800000) // 2026-10-13 00:00 +08:00
	resp, err := computeNewUserCount(context.Background(), imLike{}, &NewUserCountReq{Start: start, End: start + 4*86400000})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Total != 2 || len(resp.DateCount) != 4 || resp.DateCount["2026-10-14"] != 2 {
		t.Fatalf("got total=%d %v", resp.Total, resp.DateCount)
	}
	if _, ok := resp.DateCount["2026-10-12"]; ok {
		t.Fatal("out-of-range day must be dropped")
	}
}
