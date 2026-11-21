package cache

import (
	"math"
	"testing"
)

func TestSplitAmountsExactAndPositive(t *testing.T) {
	cases := []struct {
		total float64
		count int32
	}{
		{0.01, 1},
		{0.02, 2},
		{0.05, 3},
		{0.08, 5},
		{0.16, 10},
		{1.00, 3},
		{10.00, 7},
		{100.00, 100},
		{99.99, 37},
	}

	for _, luck := range []bool{false, true} {
		for _, tc := range cases {
			for iter := 0; iter < 20; iter++ {
				got := splitAmounts(tc.total, tc.count, luck)
				if int32(len(got)) != tc.count {
					t.Fatalf("luck=%v total=%.2f count=%d: got len=%d", luck, tc.total, tc.count, len(got))
				}
				var sumCents int64
				for idx, a := range got {
					cents := int64(math.Round(a * 100))
					if cents < 1 {
						t.Fatalf("luck=%v total=%.2f count=%d: slot %d has non-positive amount %.2f", luck, tc.total, tc.count, idx, a)
					}
					sumCents += cents
				}
				wantCents := int64(math.Round(tc.total * 100))
				if sumCents != wantCents {
					t.Fatalf("luck=%v total=%.2f count=%d: sumCents=%d != wantCents=%d", luck, tc.total, tc.count, sumCents, wantCents)
				}
			}
		}
	}
}
