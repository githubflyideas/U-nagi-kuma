# Workspace applied configuration + restart binding (migration 19)

**Later execution checkpoint:** `docs/workspace-execution-checkpoint.md` documents
verified bounded HTTP checks, scoped state/history and cancellation in the same
App. The persistence-only limitations below describe migration 19 itself, not
that newer callable execution path. Notifications and product exposure remain blocked.

This checkpoint adds a **callable App-level durable configuration stage**, not a
Workspace scheduler, HTTP route, importer, or UI activation. One App and the
existing shared SQLite stores remain. Default config/Zone APIs and imports are
unchanged. The older runtime-boundary and migration-18 notes predate this stage.

## Exact callable APIs

```go
// internal/app — ctx.UserID must come from trusted authenticated identity.
func (a *App) PreviewWorkspaceConfig(ctx store.WorkspaceContext, raw []byte, expected int64) (store.WorkspaceConfigState, error)
func (a *App) ApplyWorkspaceConfig(ctx store.WorkspaceContext, raw []byte, expected int64) (store.WorkspaceConfigState, error)
func (a *App) WorkspaceConfigState(ctx store.WorkspaceContext) (store.WorkspaceConfigState, error)

// Trusted installation supervisor ONLY; not a user-facing DTO/endpoint.
func (a *App) RestoreWorkspaceConfig(tenant store.TenantID, workspace store.WorkspaceID, generation int64) (*config.Config, *store.RuntimeSecretReader, error)

// internal/store — Secrets is the existing installation-KEK handle.
type WorkspaceConfigState struct {
    TenantID TenantID
    WorkspaceID WorkspaceID
    Revision int64
    Generation int64
}
type WorkspaceConfigValidator func(body []byte, resolve func(string) (string, error)) error
func (s *Store) WorkspaceConfigState(ctx WorkspaceContext) (WorkspaceConfigState, error)
func (x *Secrets) PreviewWorkspaceConfig(ctx WorkspaceContext, body []byte, expected int64, validate WorkspaceConfigValidator) (WorkspaceConfigState, error)
func (x *Secrets) ApplyWorkspaceConfig(ctx WorkspaceContext, body []byte, expected int64, validate WorkspaceConfigValidator) (WorkspaceConfigState, error)
func (x *Secrets) RestoreWorkspaceConfig(tenant TenantID, workspace WorkspaceID, generation int64, validate WorkspaceConfigValidator) ([]byte, *RuntimeSecretReader, error)
```

Store validators are trusted App glue, not caller-supplied policy. They execute
inside the immediate core transaction: no nested Store calls, external I/O,
retained resolver, or plaintext error messages. Use the App methods for validated
configuration admission. Returned restore models/body are private supervisor
input, not HTTP DTOs; literal user-provided config may itself contain credentials.
The opaque service capability is never returned by these APIs.

## Admission and publication contract

- Preview, apply and state read require a live unrestricted company admin or
  active Workspace admin, with active company membership/account. No implicit
  scope or global secret fallback. These contexts are not authentication tokens.
- Expected revision **0** means create only. Every publication increments the
  durable revision and advances the service generation; stale CAS fails. A
  successful preview returns the current state, not a future admission ticket.
- Authorization, CAS, runtime-active policy, strict scoped secret decryption,
  typed config validation, new capability issuance and encrypted snapshot
  publication share one immediate SQLite transaction. Validation happens before
  credential replacement. Late snapshot-write failures roll back replacement as
  well as snapshot writes; prior credentials remain usable.
- Config is bounded to 1 MiB. `config.Load` validates typed monitor parents,
  policies, channels/groups, escalation, periods and maintenance within the owned
  model. Resolved probes/channels are validated without network calls. NODE and
  STATUSPAGE records, host handlers and rename aliases are rejected at this
  checkpoint rather than inheriting global effects.
- Only `${secret:NAME}` with `[A-Za-z0-9_.-]+` names is accepted. Environment,
  malformed and unknown references fail closed, including references in comments.
  Values are not recursively expanded. The reader's Resolve now shares this
  strict grammar with admission, matching the config compiler's grammar.
- Stored configuration contains original references, not their expanded values.
  Migration 19 adds `workspace_applied_configs` keyed by company/Workspace. The
  entire original body and random service credential are AES-GCM envelope
  encrypted using a random DEK wrapped by the installation KEK. The authenticated
  encrypted payload also contains the exact company/Workspace/revision/generation;
  swapping ciphertext between departments cannot rebind authority.
- `workspace_secret_readers` continues storing only the credential hash, active
  flag and generation. No interactive UserID is persisted in service authority.
  Metadata DTOs contain only the two IDs, revision and generation.
- Trusted installation-wide `Secrets.Rotate` rewraps both secret and configuration
  DEKs in the same transaction. Rotation preserves revision/generation. Existing
  quiesce/key-file coordination requirements still apply; hot rotation and atomic
  external key-file replacement are not introduced here.

## Recovery and revocation

Restore requires an exact requested company/Workspace/generation and a matching
current active service hash/generation. It decrypts and checks the payload binding,
then repeats typed/resolved validation in the same transaction. Missing secrets
block restore without mutating the stored snapshot. Wrong keys, swapped payloads,
revoked/replaced capabilities and inactive Workspaces fail closed. Enabling a
Workspace cannot resurrect an old stored capability; publish again using the
current config revision. Removing the issuing admin's grant does not revoke
already issued service authority; explicit service revoke/inactive controls do.

An issued reader checks live authority on every Get/Resolve. Plaintext already
returned cannot be recalled. Nothing here admits executions or cancels compiled
probes. State metadata may describe a saved snapshot whose service authority has
since been revoked; it is not an active-runtime status DTO.

## Concrete next scheduler hook (not implemented)

A trusted supervisor owning an exact `(TenantID, WorkspaceID, Generation)` calls
`App.RestoreWorkspaceConfig` on restart/replacement and supplies the returned
reader's `Resolve` to a future scoped compiler (the current translation seam is
`internal/app/build.go: engineConfigResolved`). Before connecting it, the parent
must add stable scoped engine keys/typed edge mapping, scoped monitor registration
and sink/incident/notification/event ownership. Do **not** pass this model to
`installRuntime` or `publishRuntime`: those remain default-only.

The supervisor must cancel and join the previous generation and recheck authority
at each execution admission, rather than caching a one-time resolved credential.
Scope/generation discovery, automatic startup workers, execution fencing and
cancellation are not implemented. No fake runtime success is returned by apply.

## Verification observed

Genuine RED -> GREEN:

- `TestWorkspaceConfigPublishRecovery`: missing App methods -> authorized A/B
  publication, readback, scoped same-reference values and actual App reopen.
- `TestWorkspaceConfigKeyRotation`: restore failed with tenant access denied
  after KEK rotation -> configuration and service payload DEKs rotate atomically.
- `TestWorkspaceConfigStrictReferenceGrammar`: malformed but existing reference
  was admitted -> resolver/compiler reference grammar aligned.

Additional regression coverage: concurrent CAS (one winner), preview/apply
permission denial, revoked admin grants, foreign-only/missing/env references,
invalid syntax, missing typed edges, resolved invalid targets with secret-safe
errors, unchanged prior snapshot/credential after rejection or injected late
failure, exact generation/revoke/inactive across App restart, no default-secret
fallback, encrypted payload inspection, ciphertext scope substitution, wrong KEK,
late rotation rollback, real v18 migration rollback/backup/retry/publication/reopen.
Tests use loopback configuration and do not start a department scheduler.

Final supplied Windows Go commands used scratch TEMP/TMP/TMPDIR, `-p 1 -count=1`:

- Store/App/importer/server/command package suites passed; pass events top-level /
  including subtests: **73/77, 51/83, 16/47, 83/135, 3/5** respectively.
- `go vet -p 1` passed for those five packages.
- Four real in-process e2e regressions passed: Workspace legacy activation
  rejection, Zone standalone, restart, invalid-CAS/recovery. These demonstrate
  default continuity, not department execution.
- Existing Windows symlink-privilege test skipped; no new skip introduced.
- Raw evidence: Hermes scratch `workspace-config-packages-final.jsonl` and
  `workspace-config-e2e-final.jsonl`. Disk free after checks: approximately 748 MiB.

No commit, release, user-data deletion, external probes or notifications. These
checks do not replace the parent's three distinct frozen-tree instances or the
extracted release-package acceptance gates.
