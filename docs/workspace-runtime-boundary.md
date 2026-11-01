# Workspace runtime admission boundary — earlier safeguard checkpoint

**Update:** a later bounded check-only execution path is now verified; see
[Workspace execution checkpoint](workspace-execution-checkpoint.md). The original
notes below describe the earlier default-runtime safeguards. Full department
scheduler/incident/notification/server/UI integration remains blocked.

This checkpoint does **not** complete the requested department runtime vertical.
It closes observed fail-open paths before adding a service-scoped runtime. There
is still one App and the existing shared core/history SQLite stores. No new
schema, HTTP route, UI switch, company scheduler, or cloud service was added.

## Observed blockers and implemented fixes

- `installRuntime` previously logged a failed monitor registration and still
  installed the unowned name in the real scheduler. It now retains the previous
  runtime on registration failure. Startup refuses to return a usable App if no
  runtime could be registered.
- Base-file validation rejects known foreign-only monitor names before writing
  the file. API save, file polling, and startup/fallback use this check. Zone-local
  names are not interpreted as base names; their registration is checked after
  the existing typed Zone namespace transform.
- `ApplyZone` previously saved its restart manifest before discovering a monitor
  ownership collision. Default monitor registration/retirement and the applied
  Zone manifest now commit in one core transaction. Failed snapshot writes roll
  back registry changes and leave its identity cache unchanged.
- The app sink no longer embeds Store. Its explicit methods refuse foreign-only
  or unregistered monitor records before persistence/global publication, and
  check incident references on state, intents, delivery, and decision paths.
  Default rename aliases remain supported. Instance events still use the legacy
  empty-monitor convention; that convention is **not** workspace authority.
- Startup filters known foreign/unregistered state, incidents, and pending
  notifications before the legacy engine can expose or coalesce them. Legacy
  global Store APIs themselves have NOT become scoped repositories.
- The engine previously sent notifications even when `SaveIntent` failed, both
  at enqueue and at dispatch of restored intents. Failed creation now reports
  not queued. Failed dispatch restores the unstarted delivery step and does not
  send; later dispatch may retry. This also changes storage-error behavior:
  durable intent admission now takes precedence over best-effort sending.

These are default compatibility safeguards, **not complete multitenant security**.
Name-only records cannot distinguish two companies with the same display name.
The admission preflight conservatively rejects foreign-only names, including
retained foreign identities. It does not establish tenant ownership from names.

## Exact callable API

Existing `App.SaveConfig`, `App.ApplyZone`, `App.Reapply`, and `App.New` remain
**default-company/default-workspace** operations. Existing
`Store.WorkspaceMonitors(WorkspaceContext)` remains the live-authorized typed
monitor/state/history repository, NOT an engine sink.

New trusted instance helpers:

```go
func (s *Store) CheckDefaultRuntimeName(name string) error
func (s *Store) CheckDefaultRuntimeRecord(name string) error
func (s *Store) SyncDefaultRuntimeSnapshot(defs []MonitorDef, key, value string) error
```

`CheckDefaultRuntimeName` permits prospective new names; `Record` additionally
requires a registered default identity. Neither grants nondefault authority.
`SyncDefaultRuntimeSnapshot` atomically synchronizes only default monitor
identities and one settings snapshot. It must not be exposed to workspace users.

**There is no callable nondefault runtime-apply API in this checkpoint.** No
interactive user ID is captured or impersonated by a background worker.

## Genuine RED → GREEN evidence

Commands used the supplied Go toolchain, scratch temporary directory,
`-p 1 -count=1` and each named test's `-run '^TestName$'` selector.

| Package / test | Observed RED | GREEN contract |
|---|---|---|
| engine / `TestRejectedIntentNeverDispatches` | rejected durable intent reported queued; retained for dispatch | rejected creation cannot queue/send |
| engine / `TestRestoredRejectedIntentNeverDispatches` | restored rejected intent was sent | rejected dispatch cannot send |
| e2e / `TestWorkspaceRuntimeRejectsLegacyActivation` | legacy config activated workspace monitor department-a | both department names denied; real base HTTP probes continue through restart |
| app / `TestRuntimeRegistrationFailureKeepsPreviousEngine` | registration failure published unowned runtime | previous effective config retained |
| app / `TestRuntimeSinkRejectsForeignRecords` | foreign records globally published: 3 | no foreign publication; later RED repeated for unregistered names |
| app / `TestRuntimeZoneRegistrationFailureAtomic` | zone published despite ownership registration rejection | no new applied manifest or engine config |
| store / `TestDefaultRuntimeSnapshotAtomic` | missing API compilation failure | injected snapshot error rolls back retirement/cache, retry commits |
| app / `TestRuntimeStartupRejectsUnregisteredSnapshot` | startup accepted snapshot whose monitor registration failed | startup refuses invalid registration |
| app / `TestRuntimeRestoreDoesNotAdoptForeignIncidents` | foreign incident restored into default engine | foreign incident excluded from engine reads |

Two initial negative-test fixtures passed for unrelated reasons (SOA auto-bump
from serial 1; unsupported strict Zone PUSH validation). They were corrected and
rerun to the actual failing ownership behavior before implementation. Those
initial passes are not claimed as isolation evidence.

## Remaining work before the requested department vertical is safe

1. Durable runtime service identity with explicit tenant/workspace binding,
   revocation/generation semantics and permitted operations. Workspace admin
   authorization must be checked live for config mutation; background execution
   must not retain that user's mutable membership as its authority.
2. Stable internal engine keys separate from display names and probe targets;
   structurally map monitor parents, channels, policies, maintenance and all
   notification references. Name guards here are not a substitute.
3. Owned applied configuration, encrypted secrets, incidents, intents, deliveries,
   decisions, evidence and events, including immutable ownership checks on IDs,
   restore, digest/backstop/report aggregation and dedicated scoped subscribers.
   There is no scoped secret/local-log runtime in this patch.
4. Atomic workspace config publication and monitor registration; error propagation
   for the base-file/DB/runtime multi-resource transaction still needs work.
   Base validation is a preflight: an ownership race after validation is stopped
   at engine installation, but legacy SaveConfig may already have saved the file
   and revision. Do not describe that as atomic config publication.
5. Real two-workspace same-target checks with different credentials, independent
   failure/history/alerts, scoped event reads, rejected cross-scope writes and
   restart/reopen notification semantics. The new e2e test proves **rejection and
   default continuity**, not successful department execution. Source-unavailable
   Zone tests continue testing the existing default runtime only.
6. Authenticated server/UI binding and importer authorization remain separate
   integration work. No tenant switching or nondefault import activation was added.

No commit/release was made. Package and isolated in-process tests are not the
parent's three frozen-instance release gate or extracted-package acceptance.
