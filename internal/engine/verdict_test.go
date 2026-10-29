package engine

import (
	"testing"

	"github.com/githubflyideas/unagikuma/internal/probe"
)

const (
	ok   = probe.VoteOK
	fail = probe.VoteFail
	abst = probe.VoteAbstain
)

func nv(node string, vote probe.Vote) NodeVote {
	return NodeVote{Node: node, Online: true, Present: true, Vote: vote}
}

func offline(node string) NodeVote { return NodeVote{Node: node} }

func localAbstain(node, reason string) NodeVote {
	return NodeVote{Node: node, Online: true, Present: true, Vote: abst, Reason: reason}
}

func TestQuorumOf(t *testing.T) {
	cases := []struct {
		spec string
		n, q int
	}{
		{"majority", 1, 1}, {"majority", 2, 2}, {"majority", 3, 2}, {"majority", 4, 3}, {"majority", 5, 3},
		{"all", 3, 3}, {"1", 3, 1}, {"9", 3, 3}, {"60%", 5, 3}, {"50%", 4, 2}, {"34%", 3, 2}, {"", 3, 2},
	}
	for _, c := range cases {
		if q, err := QuorumOf(c.spec, c.n); err != nil || q != c.q {
			t.Errorf("QuorumOf(%q,%d)=%d,%v want %d", c.spec, c.n, q, err, c.q)
		}
	}
	if _, err := QuorumOf("abc", 3); err == nil {
		t.Error("bad spec accepted")
	}
}

// Dynamic quorum examples (plus quorum=all).
func TestDynamicQuorum(t *testing.T) {
	cases := []struct {
		name  string
		votes []NodeVote
		spec  string
		qeff  int
		kind  VerdictKind
	}{
		{"3/Q2 all healthy, 2 fail", []NodeVote{nv("a", fail), nv("b", fail), nv("c", ok)}, "majority", 2, VerdictCritical},
		{"3/Q2 one offline, 1 fail", []NodeVote{nv("a", fail), nv("b", ok), offline("c")}, "majority", 2, VerdictPartial},
		{"3/Q2 one offline, 2 fail", []NodeVote{nv("a", fail), nv("b", fail), offline("c")}, "majority", 2, VerdictCritical},
		{"3/Q2 two offline, 1 fail", []NodeVote{nv("a", fail), offline("b"), offline("c")}, "majority", 1, VerdictCritical},
		{"2/Q2 one offline, 1 fail", []NodeVote{nv("a", fail), offline("b")}, "majority", 1, VerdictCritical},
		{"5/Q3 two offline", []NodeVote{nv("a", fail), nv("b", fail), nv("c", ok), offline("d"), offline("e")}, "3", 2, VerdictCritical},
		{"4/all one offline: all of the rest", []NodeVote{nv("a", fail), nv("b", fail), nv("c", ok), offline("d")}, "all", 3, VerdictPartial},
		{"single node fail", []NodeVote{nv("m", fail)}, "majority", 1, VerdictCritical},
		{"single node ok", []NodeVote{nv("m", ok)}, "majority", 1, VerdictOK},
	}
	for _, c := range cases {
		v := Decide(c.votes, QuorumRule{Spec: c.spec})
		if v.QEff != c.qeff || v.Kind != c.kind {
			t.Errorf("%s: qeff=%d kind=%s want %d/%s", c.name, v.QEff, v.Kind, c.qeff, c.kind)
		}
	}
}

func TestUnknownReasonsPriority(t *testing.T) {
	// Single machine, uplink down: local_network, never no_healthy_nodes.
	v := Decide([]NodeVote{{Node: "m", Online: true, Excluded: true, Present: true, Vote: abst, Reason: probe.ReasonLocalNetwork}}, QuorumRule{})
	if v.Kind != VerdictUnknown || v.UnknownReason != UnknownLocalNetwork {
		t.Errorf("single localnet: %+v", v)
	}
	// Online nodes all abstaining for local reasons.
	v = Decide([]NodeVote{localAbstain("a", probe.ReasonPrivilege), localAbstain("b", probe.ReasonPlatform)}, QuorumRule{})
	if v.UnknownReason != UnknownAllAbstain {
		t.Errorf("all abstain: %+v", v)
	}
	// All offline.
	v = Decide([]NodeVote{offline("a"), offline("b")}, QuorumRule{})
	if v.UnknownReason != UnknownNoHealthyNodes {
		t.Errorf("offline: %+v", v)
	}
	// Healthy node exists but its vote did not arrive in time.
	v = Decide([]NodeVote{{Node: "a", Online: true}, nv("b", fail)}, QuorumRule{OnNodeLoss: "unknown"})
	if v.Kind != VerdictUnknown || v.UnknownReason != UnknownInsufficient {
		t.Errorf("insufficient: %+v", v)
	}
}

func TestTwoNodesWithLocalAbstention(t *testing.T) {
	// one node fails, the other abstains locally (no ICMP privilege)
	// → the failing node decides alone in both modes.
	for _, spec := range []string{"majority", "1"} {
		v := Decide([]NodeVote{nv("master", fail), localAbstain("agent", probe.ReasonPrivilege)}, QuorumRule{Spec: spec})
		if v.Kind != VerdictCritical || v.QEff != 1 {
			t.Errorf("%s: %+v", spec, v)
		}
	}
}

func TestWeakFloor(t *testing.T) {
	// majority on 3 nodes is already 2.
	v := Decide([]NodeVote{nv("a", fail), nv("b", ok), nv("c", ok)}, QuorumRule{Weak: true})
	if v.QEff != 2 || v.Kind != VerdictPartial {
		t.Errorf("weak majority: %+v", v)
	}
	// percentage resolving to 1 is lifted to 2.
	v = Decide([]NodeVote{nv("a", fail), nv("b", ok), nv("c", ok)}, QuorumRule{Spec: "10%", Weak: true})
	if v.QEff != 2 {
		t.Errorf("weak percent: %+v", v)
	}
	// explicit integer (two-node avoid-misses mode) is respected.
	v = Decide([]NodeVote{nv("a", fail), nv("b", ok)}, QuorumRule{Spec: "1", Weak: true})
	if v.QEff != 1 || v.Kind != VerdictCritical {
		t.Errorf("weak explicit: %+v", v)
	}
	// a single healthy node decides alone.
	v = Decide([]NodeVote{nv("a", fail), offline("b")}, QuorumRule{Weak: true})
	if v.QEff != 1 || v.Kind != VerdictCritical {
		t.Errorf("weak single: %+v", v)
	}
}

func TestOnInsufficient(t *testing.T) {
	v := Decide([]NodeVote{nv("a", fail), {Node: "b", Online: true}, {Node: "c", Online: true}}, QuorumRule{OnNodeLoss: "unknown", OnInsufficient: "critical"})
	if v.Kind != VerdictCritical {
		t.Errorf("on-insufficient: %+v", v)
	}
}

func TestSummary(t *testing.T) {
	if s := Decide([]NodeVote{nv("m", fail)}, QuorumRule{}).Summary(); s != "" {
		t.Errorf("single node summary must be empty, got %q", s)
	}
	if s := Decide([]NodeVote{nv("a", fail), nv("b", fail), nv("c", ok)}, QuorumRule{}).Summary(); s == "" {
		t.Error("cluster summary missing")
	}
}

func TestQuorumRejectsNaN(t *testing.T) {
	for _, s := range []string{"nan%", "NaN%", "inf%", "-inf%", "0%", "101%"} {
		if _, _, err := parseQuorum(s); err == nil {
			t.Errorf("accepted quorum %q", s)
		}
	}
}
