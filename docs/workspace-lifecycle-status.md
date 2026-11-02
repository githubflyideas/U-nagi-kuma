# Workspace scheduler status

## Caller contract

The shared App owns one bounded supervisor for published nondefault Workspaces.
The default Workspace remains owned by the existing engine. No schema migration
is required by lifecycle status; snapshots are in-memory observations, not durable
health history. Tenant means company; Workspace means department/project.

```go
func (a *App) WorkspaceRuntimeStatus(ctx store.WorkspaceContext) (app.WorkspaceRuntimeStatus, error)
func (a *App) WorkspaceDiscoveryStatus(userID int64) (app.WorkspaceDiscoveryStatus, error)
```

For server integration, derive `UserID` from the authenticated session/token, never
from request JSON/query parameters. Scope identifiers may be selected by the
caller, but the App/Store checks live access to that exact company/Workspace on
every read. Workspace viewers may read their own scope; revoked membership and
foreign scopes fail with `store.ErrTenantAccess`. Installation discovery requires
a live, enabled **global admin** account; Workspace/Tenant admin grants alone do
not grant this authority. Authentication itself remains the server's job.

DTOs contain only IDs, revision/generation, fixed state/code strings, timestamps,
and counts. They exclude configuration, target addresses, monitor names, resolved
secrets, capability material, and raw errors. Errors returned by these methods
must use the server's generic error mapping, not `err.Error()` in HTTP responses.
A failed authorization/storage read returns a zero DTO, never a stale snapshot.

### Workspace JSON fields

- `tenant_id`, `workspace_id`, `revision`, `generation`: exact authorized publication.
- `state`: `not_published`, `pending`, `queued`, `running`, `scheduled`, `blocked`,
  `error`, `canceled`, `inactive`, or `stopped`.
- `code`: omitted on normal progress; otherwise `unsupported_execution`,
  `unsupported_notifications`, `authority_unavailable`, `execution_failed`,
  `execution_canceled`, `authority_inactive`, or `discovery_failed`.
- `last_attempt`, `last_completed`, `next_attempt`: wall-clock timestamps, zero
  when unavailable. Completion means the bounded pass ended, not target health.
- `consecutive_errors`: failed execution-pass count; cleared after success or a
  new publication. Retry delay is 1, 2, 4, 8, 16, then 30 seconds, capped at 30.
- `notification_status`: `not_started` before the first pass, then the executor's
  allowlisted `ready`, `pending_delivery`, `error`,
  `blocked_unsupported_notifications`, or legacy `blocked_unscoped_notifications`.
  Unknown executor values become `unknown`. `ready` is not proof of delivery.

`scheduled` means the pass succeeded and recurrence is scheduled; it does not mean
all monitored targets are healthy. Disabled/revoked authority overrides a cached
status immediately at read time. A new revision/generation cannot inherit a stale
success. Reopening an App starts published active scopes at `pending`, not healthy.

### Installation discovery JSON fields

`state` is `not_started`, `starting`, `ready`, `error`, or `stopped`. `code` is
`discovery_failed` on failure. `last_attempt`, `last_success`, `next_attempt`, and
`consecutive_errors` expose discovery timing independently of probe timing.
`active_workers` and `capacity` expose admitted passes (capacity eight), not HTTP
requests or a count of healthy departments. It deliberately exposes no scope list.

Discovery errors stop new admission, cancel existing passes, and retry with the
same capped exponential backoff. Recovery clears the discovery error and resumes
reconciliation. A scope status reflects `discovery_failed` while metadata remains
readable. If the core database itself is unavailable, live authorization cannot
succeed: the authenticated API returns a generic error rather than failing open.

## Lifecycle guarantees

- One App/shared Store, at most eight concurrent passes; no goroutine per timer tick.
- Oldest-due-first admission with deterministic scope tie breaking. Scopes waiting
  for capacity remain visibly `queued`; more than eight departments are supported.
- Each scope keeps its own monitor cadence. Full registration precedes due-subset
  execution, so a slow monitor is not retired by another monitor's faster cadence.
- Configuration replacement cancels the old pass before admitting its successor.
- Store revocation/disable fences execution/results and cancels admitted probes;
  polling latency is not an authorization grace period.
- Shutdown cancels and joins all workers before App.Run closes the Store. Status
  is process-local; the Store cannot authorize reads after closure.
- This supervisor does not implement notification dispatch. It reports the scoped
  executor's result; notification ownership, delivery retries, and supported channel
  semantics are implemented and tested separately. No global notifier fallback.
- Cancellation fencing is within the shared Store instance, not a distributed lease
  or cross-process scheduling claim. Core/history durability remains governed by the
  Store execution contract; status does not make cross-database commits atomic.

## Acceptance matrix and tests

All scheduler e2e cases run the real `App.Run` with disposable shared databases and
localhost HTTP targets; no external targets or notifications are configured here.

| Boundary | Regression |
|---|---|
| Authorized viewer, anonymous/foreign/revoked member, installation admin | `TestWorkspaceLifecycleStatusAuthorization` |
| Fixed notification status allowlist and retry cap | `TestWorkspaceLifecycleStatusSanitizationAndBackoff` |
| Closed-store discovery is visible internally and backs off | `TestWorkspaceLifecycleDiscoveryFailureVisible` |
| Unsupported execution, injected secret-bearing DB error, 1s/2s backoff, replacement/recovery | `TestWorkspaceScheduleBlockedAndErrorBackoff` |
| Unsupported notification semantics are explicitly blocked | `TestWorkspaceScheduleUnsupportedNotificationsVisible` |
| Authorized discovery-error API and recovery | `TestWorkspaceScheduleDiscoveryFailureAndRecovery` |
| 12 scopes each recur, eight held slots, four visibly queued | `TestWorkspaceScheduleCapacityAndFairness` |
| Revoke/disable/replace cancel a 5s HTTP probe within 1s, no late history, unrelated scope continues | `TestWorkspaceScheduleRevocationCancelsNoLateHistory` |
| Mixed cadences, live update, restart publication/status, stable IDs, disabled/revoked scope stays off | `TestWorkspaceScheduleRecursAndRestarts` |

Test fixtures use a persistent SQLite trigger for pooled-connection-safe failure
injection. The replacement cancellation test keeps the monitor registered but
paused: retired IDs are intentionally unreadable through the ordinary scoped
history repository, so deleting it would test authorization rather than absence
of late history.

Run with the pinned Go toolchain and disposable TMP/TEMP/TMPDIR:

```sh
go test -p 1 ./internal/app -run TestWorkspaceLifecycle -count=1 -json
go test -p 1 ./e2e -run TestWorkspaceSchedule -count=1 -json
go test -p 1 ./internal/app ./internal/store ./e2e -count=1 -json
```

These are App/Store acceptance checks. HTTP route mounting, browser presentation,
packaged-binary acceptance, cross-process authority fencing, and notification
end-to-end acceptance are separate integration gates, not implied by these tests.
