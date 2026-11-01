package server

import (
	"net/http/httptest"
	"testing"
	"time"
)

func TestHistoryShortRanges(t *testing.T) {
	for key, want := range map[string]time.Duration{"5m": 5 * time.Minute, "15m": 15 * time.Minute, "30m": 30 * time.Minute, "12h": 12 * time.Hour, "24h": 24 * time.Hour} {
		t.Run(key, func(t *testing.T) {
			gotKey, got := rangeOf(httptest.NewRequest("GET", "/?range="+key, nil))
			if gotKey != key || got != want {
				t.Fatalf("range %s: got %s %s, want %s", key, gotKey, got, want)
			}
		})
	}
}
