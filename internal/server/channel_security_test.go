package server

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRejectedChannelSavePreservesCredential(t *testing.T) {
	e := newEnv(t, testBind, nil)
	e.setup()
	save := func(token, kind string, headers ...string) int {
		return e.do("POST", "/api/v1/channels", map[string]any{"name": "pager", "kind": kind, "opts": map[string]string{"token": token, "chat": "123"}}, nil, headers...)
	}
	if code := save("original-credential", "telegram"); code != 200 {
		t.Fatalf("create: %d", code)
	}
	ref := e.app.Config().Config.FindChannel("pager").Opts["token"]
	for _, tc := range []struct {
		name, kind string
		headers    []string
	}{
		{"stale", "telegram", []string{"If-Match", `"stale"`}},
		{"invalid", "nonexistent", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if code := save("rejected-credential", tc.kind, tc.headers...); code < 400 {
				t.Fatalf("accepted: %d", code)
			}
			got, err := e.app.Resolver()(ref)
			if err != nil || got != "original-credential" {
				t.Fatalf("rejected save changed live credential: %q, %v", got, err)
			}
			secrets, err := e.app.SecretStore().List()
			if err != nil || len(secrets) != 1 {
				t.Errorf("rejected save left staged secrets: %d, %v", len(secrets), err)
			}
		})
	}
	oldTag := e.srv.recordETags()["pager"]
	if code := save("replacement-credential", "telegram", "If-Match", oldTag); code != 200 {
		t.Fatalf("valid credential update: %d", code)
	}
	if e.srv.recordETags()["pager"] == oldTag {
		t.Error("credential update did not change record ETag")
	}
	if code := save("***", "telegram"); code != 200 {
		t.Fatalf("masked credential update: %d", code)
	}
	currentRef := e.app.Config().Config.FindChannel("pager").Opts["token"]
	if got, err := e.app.Resolver()(currentRef); err != nil || got != "replacement-credential" {
		t.Fatalf("masked credential not preserved: %q, %v", got, err)
	}
	if got, err := e.app.Resolver()(ref); err != nil || got != "original-credential" {
		t.Fatalf("old config reference no longer resolves: %q, %v", got, err)
	}
}

func TestChannelTestDoesNotExposeResolvedSecrets(t *testing.T) {
	e := newEnv(t, testBind, nil)
	e.setup()
	for _, secret := range []string{"private-provider-credential", "xy"} {
		if err := e.app.SecretStore().Put("test_credential", secret, nil); err != nil {
			t.Fatal(err)
		}
		var out json.RawMessage
		code := e.do("POST", "/api/v1/channels/test", map[string]any{"name": "probe", "kind": "webhook", "opts": map[string]string{"preset": "${secret:test_credential}"}}, &out)
		if code != 200 {
			t.Fatalf("status = %d: %s", code, out)
		}
		if strings.Contains(string(out), secret) {
			t.Errorf("resolved secret leaked: %s", out)
		}
	}
}
