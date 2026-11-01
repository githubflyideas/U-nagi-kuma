package app

import (
	"fmt"
	"testing"
	"time"

	"github.com/githubflyideas/unagikuma/internal/oncall"
)

var benchmarkCalendarResult []CalendarSegment
var benchmarkShiftsResult []oncall.Shift

// BenchmarkCalendarWindow isolates calendar generation from HTTP, SQLite and auth.
// A daily UTC rotation yields exactly one segment per day at its handoff.
func BenchmarkCalendarWindow(b *testing.B) {
	r, err := oncall.Parse(map[string]string{"rotate": "daily", "handoff": "09:00", "start": "2026-01-01", "tz": "UTC"}, []string{"a", "b", "c", "d"})
	if err != nil {
		b.Fatal(err)
	}
	from := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	for _, days := range []int{1, 28, 92} {
		b.Run(fmt.Sprintf("days_%d", days), func(b *testing.B) {
			to := from.AddDate(0, 0, days)
			if got := calendar(r, "rotation", nil, from, to, time.UTC); len(got) != days {
				b.Fatalf("segments: got %d, want %d", len(got), days)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				benchmarkCalendarResult = calendar(r, "rotation", nil, from, to, time.UTC)
			}
		})
	}
	// The production calendar eagerly asks Next for 4000 shifts before
	// breaking at the requested date. These controls quantify that cost;
	// Next(29) is NOT a proposed general replacement (DST/rotation lengths).
	for _, count := range []int{29, 4000} {
		b.Run(fmt.Sprintf("next_%d", count), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				benchmarkShiftsResult = r.Next(from, count, time.UTC)
			}
		})
	}
}
