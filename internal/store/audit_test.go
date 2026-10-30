package store

import (
	"testing"
	"time"
)

func auditN(t *testing.T, s *Store, c *clock, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		c.t = c.t.Add(time.Minute)
		if err := s.Audit(AuditEntry{Actor: "alice", Action: "monitor.update", Target: "web", Addr: "10.0.0.1",
			Detail: map[string]any{"before": "a", "after": "b", "i": i}}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestAuditChainVerifies(t *testing.T) {
	s, c, _ := open(t)
	auditN(t, s, c, 5)
	s.Audit(AuditEntry{Actor: "bob", Action: "user.delete", Target: "carol"})
	r, err := s.VerifyAudit()
	if err != nil || !r.OK || r.Entries != 6 || r.Head == "" {
		t.Fatalf("%v %+v", err, r)
	}
	items, _ := s.AuditLog(AuditQuery{Risk: "high"})
	if len(items) != 1 || items[0].Action != "user.delete" {
		t.Fatalf("high-risk filter: %+v", items)
	}
	if !s.AuditContains(r.Head) {
		t.Fatal("head not found")
	}
}

// Entries cannot be updated; an edit made around that (the trigger
// dropped), a removal or an insertion is found and located.
func TestAuditTamperDetected(t *testing.T) {
	s, c, _ := open(t)
	auditN(t, s, c, 6)
	if _, err := s.core.Exec(`UPDATE audit SET actor='mallory' WHERE id=3`); err == nil {
		t.Fatal("an audit entry could be updated")
	}
	if _, err := s.core.Exec(`DROP TRIGGER audit_append_only`); err != nil {
		t.Fatal(err)
	}
	s.core.Exec(`UPDATE audit SET actor='mallory' WHERE id=3`)
	r, _ := s.VerifyAudit()
	if r.OK || r.BadID != 3 {
		t.Fatalf("edit not found: %+v", r)
	}
	s.core.Exec(`UPDATE audit SET actor='alice' WHERE id=3`)
	if r, _ := s.VerifyAudit(); !r.OK {
		t.Fatalf("restored entry: %+v", r)
	}
	s.core.Exec(`DELETE FROM audit WHERE id=4`)
	if r, _ := s.VerifyAudit(); r.OK || r.BadID != 5 {
		t.Fatalf("removal not found: %+v", r)
	}
	s.core.Exec(`INSERT INTO audit(id,at,actor,action,prev,hash) SELECT 4,at,'x','user.create',prev,hash FROM audit WHERE id=3`)
	if r, _ := s.VerifyAudit(); r.OK {
		t.Fatalf("forged entry not found: %+v", r)
	}
}

// Old entries leave after retention.audit; the chain then starts at the
// anchor and still verifies, and new entries continue it.
func TestAuditPruneKeepsChain(t *testing.T) {
	s, c, _ := open(t)
	auditN(t, s, c, 4)
	before, _ := s.VerifyAudit()
	c.t = c.t.Add(10 * 24 * time.Hour)
	auditN(t, s, c, 2)
	if err := s.Prune(Retention{Raw: time.Hour, Agg5m: time.Hour, Agg1h: time.Hour, Events: time.Hour, Audit: 24 * time.Hour}); err != nil {
		t.Fatal(err)
	}
	r, _ := s.VerifyAudit()
	if !r.OK || r.Entries != 2 {
		t.Fatalf("after prune: %+v", r)
	}
	if !s.AuditContains(before.Head) {
		t.Fatal("the last pruned hash is the anchor")
	}
	auditN(t, s, c, 1)
	if r, _ := s.VerifyAudit(); !r.OK || r.Entries != 3 {
		t.Fatalf("continued: %+v", r)
	}
}

// Entries written before the chain existed are chained at open.
func TestAuditLegacyChained(t *testing.T) {
	s, _, dir := open(t)
	s.core.Exec(`INSERT INTO audit(at,actor,action) VALUES(1,'old','config.apply'),(2,'old','user.create')`)
	s.Close()
	s2, err := Open(dir, Options{CommitInterval: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	if r, _ := s2.VerifyAudit(); !r.OK || r.Entries != 2 {
		t.Fatalf("%+v", r)
	}
}
