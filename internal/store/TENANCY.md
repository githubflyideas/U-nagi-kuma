# Tenant / Workspace repository checkpoint (not full product isolation)

**Current checkpoint: migration 17 and the Workspace API documented at the end
of this file.** Migration-15/16 sections below retain their historical evidence;
the migration-17 authorization contract supersedes their tenant-only read policy
and the design-only Workspace seam. Runtime/UI activation is still unavailable.

The identity/membership foundation below is followed by the bounded migration-16
monitor/state/history repository slice. Company = Tenant; departments/projects
will be Workspaces inside that Tenant. This checkpoint does **not** equate a
department with a Tenant or add a competing Workspace schema.

This is the additive identity/membership stage of the architect's shared-schema
SQLite design. It does not change the scheduler, introduce per-tenant App or
separate databases, or expose tenant creation/switching to HTTP clients.
`core.db` and `history.db` remain the existing stores.

## Exact typed repository API

```go
type TenantID string
type NetworkScopeID string
type Tenant struct { ID TenantID; Name string }
type TenantContext struct { TenantID TenantID; UserID int64 }
type TenantMember struct { TenantID TenantID; UserID int64; Role string; Areas []string }
type NetworkScope struct { ID NetworkScopeID; TenantID TenantID; Label string }

func (s *Store) DefaultTenant() (Tenant, error)
func (s *Store) TenantMember(ctx TenantContext) (TenantMember, error)
func (s *Store) CreateNetworkScope(ctx TenantContext, label string) (NetworkScope, error)
func (s *Store) NetworkScope(ctx TenantContext, id NetworkScopeID) (NetworkScope, error)

var ErrTenantAccess error
var ErrNetworkScopeLabel error
```

`TenantContext` is not an authentication credential. The trusted caller must
populate UserID from the authenticated principal, never from a request body.
Every membership/scope operation rechecks live membership and account disabled
state. Missing/disabled/wrong-tenant/wrong-user resources produce the same
`ErrTenantAccess`. No default-tenant fallback occurs in these methods.

Network-scope creation requires an active tenant-admin membership and performs
its authorization and INSERT in one SQL statement. Duplicate labels in the same
tenant return a SQLite constraint error. The same label in different tenants is
valid and produces distinct random IDs. This registry is a documented extension
for overlapping private networks, not a substitute for tenant identity and not
yet a probe-routing mechanism.

## Exact SQL and compatibility contract

The authoritative complete SQL is `tenant_schema.go`:
`tenantFoundationMigration`, appended as core schema migration **15** in
`schema.go`; migrations 1–14 and all history migrations are unchanged.

- `tenants(id TEXT PRIMARY KEY, name TEXT, is_default INTEGER)`; 128-bit random
  lowercase hexadecimal IDs, nonempty display names, one immutable default flag.
- `tenant_members(tenant_id TEXT, user_id INTEGER, role TEXT, disabled INTEGER)`;
  composite primary key `(tenant_id,user_id)`, checked roles, foreign keys to
  tenants/users with cascading deletion.
- `network_scopes(id TEXT PRIMARY KEY, tenant_id TEXT, label TEXT)`; random stable
  IDs, tenant FK and `UNIQUE(tenant_id,label)`, immutable ID and ownership.
- Triggers prohibit changing tenant IDs/default identity, deleting the default
  tenant, or changing scope IDs/ownership. Display names/labels are not identity.
- Legacy users are backfilled into the generated default tenant with their exact
  role and disabled state. Area restrictions remain on users and are returned by
  membership lookup; they are not broadened or replaced.
- All existing user creation paths intentionally get a default membership with
  the exact requested role, via an atomic INSERT trigger. `CreateUser` remains
  the existing explicit administrator constructor. `CreateUserRole` and
  `CreateUserHash` do not promote viewer/operator accounts.
- Global role changes synchronize default membership. Other tenant memberships
  are capped downward on demotion, never promoted by a global promotion.
- Account disabled changes synchronize membership account-disabled flags;
  lookup also joins live users defensively. These flags mirror account state,
  not an independent per-tenant suspension API. User deletion cascades members.
- No tenant creation, grant, or tenant-switch HTTP API was added.

`migrateDB` backs up the old core database to `core.db.bak-14`, executes migration
15 and its user_version update in one transaction, and rolls back on failure.
Reopening is idempotent. Tests inject failure after the whole migration body,
verify no new schema/backfill/version remains, inspect the old-schema backup,
and then retry successfully. Binary downgrade requires stopping the application
and restoring a suitable pre-upgrade backup; this checkpoint does not implement
a live down-migration. Monitor IDs/history rows are untouched.

## Observed RED → GREEN evidence

All commands used the supplied Go toolchain and `-p 1 -count=1`.

1. `go test -p 1 ./internal/store -run TestTenantFreshSchema -count=1`
   RED: `SQL logic error: no such table: tenants (1)`; GREEN after migration.
2. `go test -p 1 ./internal/store -run 'TestTenant(LegacyUpgrade|MembershipTracksUsers)' -count=1`
   RED: both reported `sql: no rows in result set`; GREEN after backfill/triggers.
3. `go test -p 1 ./internal/store -run TestTenantContextAndNetworkRegistry -count=1`
   RED was a compile failure for the not-yet-defined typed API; GREEN after API
   implementation. An intermediate fixture failure exposed SQLite multi-statement
   parameter binding; fixture statements were split, not authorization weakened.
4. `go test -p 1 ./internal/store -run TestTenantIdentityImmutable -count=1`
   RED: `identity mutation accepted: UPDATE tenants SET is_default=0`; GREEN
   after immutable-identity triggers.

Additional regression tests verify migration rollback/retry/backup, imported
roles/areas, no cross-tenant promotion, current disabled state despite a stale
membership, delete cleanup, failed user creation atomicity, and persisted scope
identity after close/reopen. These additional tests were written after the
underlying behavior and are regression evidence, not claimed as new RED cycles.

Final package results are reported by the implementing agent. Fresh store Open,
old-schema upgrade and HTTP setup/auth harness tests are not a packaged-product
acceptance run or the three fresh-instance release gate.

## Migration 16: bounded monitor/state/history isolation

Baseline: architect PDF page 7 sections 5.2–5.4 and page 8 section 6.2: shared
core/history SQLite, stable monitor IDs, tenant-owned identity, backfill then
query switch. No per-tenant App/database, no target-address deduplication, and no
scheduler redesign. The later user decision reserves departments/projects for
Workspaces **within** a company Tenant; network scopes remain orthogonal.

`tenant_monitor_schema.go` is appended after migration 15. It adds/backfills
`monitors.tenant_id`, prohibits absent ownership and identity/ownership changes,
and changes the active-name index to
`UNIQUE(tenant_id,name) WHERE retired_at IS NULL`. This retains the pre-existing
retired-identity behavior: a different definition may reuse a retired name with
a new ID; an exact rollback can revive its old ID/history. The uniqueness rule
is for **active** identities, not an all-time ban on reusing retired names.
`monitor_state` is transactionally rebuilt with primary key `(tenant_id,name)`;
all existing state belongs to the default Tenant. Numeric monitor IDs, SQLite's
monitor ID sequence, rounds, hourly and five-minute history are not renumbered
or rebuilt. History ownership is derived from its globally unique, immutable
`mid`; authorization checks that ID's owner before every scoped history access.

Migration 16 uses the existing transactional migration/backup machinery. A v15
upgrade backs up to `core.db.bak-15`; a direct v14 upgrade backs up to
`core.db.bak-14`. Tests inject failure after the complete migration-16 body,
observe version 15 with no new monitor column, inspect the v15 backup, retry,
and reopen twice. Downgrade still requires stopping and restoring a suitable
pre-upgrade backup; there is no live down-migration. History is unchanged by this
schema migration, but later application writes are not undone by that backup.

### Exact scoped API

```go
func (s *Store) TenantMonitors(ctx TenantContext) (*MonitorRepository, error)
func (r *MonitorRepository) SyncMonitors(defs []MonitorDef) error
func (r *MonitorRepository) MonitorID(name string) (int64, error)
func (r *MonitorRepository) MonitorName(id int64) (string, error)
func (r *MonitorRepository) MonitorIDs() (map[string]int64, error)
func (r *MonitorRepository) SaveState(id int64, state engine.StateRecord) error
func (r *MonitorRepository) DeleteState(id int64) error
func (r *MonitorRepository) States() ([]engine.StateRecord, error)
func (r *MonitorRepository) Verdict(id int64, v engine.VerdictRecord) error
func (r *MonitorRepository) Rounds(id int64, from, to time.Time, limit int) ([]RoundRow, error)
func (r *MonitorRepository) UptimeOf(id int64, from, to time.Time) (Uptime, error)
func (r *MonitorRepository) Cells(id int64, from, to time.Time, n int) ([]Cell, error)
func (r *MonitorRepository) Latency(id int64, from, to time.Time) ([]LatencyPoint, error)
```

- The repository is deliberately **not** `engine.Sink` and has private fields.
  Missing context, zero/nil repository, unauthorized user, disabled account,
  revoked membership, inactive or foreign monitor ID return `ErrTenantAccess`.
  Supplying an unauthorized Tenant never falls back to default. Names on writes
  must match the owned ID. Nondefault state cannot reference an unscoped incident.
- UserID must be populated by trusted authentication. A tenant selector is a
  requested scope, **not authorization**; a request body's `tenant_id` or
  `user_id` must never manufacture this context. No HTTP/API binding was added.
- Every operation checks live membership and role in an immediate core database
  transaction, retained while reading/writing history. Reads permit active
  admin/operator/viewer; writes require tenant admin. Area-restricted accounts
  fail closed in this new API until persisted monitor/workspace ownership can
  enforce their restriction. Legacy default application area behavior is unchanged.
  This is a conservative checkpoint policy, not the future workspace-role model.
- The cache is partitioned by TenantID, including rename aliases. Scoped
  identity reads use authorized SQL, not cached grants. Aggregation caches use
  immutable globally unique monitor IDs, so same names never share state.
- Scoped verdicts are synchronous and never enter a queue with stale authority.
  Raw and both aggregate tiers commit in one history transaction. Statement or
  commit failure leaves the aggregation cache unchanged and is reported to the
  caller; legacy batches requeue failures. Core/history are still two databases:
  this does not claim atomic multi-database business transactions.
- Legacy `Store` monitor/state/history methods intentionally resolve **only** the
  immutable default Tenant. This includes name lookup, reverse-ID lookup,
  synchronization/retirement, rename, state restore/delete, and batched verdicts.
  They are trusted compatibility adapters, not a general tenant repository.
  The private sync helper rejects a nil context for nondefault ownership and a
  context/tenant mismatch. Scoped rename is intentionally absent until incident,
  secret/push and event references can be moved safely together.
- Global retention, metrics and database backups remain trusted instance-level
  operations. Do not expose them as tenant-scoped APIs.

### Observed RED → GREEN (migration-16 slice)

Commands used the supplied Go executable, scratch TEMP/TMP/TMPDIR and
`go test -p 1 ./internal/store -run '^TESTNAME$' -count=1`:

| Test name | Observed RED | GREEN |
|---|---|---|
| `TestTenantMonitorSchema` | `no such column: tenant_id` | migration/backfill/index/guards pass |
| `TestTenantMonitorIdentity` | undefined `MonitorRepository` / `TenantMonitors` | typed identity and scoped sync pass |
| `TestTenantMonitorStates` | missing `SaveState` / `States` methods | isolated state and live role/revocation pass |
| `TestTenantMonitorHistory` | missing verdict/history methods | A/B rounds, uptime, cells, latency and foreign-ID rejection pass |
| `TestTenantMonitorHistoryAtomicFailure` | `rounds write failure reported success` | injected rounds/agg5m/hourly failure rollback and cache-safe retry pass |
| `TestTenantMonitorDeleteAndNames` | missing reverse/list/delete methods | owned reverse lookup/list and state deletion pass |
| `TestTenantMonitorInternalDefaultBoundary` | `implicit trusted nondefault access: <nil>` | private nil/mismatched context rejection pass |

Additional regressions (not claimed as new RED cycles): real v14/v15 upgrade,
late migration rollback and backup inspection, restart persistence, raw/five-minute/
hourly paths, retired-ID revival/replacement, default rename with colliding
nondefault names, disabled accounts, area restriction, role matrix and rejected
cross-business incident references. The first identity test fixture initially
omitted required `tenant_members.disabled`; that fixture error was fixed without
relaxing the schema or authorization.

### Final bounded verification (Windows toolchain)

All commands used `-p 1 -count=1 -json`; counts below were parsed from actual
`pass` events (top-level / including subtests): store **48 / 52**, app **41 / 59**,
server **83 / 135**, engine **122 / 149**, config **39 / 51**. The real in-process
`./e2e -run '^TestZoneRuntime'` suite passed all **3** scenarios: standalone,
restart, invalid CAS/recovery. `go vet -p 1` passed for those five internal packages.
Two unrelated existing platform skips: app symlink test lacks Windows symlink
privilege; engine shell-handler test requires a shell-script platform.

Raw command outputs are in Hermes scratch:
`tenant-monitor-packages-final.jsonl`, `tenant-monitor-zone-final.jsonl`.
These are worker/package and real in-process Zone checks, **not** packaged-binary
acceptance or the three frozen-tree release rounds. No commits or release were
made; disk free space was about 1.5 GiB after verification.

## Remaining boundaries and exact integration call sites

**Nondefault business HTTP/UI/runtime activation and tenant switching remain
unavailable.** Do not wire this repository into the current global engine as if
it completed multitenancy. No tenant provisioning, grants, switching or Workspace
schema/UI was added. Default company/default Workspace is the next product
integration goal, not a claim that Workspace persistence already exists.

Required next parent-owned integration points:

- `internal/app/app.go`: `States()` restore; `RenameMonitor` during apply.
- `internal/app/zone_runtime.go`: `installRuntime` / `SyncMonitors(defs)`
  must eventually sync only the authorized company's/workspace's definitions,
  not retire other departments' monitors. Current applied Zones remain default.
- `internal/app/sink.go`: `SaveState`, `Verdict` and global live-update emission;
  `internal/engine/sink.go`, `incident.go`, `process.go`, `apply.go`: carry trusted
  scope and stable monitor ID through records, deletion, callbacks and restore.
- `internal/server/precond.go`: numeric ID-to-name lookup; `h_monitors.go`:
  ID/name metadata, uptime, heartbeat cells, latency and CRUD/config ownership;
  `h_reports.go`, `h_public.go`: reports/status pages and shared-public grants;
  `internal/postmortem/postmortem.go`: incident-owned history queries.
- HTTP/session/token middleware must resolve authenticated principal plus allowed
  Tenant/Workspace, enforce CSRF and live roles, and keep public/shared grants
  explicit. Do not trust a raw body or URL identifier as an ownership grant.
- Incidents, events, decisions, deliveries, intents, evidence, push tokens,
  secrets, snapshots/settings/config, audit, status pages, contacts/on-call and
  Zone storage remain global. These are intentionally outside this migration.
  Audit/export/import/backup, caches, task/result/lease/dedup ownership and runtime
  configuration need integration before nondefault product exposure.

### Workspace extension seam (design only; no parallel schema)

Keep `TenantContext` as company/principal context. Add a separate typed
`WorkspaceID` / `WorkspaceContext` containing that context when the parent lands
Workspace ownership and default-workspace backfill; never reinterpret TenantID
or NetworkScopeID as a department. Require explicit authorized workspace grants,
with company-wide administration versus workspace administration decided there.
The new repository's `authorizeMonitors`, `ownedMonitor`, `MonitorID(s)`, `States`
and especially `syncMonitors` selection/retirement are the centralized filtering
seams. Extend cache ownership keys and state ownership together; stable monitor
IDs remain unchanged, so history can continue validating ownership via the
registry. Preserve company-scoped name uniqueness unless an explicit later
schema decision changes it. Shared resources need explicit grants rather than
an absent-workspace wildcard or automatic fallback.

## Migration 17: Workspace authorization and monitor ownership

`workspace_schema.go` appends migration 17; versions 1–16 and all history
migrations are unchanged. Companies are Tenants; departments/projects are
Workspaces within a company. Shared core/history SQLite remains in use.

### Exact added API

```go
type WorkspaceID string
type WorkspaceContext struct { TenantContext; WorkspaceID WorkspaceID }
type Workspace struct {
    ID WorkspaceID
    TenantID TenantID
    Name string
    IsDefault bool
}
type WorkspaceMember struct { WorkspaceContext; Role string; Disabled bool }
var ErrWorkspaceName error

func (s *Store) CreateWorkspace(ctx TenantContext, name string) (Workspace, error)
func (s *Store) Workspaces(ctx TenantContext) ([]Workspace, error)
func (s *Store) Workspace(ctx WorkspaceContext) (Workspace, error)
func (s *Store) RenameWorkspace(ctx WorkspaceContext, name string) error
func (s *Store) DeleteWorkspace(ctx WorkspaceContext) error
func (s *Store) SetWorkspaceMember(ctx WorkspaceContext, userID int64, role string, disabled bool) error
func (s *Store) RemoveWorkspaceMember(ctx WorkspaceContext, userID int64) error
func (s *Store) WorkspaceMembers(ctx WorkspaceContext) ([]WorkspaceMember, error)
func (s *Store) WorkspaceMonitors(ctx WorkspaceContext) (*MonitorRepository, error)
```

The existing MonitorRepository methods listed above retain their signatures.
`TenantMonitors(TenantContext)` is now **explicitly company-admin-only for both
reads and writes**, including operations on a previously obtained repository.
Viewer/operator company membership cannot use it to bypass Workspace grants.
Use `WorkspaceMonitors` for department-scoped reads/writes. Trusted authentication
must still supply UserID; these structs are not credentials or HTTP bindings.

### Authorization contract

- A live, unrestricted company admin administers every Workspace in that company,
  without requiring a department grant. Company membership/account disable or
  revocation denies all access, including through a previously obtained handle.
- Other active company members require an explicit, enabled Workspace membership.
  The department role is independent of the company role: the same company viewer
  can be department-A admin and department-B viewer. Workspace admin may rename,
  delete an empty nondefault Workspace, manage its memberships, and write its
  monitors/state/history. Viewer/operator may read, but cannot grant themselves
  admin or mutate monitors. Membership listing is administrator-only.
- Creating a Workspace requires company admin. Listing Workspaces shows only live
  grants except for an explicitly authorized company admin. An empty WorkspaceID
  is never a wildcard. Wrong company/department/user, missing resources and
  revoked/disabled access return `ErrTenantAccess`.
- Each repository operation rechecks authority inside the immediate core
  transaction; membership is not cached. Existing global area restrictions fail
  closed rather than being broadened. Default legacy engine behavior is unchanged.
- Membership writes do not modify company roles or company membership. Targets
  must have active company membership. Removal cascades on company-membership or
  account deletion; rejoining does not resurrect old nondefault department grants.
- Identity/ownership/default flags are immutable. Default Workspace deletion is
  prohibited. Deleting a Workspace containing any monitor identity (even retired)
  or stored state fails rather than destroying stable history ownership.

### Migration and data boundaries

- Every existing company receives one persisted random default Workspace; future
  trusted tenant insertion creates one atomically. Existing company memberships
  are backfilled with their exact role into that default only. User/account and
  company disabled flags are checked live, independently of Workspace suspension.
- Existing user creation/role paths continue synchronizing default membership.
  No promotion is copied to nondefault Workspace grants. Existing legacy area
  restrictions are neither removed nor reinterpreted as department grants.
- Existing monitor IDs, retired IDs, sequence values, rounds and aggregate history
  remain unchanged. Monitors gain immutable `workspace_id`. Trusted legacy SQL
  inserts that omit it resolve the company's default, not a wildcard; all scoped
  repository insertions supply explicit ownership.
- State is rebuilt transactionally with primary key
  `(tenant_id,workspace_id,name)`, backfilled to each company's default. This is
  necessary because a retired name can later be used in another department:
  updating/deleting the new monitor must not overwrite the previous department's
  state. Active monitor-name uniqueness remains **company-scoped**, unchanged.
- Workspace sync selects, updates, revives and retires only that Workspace's
  identities. Identical definition/address hashes never deduplicate monitors.
  A conflicting active name in another department fails atomically. Deliberate
  company-admin `TenantMonitors.SyncMonitors` may retire across departments;
  definitions it newly creates belong to the company's default Workspace.
- Legacy Store monitor/cache/state/history adapters remain restricted to the
  default company/default Workspace. Per-company compatibility caches contain
  **only default-Workspace IDs**; nondefault Workspace repositories use authorized
  SQL rather than shared name/alias caches. Aggregation stays keyed by stable ID.
- All scoped history methods use the same Workspace-aware owned-ID lookup.
  Workspace state writes reject unscoped incident references. Company-admin
  repositories likewise cannot attach global incident IDs to nondefault Workspace
  monitors. The legacy default path retains its existing incident behavior.
- Version-16 upgrades use `core.db.bak-16`; the whole migration/version change is
  atomic. Tests inject failure after all migration SQL, check schema rollback and
  backup, retry through real Open, and reopen again with IDs/state/history intact.

### Observed RED → GREEN and adversarial review

Commands used the supplied Go toolchain, scratch TEMP/TMP/TMPDIR, and
`go test -p 1 ./internal/store -run '^TESTNAME$' -count=1`.

| Test | Observed RED | Final GREEN |
|---|---|---|
| `TestWorkspaceSchema` | `no such table: workspaces` | default schema/backfill |
| `TestWorkspaceAuthorization` | missing Workspace API compilation errors | CRUD/live grants |
| `TestWorkspaceMonitors` | missing WorkspaceMonitors compilation errors | mixed roles, guessed IDs, scoped sync |
| `TestWorkspaceRetiredStateOwnership` | B overwrote A's retired state | Workspace-keyed state/delete |
| `TestWorkspaceCompanyRepositoryIncidentBoundary` | unscoped incident accepted | actual Workspace checked |
| `TestWorkspaceRetiredNameDefaultReuse` | default reuse state empty | active-owner legacy guard |

The last three cases came from adversarial code review, not only happy-path
requirements. Additional regression coverage includes late migration failure,
real v16 upgrade/reopen, post-write restart, injected retirement/state/grant/rename
failures with unchanged readback, independent company/Workspace roles, invalid
roles, disabled account/company/department, removal and rejoin, wrong-company
scopes, immutable identities, default wrappers and explicit company-wide admin.
Those additional tests are regression evidence, not claimed as separate RED cycles.
The older tenant-viewer tests were intentionally updated to require denial from
company-wide repositories; department viewer reads remain explicitly tested.

Final integrated-tree package verification (`-p 1 -count=1 -json`), top-level /
including subtests: store **58 / 62**, app **41 / 59**, server **83 / 135**, engine
**122 / 149**, config **39 / 51**. Three real in-process Zone runtime scenarios
passed: standalone, restart, invalid CAS/recovery. `go vet -p 1` passed for all
five packages. Existing Windows skips remain: symlink privilege and shell-script
handler platform. Logs: Hermes scratch `workspace-packages-final.jsonl` and
`workspace-zone-final.jsonl`. Disk free after verification was approximately
1.4 GiB. No commit/release or production probes/notifications were performed.

### Still outside this checkpoint

No server/UI/app/runtime activation or product switch was added. The integration
call sites listed above still need authenticated scope propagation, configuration
ownership, isolated restore/apply/sink/event routing, and real API/UI acceptance.
Incidents, secrets, push tokens, intents/deliveries, audit, configuration snapshots,
Zone storage, public grants and probe/result/task ownership remain unscoped.
Importer apply is still a global-admin path and was not edited. No secrets/probes
are shared automatically. Company-wide `States()` is not a runtime restore DTO:
retired-name states can exist in multiple departments and must not be fed into the
current name-keyed global engine. Metadata/repository boundaries are not complete
product isolation. The parent's three frozen integrated instances and extracted
package acceptance remain mandatory and **were not performed by this worker**.
