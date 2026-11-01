# Workspace check execution checkpoint (not a product/release gate)

This supersedes the **no department execution** statement in the earlier
runtime-boundary and migration-19 notes. There is now an exercised, callable,
bounded check path in the **existing App and shared core/history SQLite**. It is
not a second App/database/legacy engine per Workspace. No schema migration,
server route, UI activation, automatic scheduler, or notification integration was
added. Existing default and Zone runtime installation remains unchanged.

## Acceptance boundary

| Requirement | Implemented/check exercised |
|---|---|
| Same target, same display name, two departments, different credentials | Real localhost HTTP, distinct scoped secrets and numeric monitor IDs; A OK, B critical |
| Actual scoped state/history | Synchronous owned registration, pure engine state transition, scoped state + raw/aggregate history writer |
| Concurrent departments | Both HTTP requests enter concurrently; duplicate pass in A denied; revoke A cancels only A and B completes |
| Execution admission/revocation | Exact published generation + live reader hash/activity; successful revoke, disable and replacement cancel admitted HTTP; old generation cannot register/write/execute |
| Restart | Actual Store close / App.New; same IDs, independent state/history preserved; each scope has two real rounds after second pass |
| Permissions | Unauthenticated/foreign/revoked-admin mutations denied; foreign ID writes/reads denied; zero/forged/secret-only writer denied; no interactive issuer retained |
| Notifications/publication | Explicit `blocked_unscoped_notifications`; no incident/event/delivery/evidence/global publisher writes, even with configured webhook policy |
| Failure behavior | Late registry failure rolls back; raw/agg5m/hourly write faults return error and roll back core state/history transaction writes; failed revoke does not cancel current work |

## Exact caller hooks

```go
// Trusted installation supervisor ONLY, never directly bound to request IDs.
func (a *App) RunWorkspaceChecks(ctx context.Context, tenant store.TenantID,
    workspace store.WorkspaceID, generation int64) (WorkspaceCheckBatch, error)

type WorkspaceCheck struct { ID int64; Name string; Status string }
type WorkspaceCheckBatch struct {
    NotificationStatus string
    Checks []WorkspaceCheck
}

// Trusted store glue, not an interactive/user repository or engine.Sink.
func (s *Store) BindWorkspaceRuntime(reader *RuntimeSecretReader) (*WorkspaceRuntimeWriter, error)
func (w *WorkspaceRuntimeWriter) BeginExecution(parent context.Context) (context.Context, func(), error)
func (w *WorkspaceRuntimeWriter) Register(defs []MonitorDef) (map[string]int64, error)
func (w *WorkspaceRuntimeWriter) Record(id int64, result probe.Result,
    interval time.Duration, params engine.StepParams) (engine.StateRecord, error)
func (w *WorkspaceRuntimeWriter) Pause(id int64) error
func WorkspaceMonitorKey(workspace WorkspaceID, name string) string
```

`RunWorkspaceChecks` restores the exact published model and scoped reader, validates
this checkpoint's supported subset, binds a writer, admits a cancellation lease,
registers only that scope, and sequentially executes its enabled checks with the
existing `probe.Run` interface. `engine.Step` advances the existing availability/
performance state machine from scoped persisted state. It never runs the legacy
incident, notification, evidence, or event loop. Probe output details/raw exchanges
are deliberately discarded rather than risk retaining response credentials.

Readback uses existing live-authorized `Store.WorkspaceMonitors(WorkspaceContext)`:
`States`, `MonitorIDs`, `Rounds(id, ...)`, `Cells`, `Latency`, `UptimeOf`.
`WorkspaceCheck.Name` is the configuration/display identifier; the registry/state
name is `workspace:<base64url WorkspaceID>:<base64url name>`. Registry ownership
still includes TenantID + WorkspaceID, and history uses its globally unique numeric
ID. Targets, definition hashes, titles and credentials are **not identity keys**.
Active IDs survive definition edits and restart. Display-name renames do not carry
an ID automatically; aliases remain unsupported. State reads can include retained
retired states; filter against authorized active IDs when building future views.

The writer cannot be obtained from a zero/foreign Store reader or a secret-only
credential newly issued outside configuration publication. Every operation repeats
live authority and published-generation checks inside the immediate core
transaction. `Register` is atomic with its admission, not with the earlier config
publication: saving a snapshot is not proof registration/execution succeeded.
No default monitor cache is populated. No background user identity is captured;
removing the publishing user's grant prevents future edits, but explicit service
revocation/activity controls own already-issued service authority.

## Bounds, cancellation and plaintext lifetime

- One pass per Workspace at a time; at most eight passes per Store. Busy admission
  returns `store.ErrWorkspaceRuntimeBusy`; no unbounded queue is created.
- At most 64 monitors per pass. HTTP only, timeout at most five seconds; a pass's
  execution context has a 30-second deadline. Database/lock acquisition uses the
  existing Store behavior; this is not a hard real-time 30-second API SLA.
- Non-HTTP monitors, parents, NODE assignments, maintenance windows and longer
  timeouts fail with `ErrWorkspaceExecutionUnsupported` before registration/probes.
  Unsupported config may still be durably staged by the broader config API.
- `disabled=yes` persists paused state without sending HTTP or inventing a history
  sample. Re-enable requires execution of an enabled published configuration.
- Immediately before **each** actual probe, generation/activity is rechecked and
  scoped references are resolved anew. Nothing stores a compiled plaintext probe
  between calls. Deleting a secret blocks the next pass rather than reusing the
  prior value; changing it is observed in the next real request.
- Successful Workspace transactions fence leases after commit, while holding the
  Store write mutex. Revoke, runtime disable and new publication cancel old scoped
  work before the mutation returns. Failed/unauthorized writes do not cancel it.
  The caller must still join execution; cancellation cannot undo bytes already
  sent or erase Go strings already returned. Revoked results are not persisted.
- This cancellation registry belongs to the **single Store in the App**. Direct
  database writes or another independently opened process/Store are not a supported
  live revocation transport. Their changed authority is still checked on the next
  reader/admission/write, but no cross-process cancellation mechanism exists.
- Caller owns the parent context, cadence, cancellation and join before App/store
  shutdown or KEK rotation. No automatic App.Run startup, discovery, recurring
  scheduler, takeover/supersession worker lifecycle or task leases were wired.
  Do not pass the model into `installRuntime` / `publishRuntime`.

## Exact limitations and parent integration work

1. **Notifications are blocked, not working local alerts.** Every batch, including
   errors, reports `blocked_unscoped_notifications`. Scoped incidents, decisions,
   intents, delivery retries/dedup, recovery/reopen, audit and subscribers remain
   unimplemented. No fallback to default policy/channel/public event paths exists.
2. This is a check-only executor, not the complete scheduler: no recurring timer,
   dynamic cadence/retry scheduling, multi-node quorum, maintenance/parent semantics,
   local-network anomaly gating, scoped push/EXEC/other probes or public status
   grants. Only the documented subset is admitted. Notification settings are
   validated/stored but never executed. Do not claim full monitoring product parity.
3. Core and history remain separate databases. Normal record calls write core state
   in a transaction, commit history synchronously, then commit core. Injected state
   and history statement failures are fail-closed, but a core **commit** failure
   after a successful history commit can leave a history sample without updated
   state. The API returns error; no blind retry/exactly-once guarantee is claimed.
   Parent must decide cross-store result identity/reconciliation before durable
   task/result delivery. The batch aggregation cache still follows history commit.
4. Supervisor scope selection is trusted installation plumbing, not authorization
   supplied by raw user IDs or a generation number. Future authenticated APIs must
   authorize Workspace selection and keep runtime capability handling private.
5. Before release, parent must integrate scoped incidents/notification and worker
   lifecycle into this same App; add authorized server/UI/import/report paths,
   test sustained scheduling, alert/recovery/reopen and deployment restart, and
   complete the full three-instance/frozen-tree/extracted-package acceptance.

## Observed verification

Genuine RED → GREEN cases:

- `TestWorkspaceExecutionIsolation`: missing callable execution method → two actual
  HTTP probes, distinct credentials, IDs 1/2, state OK/critical, one round each.
- `TestWorkspaceExecutionRevocationCancels`: real five-second HTTP request did not
  cancel/join within one second after revoke/disable/replacement → cancellation
  lease fencing. An initial 100ms fixture accidentally passed by probe timeout;
  that fixture was corrected before implementing cancellation and is not evidence.
- `TestWorkspaceExecutionRestartAndDisable`: disabled pass left persisted OK state
  → paused state is saved without a fake history sample.

Additional regression coverage: simultaneous A/B execution and independent
cancellation, stale generation, missing secret/fresh plaintext resolution, denied
foreign/revoked-user mutations, service independence from issuer grants, cap/release,
foreign IDs, zero/secret-only authority, unsupported subset, late registration
rollback, injected raw and both aggregate history failures. The aggregate fault
fixture uses positive latency to actually exercise both aggregate inserts.

Final targeted commands use the supplied Windows Go, scratch TEMP/TMP/TMPDIR,
`-p 1 -count=1 -json`:

- Full `./internal/store ./internal/app` suites passed. Programmatically counted
  pass events, top-level / including subtests: **store 76 / 83; App 57 / 95**.
  Existing Windows symlink-privilege test skipped; no new skips.
- `go vet -p 1 ./internal/store ./internal/app` passed.
- Four real in-process e2e regressions passed: legacy Workspace activation rejection,
  Zone standalone, restart, and invalid-CAS/recovery. They verify default/Zone
  continuity, not department notification completion.
- Exact last isolation evidence: scopes `6eecddbf7aec508f44f5787c59eb92bc` and
  `8cb8efd8045a6d950f3861abfee4db3b`, both display `check`; IDs **1 / 2**,
  states **ok / critical**, **one round each**; localhost observed exactly
  `department-A:1`, `department-B:1` credentials. A configured notification webhook
  produced no additional request or global publication.
- Restart fixture readback: IDs **1 / 2**, states **ok / critical**, **two rounds
  each**. Disabled monitor emitted no additional probe/history. Both inactive and
  revoked scopes stayed denied after another App.New; enabling alone did not
  restore authority; republishing resumed the same stable ID.

Raw evidence lives in Hermes scratch:
`workspace-execution-packages-final.jsonl`,
`workspace-execution-default-zone.jsonl`.
Last measured free space after tests: **672260096 bytes**. The additional core-state
fault assertion in `TestWorkspaceExecutionWriteRollback` and final vet were rerun
successfully after the full package logs above. No user-data/cache cleanup,
large cross-build, race-instrumented rebuild, commit, release, server/UI exposure,
external probe or real notification was performed. This is a concrete execution
checkpoint, not the parent's final product/release verification.
