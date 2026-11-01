package ratelimit

import (
	"testing"
	"time"
)

func TestKeyedCheckDoesNotConsumeOrAllocate(t *testing.T) {
	k := NewKeyed(1, 1, 2)
	now := time.Now()
	for i := 0; i < 3; i++ {
		if ok, _ := k.Check("a", now); !ok {
			t.Fatal("check consumed quota")
		}
	}
	if k.Len() != 0 {
		t.Fatal("check allocated an unvalidated identity")
	}
	k.Allow("a", now)
	if ok, wait := k.Check("a", now); ok || wait != time.Second {
		t.Fatalf("exhausted: %v %s", ok, wait)
	}
	if ok, _ := k.Check("a", now.Add(time.Second)); !ok {
		t.Fatal("no refill")
	}
}
