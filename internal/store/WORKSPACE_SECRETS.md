# Scoped secrets / runtime secret authority checkpoint (migration 18)

**Update:** migration 19 adds callable App-level owned configuration publication
and encrypted supervisor restart binding; see `WORKSPACE_CONFIG.md`. That newer
checkpoint supersedes the persistence/app-wiring gaps below, but still does not
activate a department scheduler or expose HTTP/UI/import routes.

This is a bounded prerequisite, **not department runtime activation**. There is
still one App and shared core/history SQLite. No HTTP/UI, engine, importer or
runtime call site was changed. Earlier `TENANCY.md` / runtime-boundary notes that
call all secrets global describe the pre-18 checkpoint; their other activation
blockers remain open.

## Callable APIs

```go
func (s *Store) WorkspaceSecrets(ctx WorkspaceContext, kek []byte) (*WorkspaceSecretRepository, error)
func (r *WorkspaceSecretRepository) Put(name, value string, scopes []string) error
func (r *WorkspaceSecretRepository) Get(name string) (string, error)
func (r *WorkspaceSecretRepository) Delete(name string) error
func (r *WorkspaceSecretRepository) List() ([]SecretInfo, error)

func (s *Store) IssueWorkspaceSecretReader(ctx WorkspaceContext) (string, error)
func (s *Store) RevokeWorkspaceSecretReader(ctx WorkspaceContext) error
func (s *Store) SetWorkspaceRuntimeActive(ctx WorkspaceContext, active bool) error
func (s *Store) RuntimeSecrets(credential string, kek []byte) (*RuntimeSecretReader, error)
func (r *RuntimeSecretReader) Get(name string) (string, error)
func (r *RuntimeSecretReader) Resolve(input string) (string, error)
```

`WorkspaceContext.UserID` is trusted authenticated principal plumbing, **never**
source/import/body identity. All interactive operations, including plaintext
reads and metadata listing, require a live unrestricted company admin or enabled
Workspace admin with active company membership/account. Authorization and each
operation occur inside the existing immediate core transaction. A saved handle
is not a cached membership grant. `scopes` remains descriptive legacy metadata,
not a role or resource-sharing grant. List returns the existing `SecretInfo` DTO
(name, scopes, timestamp), never encrypted material, key IDs or plaintext.

Service authority is deliberately narrower than general runtime ownership:

- Issue generates a random 256-bit opaque credential, returns it once, and stores
  only SHA-256. It is **not** a users/tokens/session identity. No admin UserID is
  retained. The reader has private immutable tenant/workspace/generation binding
  and exactly two read methods: `Get` and `Resolve`; no grant, membership,
  secret/config mutation, arbitrary scope or monitor/history write methods.
- One current service-secret generation exists per Workspace. Issue replaces the
  previous credential. Revoke clears the hash and advances generation. Every read
  rechecks generation, hash, active flag and live Workspace existence in the same
  transaction that decrypts values. Revocation does not depend on the issuer's
  account subsequently remaining enabled.
- `SetWorkspaceRuntimeActive` controls **only this service-secret boundary**, not
  the legacy scheduler or all Workspace APIs. Every call invalidates old
  credentials. Disabling prevents issuance and reads; enabling never resurrects
  a credential. First issuance initializes an absent policy as active. No policy
  or credential means no service access. Interactive admins may still repair
  secrets while service access is inactive.
- `Get` takes a reference name, never a scope. `Resolve` expands `${secret:NAME}`
  under one authorized snapshot. Missing names return `ErrNotFound`; env/unknown/
  malformed placeholders return `ErrSecretReference`. Neither uses default/global
  secrets or ambient environment. Errors return an empty result, not partial
  plaintext. Substituted secret values are not recursively interpreted.
- Missing/revoked/foreign authority returns `ErrTenantAccess`. A zero/nil reader
  fails closed. Credentials must be saved by a future trusted service supervisor
  to rebind after restart; **no automatic credential file or startup wiring was
  added**. Do not put credentials in user-facing configuration/DTOs/logs.

Already disclosed plaintext cannot be recalled. Future runtime integration must
recheck this authority at execution admission and invalidate/cancel cached
compiled credentials on revocation; resolving once at compile time is not enough
to claim that a long-running scheduler obeys revocation.

## Migration, compatibility and rotation

`workspace_secret_schema.go` appends migration 18. It transactionally rebuilds
`secrets` with primary key `(tenant_id, workspace_id, name)` and a composite
Workspace FK. Existing ciphertext, DEKs, key IDs, metadata and timestamps are
copied unchanged to the immutable default company/default Workspace. No KEK is
required for migration; monitor/history identities are untouched. Ownership/name
updates are prohibited. A Workspace with secrets cannot be deleted implicitly.

Legacy `Secrets.Put/Get/Has/Delete/List` are now explicitly default-only. Existing
secret reference names and legacy import behavior are preserved. A nondefault
secret with the same name neither appears in legacy listing nor prevents a
legacy default import. No implicit cross-workspace sharing is supported.

`workspace_secret_readers` stores the immutable composite owner, active flag,
monotonically increasing integer generation and nullable credential hash. Its FK
cascades on Workspace deletion. It stores no user identity. Generation/hash/
inactive state survives restart; deleted workspaces deny old readers.

`Secrets.Rotate` remains a **trusted installation-wide operation**, not a
Workspace-admin or service method. It rewraps *all* workspace DEKs in one
transaction using the complete owner/name key; using name alone would corrupt
same-name secrets across departments. The same installation KEK must be supplied
to all scoped repositories. Rotation does not re-encrypt values or change service
generations. Existing external repository/reader handles retain their old KEK:
**quiesce all writers/readers, rotate, persist the new installation key using the
existing operator procedure, and recreate all handles before resuming**. Do not
reuse old-key writers. Atomic coordination of the DB rotation and external key
file, automatic hot-rebinding and crash-safe key-file rotation are not introduced
by this checkpoint.

A version-17 upgrade backs up to `core.db.bak-17` using the existing migration
machinery. Upgrade and version update are atomic; late injected failure restores
the original table and version. Older-version upgrade/reopen and default secrets
are covered by package regressions. Downgrade needs a stopped application and
matching pre-upgrade DB/key backup, not a live down-migration.

## Observed verification

Genuine RED -> GREEN cases:

- `TestWorkspaceSecretsIsolation`: missing typed repository API -> isolated A/B
  same-reference CRUD and explicit default compatibility.
- `TestWorkspaceSecretsRotation`: `cannot decrypt secret` after name-only rotation
  -> full ownership-key rewrap preserves A/B/default values.
- `TestWorkspaceRuntimeSecretAuthority`: missing service API -> durable narrow
  authority, guessed ID denial, inactive/revoke denial, no captured admin UserID.
- `TestWorkspaceRuntimeResolver`: missing Resolve API -> strict scoped expansion,
  env/missing/malformed rejection and revoked literal denial.

Additional regression tests cover current roles/disabled user and membership,
revoked grants, safe metadata DTO and reader serialization, nil handles, generation
replacement, deleted Workspace, ciphertext storage, failed put/delete/lifecycle
writes, late rotation rollback, version-17 backup/late migration rollback/retry,
reopen, persisted inactive/revoked credentials and live rebind. These additions
are regression evidence, not claimed as separate RED cycles.

Commands (repository root; supplied Windows Go, scratch TEMP/TMP/TMPDIR):

```bash
GO='C:/Users/fengyongxiao.LAPTOP-05HDB6FR/toolchains/go/bin/go.exe'
export TMP='C:/Users/fengyongxiao.LAPTOP-05HDB6FR/AppData/Local/hermes/cache/scratch'
export TEMP="$TMP" TMPDIR="$TMP"
"$GO" test -p 1 ./internal/store ./internal/app ./internal/importer ./internal/server ./cmd/unagikuma -count=1 -json
"$GO" vet -p 1 ./internal/store ./internal/app ./internal/importer ./internal/server ./cmd/unagikuma
"$GO" test -p 1 ./e2e -run '^(TestWorkspaceRuntime|TestZoneRuntime)' -count=1 -json
```

All passed against the integrated tree. Pass events, top-level / with subtests:
store **69 / 73**, app **46 / 64**, importer **16 / 47**, server **83 / 135**,
command **3 / 5**. Existing Windows symlink-privilege app test skipped.
Four real in-process e2e tests passed: legacy Workspace activation rejection plus
Zone standalone, restart and invalid-CAS/recovery. These prove default continuity,
**not successful department execution**. Evidence in Hermes scratch:
`workspace-secret-packages-final.jsonl`, `workspace-secret-e2e-final.jsonl`.

## Remaining parent-owned wiring

1. Authenticated Workspace selection and admin HTTP mutations; never trust body
   UserID. Use the scoped repository rather than `App.SecretStore()` for future
   department operations. Existing imports remain global-admin/default-only.
2. Trusted supervisor credential persistence, restart rebind via `RuntimeSecrets`,
   generation lifecycle and cancellation; bind `reader.Resolve` into the future
   workspace compile path, **not** `App.resolver()` / `App.Resolver()` (which still
   intentionally permit legacy environment references).
3. Atomic owned configuration publication/monitor registration, scoped engine
   identities and state/sink/event/incident/notification ownership. This service
   capability intentionally grants none of those write permissions.
4. Real department A/B same-target execution with separate credentials, history,
   failure/alerts and restart semantics; authenticated UI/API activation only
   after those boundaries are trusted. No runtime/UI activation in this patch.
5. Parent's three distinct frozen-tree instances and extracted real-package gate
   remain unperformed. No commit/release, production calls or user-data deletion.
