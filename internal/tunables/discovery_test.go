package tunables

import (
	"testing"
	"time"
)

func TestDiscoveryInterval(t *testing.T) {
	for _, value := range []string{"1m", "5m", "1h", "24h"} {
		t.Run(value, func(t *testing.T) {
			ts, err := New(map[string]string{"cluster.discovery-interval": value})
			if err != nil {
				t.Fatalf("discovery interval must be independently configurable: %v", err)
			}
			want, _ := ParseDuration(value)
			if got := ts.Duration("cluster.discovery-interval"); got != want {
				t.Fatalf("got %v want %v", got, want)
			}
			if ts.Duration("cluster.ping-interval") != 20*time.Second || ts.Duration("cluster.vote-grace") != 5*time.Second {
				t.Fatal("discovery changed heartbeat or vote timing")
			}
		})
	}
	for _, value := range []string{"0s", "-1s", "999h", "no"} {
		if _, err := New(map[string]string{"cluster.discovery-interval": value}); err == nil {
			t.Errorf("accepted invalid discovery interval %q", value)
		}
	}
}
