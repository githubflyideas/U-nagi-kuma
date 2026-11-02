package app

import (
	"context"
	"errors"
	"testing"
)

func TestWorkspaceNotificationUnsupportedRejected(t *testing.T) {
	for _, body := range []string{
		"one IN CHANNEL log retries=2\ndefault IN POLICY chain=one\n",
		"one IN CHANNEL log\ntwo IN CHANNEL log\ndefault IN POLICY chain=one,two\n",
		"one IN CHANNEL log rate=1/m\ndefault IN POLICY chain=one\n",
		"one IN CHANNEL log\ndefault IN POLICY chain=one notify-degraded=yes\n",
	} {
		t.Run(body, func(t *testing.T) {
			a, s, _ := workspaceConfigFixture(t)
			cfg, e := a.ApplyWorkspaceConfig(s, []byte(runtimeSOA+"check IN HTTP http://127.0.0.1:1/\n"+body), 0)
			if e != nil {
				t.Fatalf("fixture publication %v", e)
			}
			out, e := a.RunWorkspaceChecks(context.Background(), s.TenantID, s.WorkspaceID, cfg.Generation)
			if !errors.Is(e, ErrWorkspaceNotificationUnsupported) || out.NotificationStatus != "blocked_unsupported_notifications" {
				t.Fatalf("unsupported accepted: %+v %v", out, e)
			}
			events, e := a.WorkspaceEvents(s, 0, 100)
			if e != nil || len(events) != 0 {
				t.Fatalf("unsupported executed %v %v", events, e)
			}
		})
	}
}
