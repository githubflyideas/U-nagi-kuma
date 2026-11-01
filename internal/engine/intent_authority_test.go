package engine

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/githubflyideas/unagikuma/internal/notify"
)

type rejectingIntentSink struct{ *memSink }

func (s *rejectingIntentSink) SaveIntent(*Intent) error {
	return errors.New("runtime authority rejected")
}

func TestRestoredRejectedIntentNeverDispatches(t *testing.T) {
	sent := 0
	e := New(Deps{Sink: &rejectingIntentSink{newMemSink()}, Send: func(context.Context, notify.Channel, *notify.Message) notify.Attempt { sent++; return notify.Attempt{} }})
	e.cfg.Policies = map[string]PolicySpec{"default": {Name: "default", Chain: []string{"log"}}}
	e.cfg.Channels = map[string]ChannelSpec{"log": {Channel: notify.Channel{Name: "log", Kind: "log"}}}
	in := &Intent{ID: "restored", State: IntentPending, Policy: "default", CreatedAt: time.Now(), DueAt: time.Now(), Delivery: NewDelivery([]string{"log"}, DeliverFallback)}
	e.Restore(nil, nil, []*Intent{in})
	var wg sync.WaitGroup
	e.dispatchIntents(context.Background(), &wg)
	wg.Wait()
	if sent != 0 {
		t.Fatal("restored rejected intent was sent")
	}
}

func TestRejectedIntentNeverDispatches(t *testing.T) {
	sent := 0
	e := New(Deps{Sink: &rejectingIntentSink{newMemSink()}, Send: func(context.Context, notify.Channel, *notify.Message) notify.Attempt { sent++; return notify.Attempt{} }})
	e.cfg.Policies = map[string]PolicySpec{"default": {Name: "default", Chain: []string{"log"}}}
	e.cfg.Channels = map[string]ChannelSpec{"log": {Channel: notify.Channel{Name: "log", Kind: "log"}}}
	in := &Intent{Policy: "default", CreatedAt: time.Now(), DueAt: time.Now()}
	if e.enqueue(in) {
		t.Error("rejected durable intent reported queued")
	}
	if len(e.intents) != 0 {
		t.Fatal("rejected intent retained for dispatch")
	}
	var wg sync.WaitGroup
	e.dispatchIntents(context.Background(), &wg)
	wg.Wait()
	if sent != 0 {
		t.Fatal("rejected intent sent")
	}
}
