# Cluster admission: authenticated workload isolation

This note supersedes only the **shared login-limiter contention** finding in `cluster-readiness.md`. It does not certify cluster HA, archive completeness, handover safety, service restart, or release readiness. No handover/application logic was changed by this work.

## Change and exact limits

`GET /api/v1/agent/connect` and `GET /api/v1/agent/state` no longer spend the interactive login/enrollment bucket. They first validate TLS, request framing, connection upgrade where applicable, the current database-backed bearer credential, configured remote-node identity, optional claimed node, and standby permission for archives. Only the **resolved node name**, never the supplied header, IP, token prefix or token string, keys authenticated workload limits.

| Scope | Rate / burst | Additional bound |
|---|---|---|
| Login, setup, join, signed retirement | Existing `api.login-rate`, default **5/minute / 3** per IP; unchanged shared bucket | Existing body limits and authentication remain |
| Invalid cluster connect/state requests | Separate per-IP failure bucket using `api.login-rate`, default **5/minute / 3** | Exhaustion checked **before** credential lookup; check/validate/charge serialized |
| Authenticated connect attempts | **12/minute / 3** per validated node | Independent of state quota |
| Authenticated state downloads | **6/minute / 2** per validated standby | **1 active archive per node, 2 overall**, covering generation and streaming; released on handler exit |
| All cluster connect/state admission attempts | **100/second / 100** globally, before credential lookup | At most **16** executing/waiting admission checks; excess rejected rather than queued |

Workload/global limits are currently fixed server policy, not new tunables. Failure rate follows changes to `api.login-rate`; this work does not raise its default. Keyed buckets use the existing bounded 10,000-key limiter; its existing capacity-eviction behavior is unchanged. The state active set is bounded at two entries. Connect rate applies to upgrades, not frames/pings on an existing session.

Successful credential checks do not consume or refill the failure bucket. A malformed/hostile peer **can still temporarily lock out valid nodes on its source IP** by exhausting that failure bucket. This is deliberate fail-closed brute-force protection; healthy shared-IP activity no longer creates this lockout. Failed cluster credentials do not consume interactive login/enrollment quota, and vice versa. Invalid requests are still rejected, not authenticated at a globally increased login rate. A distributed attack is additionally bounded by the global validation gate.

A valid node cannot reset its budget by changing source IP or rotating its token. Every request rechecks the current credential store, and the existing cluster master independently rechecks credentials/authorization before upgrading or writing an archive; no positive credential cache or revocation window was introduced. The admission check therefore performs an additional credential lookup/update on successful requests.

Large simultaneous transfers can still receive HTTP 429 from the explicit workload/concurrency limits. Responses include `Retry-After`; existing agent retry behavior is unchanged. More than two joins immediately after first-time setup can still exhaust enrollment's original burst. This is not a promise of unlimited or 429-free onboarding.

## Validation ordering

- Global admission rate/queue bound, then per-IP failure lockout, before credential DB work.
- TLS required. Both GET endpoints reject nonzero/unknown content length or transfer encoding without reading a body; body rejection closes the HTTP connection.
- Connect validates the `Connection: ... upgrade ...` token and protocol `Upgrade` before credential lookup. These checks precede hijacking.
- Missing, duplicate, empty or oversized bearer credentials fail; the token is bounded to 256 bytes. Duplicate/mismatched node headers, undeclared nodes and the local master's node are rejected before node-budget allocation. Non-standby nodes cannot fetch archives.
- Existing join: TLS then shared login limit then bounded JSON (`4 * api.max-body`) and one-time-code validation. Existing retirement: shared login limit then TLS, 4 KiB JSON and signed-notice verification. Both remain unchanged.
- Existing browser setup/login still perform bounded JSON decoding before their login limiter and expensive credential operation; this change does not alter those handlers.

## Actual default-setting results

Measured locally on Windows with the supplied Go toolchain, isolated temporary stores and loopback listeners. No production Downloads instance or external notifications were touched.

**RED:** The original handler failed the three-authenticated-nodes/same-IP test with state HTTP **429**, `Retry-After: 12`. A real setup → pinned-TLS join → live upgraded session → first archive also failed with state HTTP **429**. Initial RED preceded implementation; the final stronger state-specific RED used a Go overlay of `HEAD:internal/server/h_agent.go` without reverting the working tree.

**GREEN:** Three repeated default-setting runs each completed **two real one-time-code joins, two live pinned-TLS sessions and four encrypted state downloads** from the same loopback IP, after setup had already consumed one of the login burst's three tokens. No login-rate, ping, retry, or scheduling overrides; no retry sleeps. Total measured sequences:

| Run | Total | First archive from node 1 join start | First archive from node 2 join start |
|---|---:|---:|---:|
| 1 | 989.5005 ms | 275.063 ms | 272.827 ms |
| 2 | 708.4848 ms | 202.622 ms | 191.517 ms |
| 3 | 793.8516 ms | 270.870 ms | 199.485 ms |

The separate final full-package run completed the same sequence in **2.5262798 seconds**, also without admission failures. Timing varies with host/test load.

Timing excludes process/app initialization, setup and installation; total includes code issuance, joins, enabling standby records, connection establishment and four sequential downloads. Fixtures have no monitors/history and archives were about 10–11 KiB; these are not large-database, WAN, or fresh-install timings. A separate three-node test exercises three connect admissions and six real archive generations; its recorder intentionally cannot hijack, so live-session evidence comes from the TLS test above.

## Regression evidence

- Server/cluster/app/ratelimit/store/engine suites: **356 passing test events including subtests**, zero failures.
- Dedicated admission/limiter tests repeated three times: **96 passing test events including subtests**, zero failures.
- Covers shared-IP isolation, independent state/connect quotas, refill, token rotation without quota reset, immediate revocation, node mismatch/ambiguity, undeclared/local nodes, probe-only archive denial, 12 concurrent invalid requests admitting exactly three credential failures and rejecting nine, pre-lookup failure lockout, global validation rate and 16-request queue bound, state concurrency limits, TLS/body/upgrade/header rejection before credential writes, and unchanged exhausted join/retire/setup/login budgets.
- `git diff --check` passed. Race testing was attempted and **blocked by missing gcc** (`runtime/cgo`); no race-clean claim. Sustained load, WAN/loss, deployment restart and very large archives remain outside this test scope.

Raw evidence (retain these ignored `test-results` artifacts separately when releasing):

| File | SHA-256 |
|---|---|
| `cluster-admission-red.jsonl` | `3e9af8f95cd21c7e3445d32397fcb35b5b4b165268fd9b49fce4e99aeaa99251` |
| `cluster-admission-state-red.jsonl` | `4ef4de503a73c876b67e0e20c9125cd2d9d73105ad42bc6c46bdd653f598c685` |
| `cluster-admission-final-related.jsonl` | `211c755750a95e8787935e9c2ea446da4919a4eac3d01baa85b1557b08ef3108` |
| `cluster-admission-final-repeat.jsonl` | `9913fa0da866dc5d4c4d87a0277645b00088d09360cc9e688ba60a96eb128da5` |

Reproduce from the repository root using the supplied Go toolchain and approved scratch TEMP/TMP:

```bash
go test ./internal/server ./internal/ratelimit -run '^TestClusterAdmission|^TestKeyedCheck' -count=3 -timeout=90s -json
go test ./internal/server ./internal/cluster ./internal/app ./internal/ratelimit ./internal/store ./internal/engine -count=1 -timeout=180s -json
```

Owned changes: `internal/server/h_agent.go`, dedicated `cluster_admission*.go`, one server admission-state field in `server.go`, non-consuming `ratelimit.Keyed.Check` plus its test, and this document. `h_cluster.go` was reviewed but needed no change. No commits, pushes or release were performed.
