# Zone/discovery recovery status

This is a recovered library-level implementation, not a completed autonomous cluster. No commit or push was made by the recovery worker.

## Implemented contracts

- Opt-in `config.ParseZone` accepts a public `$ZONE` header after SOA; legacy parsing remains separate. Body hash and SOA serial are derived, not trusted from portable metadata.
- `ZoneStore` imports new independent local identities, retains origin without granting trust, rejects name collisions, and supports explicit revision-checked updates with advancing SOA serial. Secondary records are read-only to ordinary import. Rebinding atomically changes source and generation; remote sources require a SHA-256 fingerprint. Recovery added the same binding validation on disk reads.
- Storage checks IDs, regular files, bounded reads/imports, body hashes, generation overflow and binding coherence. Serialized writers share a process-local directory lock; this is not a multi-process database or protection against a malicious local administrator. Persistence uses temporary-file sync and rename; no backup/restore workflow or cross-platform crash-durability guarantee is claimed.
- `ZoneStore.Export` emits public metadata and parsed records, strips comments and local trust metadata, preserves exact secret/environment references and returns sorted missing dependencies. Non-allowlisted option values become collision-avoiding secret placeholders; their values are never resolved. The generated configuration is parsed again before returning it.
- Export deliberately fails closed for opaque monitor targets (userinfo/query/path/fragment), mixed literal credential references, unsupported target types and directives. Currently only bare HTTP(S) monitor URLs and channel-kind targets are accepted. It does not promise general lossless export of every supported configuration. Cross-zone dependency resolution and local node mapping remain unimplemented.
- Discovery reconciles the applied static NODE registry, not network-discovered peers, TCP sessions or valid observation votes. Existing tunable defaults to 5m, accepts 1m–24h, and supports explicit 1h; no default was changed to 1h. Apply reconciles immediately; delayed periodic ticks reconcile once.
- Recovery fixed the discovery test lifecycle: real `Engine.Run`, synchronized fake clock, commands on the engine loop, incident sink, cancellation and join. Production synchronization was not bypassed. Tests cover deadline timing, offline retention, unknown-peer rejection, session churn, detached snapshots, delayed ticks, immediate reload/removal and stopped-engine queries.

## Verification

Reproduced missing `Export` compilation errors and the pre-Run discovery deadlock. Added a RED persisted-binding corruption test, then shared validation between Rebind and Get.

Targeted command (Go requires `-p 1`, not `-p1`):

```
go test -p 1 ./internal/config ./internal/app ./internal/engine ./internal/tunables -run 'Zone|Discovery' -count=1 -timeout=120s -v
```

All packages passed. Symlink-record test skipped because Windows denied symlink creation privileges; regular-file rejection passed. Other store coverage includes concurrent CAS, persistence/reopen, rejected-write immutability, generation overflow, traversal/size checks and hash corruption.

Full requested packages:

```
go test -p 1 ./internal/config ./internal/app ./internal/engine ./internal/tunables -count=1 -timeout=180s
```

PASS: config 0.504s, app 15.022s, engine 96.230s, tunables 0.154s. Free space before/after: 2456854528 / 2443018240 bytes. Final full run explicitly used Hermes cache/scratch for TMP/TEMP/TMPDIR; inherited environment had pointed TMPDIR at Windows Temp during earlier targeted runs. No race build, repository-wide suite or multi-process autonomous-runtime proof was performed.

The interrupted `internal/engine/.hermes-tmp.6Z5vmE` is zero bytes, contains nothing recoverable, and was left untouched; exclude it from commits. No empty Go source files were found in the four requested packages. Prior partial edits were preserved.

## Remaining integration work

- Wire ZoneStore into application lifecycle and authenticated/authorized zone API/UI; add previews, backups, dependency mapping and missing-secret readiness reporting.
- Implement authenticated same-zone synchronization, source/generation/hash checks, last-good recovery, invitations/ACLs and explicit source-rebinding confirmation. Metadata alone grants no remote authority.
- Expose discovery metadata through API/UI with configured-registry wording; no API consumer currently calls Discovery.
- Implement and verify autonomous per-node checks/local notifications independently of source availability, observation freshness/definition filtering, dynamic valid-vote aggregation and source-failure multi-process/restart scenarios. Existing central agent scheduling is not replaced by these fields.
- Parent must freeze and verify the entire integrated tree, review remaining unrelated edits and choose commit scope. Do not describe these package passes as full cluster completion.
