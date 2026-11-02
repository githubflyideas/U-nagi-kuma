package app

import (
	"testing"
)

func TestWorkspacePublicationRejectsUnsupportedNotifications(t *testing.T) {
	a, owner, _ := workspaceConfigFixture(t)
	initial, err := a.ApplyWorkspaceConfig(owner, []byte(workspaceBody), 0)
	if err != nil {
		t.Fatal(err)
	}
	raw := []byte(workspaceBody + "alerts IN CHANNEL log retries=2\ndefault IN POLICY chain=alerts\n")
	if _, err = a.PreviewWorkspaceConfig(owner, raw, initial.Revision); err == nil {
		t.Fatal("preview accepted unsupported notification retries")
	}
	if _, err = a.ApplyWorkspaceConfig(owner, raw, initial.Revision); err == nil {
		t.Fatal("apply accepted unsupported notification retries")
	}
	after, err := a.WorkspaceConfigState(owner)
	if err != nil || after != initial {
		t.Fatalf("rejected publication changed live revision: %+v %v", after, err)
	}
	if _, _, err = a.RestoreWorkspaceConfig(owner.TenantID, owner.WorkspaceID, initial.Generation); err != nil {
		t.Fatalf("rejection invalidated existing authority: %v", err)
	}
}
