# Cluster readiness: exercised behavior, not an HA certification

Evidence: `test-results/cluster-readiness.json` and its referenced raw Go JSON logs. Audited working tree based on `9e7b8c1d856b11bfb5541134ef8230875986fa27`, Windows/amd64, Go 1.27.3. Other workers' changes were present. No production instance was changed; targets/listeners/stores were isolated, and no external notifications were sent.

## Result

- **Distributed monitoring works in the exercised local integration paths. Automatic highly available controllers are not implemented.**
- Final cluster/app/server/store/engine suites passed: **313 test pass events, including subtests**. Configuration-ack regression passed 10 consecutive runs.
- Actual existing two-process `e2e.TestFailover` **failed on Windows** after promotion requested a restart. The cause is an unmet deployment prerequisite: `cmd/unagikuma/reexec_windows.go` exits with code 3 and expects a service supervisor; the test launches unsupervised processes. A separate real-app integration explicitly restarted the standby and verified restoration. This does **not** validate Windows service recovery or Linux/container failover.
- Race testing was attempted but blocked: CGO requires `gcc`, absent from this environment. No race-clean claim.

## Supported roles and synchronization

| Role | Actual behavior |
|---|---|
| Master/controller | Owns configuration, users/settings, persistent state, scheduling, round aggregation and normal notifications. Can also probe. |
| CLI probe agent | Receives only assigned active monitor specs with resolved secrets, per-monitor definition hashes, anchors/tunables and controller-address list. Reports votes, configuration status, script inventory and local health. Monitor configuration stays in memory; there is no agent result spool or autonomous monitor scheduling during disconnection. |
| Joined server | Runs the same probe agent while keeping membership/token and observed terms/addresses on disk. Joining removes its local monitors/maintenance records; optional imports are added **paused**, with unsupported references dropped. It is not automatically a standby. |
| Explicit standby | A joined server whose master NODE has `standby=https://...`. It may probe and additionally download encrypted whole-state archives. Promotion is an operator/master request followed by restart/restore, not an election. A bare CLI agent does not implement the App state-download/restore callbacks. |

Verified standby archive manifest: `core.db`, `monitors.bind`, `installation`, `unagikuma.key`, `cluster-ca.crt`, `cluster-ca.key`, `cluster.crt`, `cluster.key`. `license.json` is included by the backup code when present, but was not present in the fixture. The restored core setting, CA fingerprint, term and controller membership state were checked after restart.

**`history.db` is NOT synchronized:** `internal/app/standby.go:writeState` leaves `backup.Options.History` false. This was confirmed by decrypting/restoring the actual downloaded archive. Promotion removes the standby's old databases before restore. Historical results therefore cannot be promised to survive takeover. Script binaries, arbitrary directory files and environment-provided secrets are not general filesystem replication. Provision them separately. Ordinary probe nodes were denied archive download.

Snapshot delivery defaults to every **5 minutes** and after configuration changes; failed copies retry up to three times (default **30 seconds**). This is not WAL streaming, synchronous replication or a zero-data-loss guarantee. Individual DB backups do not establish a cross-DB/config/key transaction. Snapshot freshness and restore readiness are operational prerequisites.

## Enrollment, acknowledgements and convergence

- Join codes contain the server address, pinned cluster-CA fingerprint, one-time secret and displayed expiry. They are bearer credentials: transfer securely; the encoding is not encryption. Server-side hashed code storage and conditional SQL consumption are authoritative. Tests verified replay rejection over pinned TLS, one winner among 16 concurrent consumers, and rejection at the exact stored expiry boundary. Default TTL is 1 hour.
- Sessions require TLS and a node-bound bearer token, **not mTLS**. Existing tests exercised incorrect CA, token, requested node and undeclared node rejection, plus active-session token revocation. The master rechecks tokens on ping. Avoid insecure-agent mode.
- **One production bug fixed, RED/GREEN:** master previously exposed any received status serial as acknowledged and ignored its hash. `internal/cluster/master.go` now requires both serial and hash to match the queued configuration; mismatches expose serial 0 while retaining health/script reports. Two failing cases (wrong hash and wrong serial) are preserved in RED logs. This is protocol consistency checking, not proof that a compromised trusted agent executed a configuration.
- Config updates coalesce to the latest pending configuration; unchanged hashes are skipped. Reconnection receives the current snapshot. A forced-disconnect test verified serial 9 convergence, a real HTTP result for the new monitor, and abstention for the removed monitor. Results lost with a session are not replayed from durable storage.
- Per-monitor definition mismatches abstain with the agent's actual hash. The engine ignores unknown rounds, wrong monitor identities and duplicate/completed-round votes; a conflicting duplicate did not overwrite the accepted verdict. Transport itself forwards votes with the authenticated node identity; deduplication is in `internal/engine/cluster.go`, not a replicated log.
- Protocol-level tests exercised alternate-controller address selection, higher observed terms, and rejection/retirement of an older-term controller. Duplicate sessions with the same token replace one another; two running copies can repeatedly reconnect and churn.

## Built-in health is not controller consensus

Master/agent ping-pong measures RTT and estimated clock skew, carries agent health/status and detects silent sessions after three intervals. Default ping is **20 seconds**; the silent-peer test used **200 ms**, observing expiry around three intervals. Socket-close detection can be faster. The configured node-outage alert delay is separately **60 seconds** by default. Time-sensitive votes can abstain for excessive clock skew (default threshold **1 second**).

This is a master-agent star, not an automatic all-pairs mesh health check or leader-election quorum. Optional CLI agent emergency notifications and the external heartbeat URL are separate opt-in mechanisms, not enabled external safety monitors by default. Emergency notification tests use a recording sender, never real delivery.

## Measured healthy onboarding

Three final-run samples reached the first master-observed successful remote HTTP result in **1027.6761, 1032.5632 and 1037.4452 ms** (min/median/max shown in ascending order; raw per-sample values and enrollment/ack times are in JSON).

Measurement starts immediately before `App.JoinCluster` and ends when `Engine.Row` contains the joining node's successful vote. These are real SQLite, TLS, application-engine and loopback HTTP integration measurements, **not** fresh-machine installation times or WAN guarantees. Prerequisites:

- Both applications/listeners already running; code already issued; download, setup, installation and service start excluded.
- Reachable loopback target, no induced delay/loss/proxy; 10 ms observation polling.
- 1-second round interval, `engine.first-round-delay=0s`, `cluster.ping-interval=200ms`, `localnet.enabled=off`.
- Fixture explicitly uses `api.login-rate=100/s`, **not** the production default.

The actual subprocess test at default login rate observed **three HTTP 429 state-download failures**, then success roughly 15 seconds later with a test retry interval of 5 seconds. Join/connect/state/retire share the per-IP login limiter (`internal/server/h_agent.go`, `h_cluster.go`), default **5/minute, burst 3**. NAT/shared-IP agents and rapid administration can compete for it. Do not promise seconds-to-onboard for the default deployment or arbitrary networks; do not globally weaken authentication rate limits as a production fix.

## Remaining rollout blockers / risks

1. **No partition fencing or automatic leader election.** Manual promotion can leave isolated controllers simultaneously active. Terms and signed retirement converge when communication resumes; they do not guarantee single-writer safety. Physically/service-level fence the previous controller before disaster promotion.
2. **Incomplete snapshot scope and unbounded data-loss window.** History is omitted; failed syncs can leave arbitrarily old snapshots. Establish explicit RPO and externally verified backups.
3. **Handover has no readiness acknowledgement.** Source review: `App.HandOver` queues Promote, then steps down after a fixed default 20 seconds; `App.Promote` may fall back to the last snapshot after refresh failure. Successful restore/start is not acknowledged before old-master retirement. Fix requires `internal/app` ownership.
4. **Deployment-specific restart must be demonstrated.** Windows requires service recovery. The unsupervised multi-process test fails as described; the explicit restart integration passes. Service-install permissions/recovery, Linux/container restart, large archives and storage failures remain unqualified.
5. **Admission contention and trust boundaries.** Separate authenticated state traffic from brute-force login protection without weakening credential checks (server ownership). Protect node tokens and standby copies: standbys receive core secrets and CA private keys. Higher-term announcements from authenticated agents are trusted; this is not a Byzantine-safe protocol.
6. Race-detector coverage, sustained concurrency/load, lossy/partitioned networks and rolling upgrades between actual released binaries remain unverified.

No fixes outside cluster ownership were made. These findings do not justify a “production ready” or automatic HA release claim.

## Reproduce

From the repository root with the supplied Go toolchain on PATH (Windows Git Bash):

```bash
export PATH="C:/Users/fengyongxiao.LAPTOP-05HDB6FR/toolchains/go/bin:$PATH"
# Keep Go-created temporary fixture files under the Hermes scratch directory.
export TEMP="$TMPDIR" TMP="$TMPDIR"
go test ./internal/cluster -run '^TestReadiness' -count=1 -timeout=90s -v
go test ./internal/cluster -run '^TestReadinessConfigurationAckRequiresMatchingHash$' -count=10 -timeout=30s
go test ./internal/cluster ./internal/app ./internal/server ./internal/store ./internal/engine -count=1 -timeout=180s -json > test-results/cluster-related.jsonl
# Expected to fail on unsupervised Windows; requires a supervisor-aware harness.
go test -tags e2e ./e2e -run '^TestFailover$' -count=1 -json > test-results/cluster-failover.jsonl
# Requires a configured Windows C compiler; blocked on the audited host.
CGO_ENABLED=1 go test -race ./internal/cluster -run '^TestReadiness' -count=1 -timeout=120s
```

Tests added: `internal/cluster/readiness_integration_test.go` and `readiness_protocol_test.go`. The JSON evidence indexes immutable captured run hashes; rerunning a raw log requires refreshing its evidence hash/timing summary rather than treating old measurements as new.
