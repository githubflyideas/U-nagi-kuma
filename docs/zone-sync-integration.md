# Zone synchronization integration contract

Backend APIs are in `internal/app/zone_sync.go` and `zone_sync_loop.go`.
These do NOT mount themselves in the production server. No server/UI file is owned by this change.

## Source server wiring (required)

Mount `a.ZoneSyncHandler()` at **GET `/api/v1/zone-sync`**, on the listener serving `a.PKI().TLSConfig(...)`. The client requests the existing cluster SNI (`agent.unagikuma.invalid`) and verifies the pinned CA chain, expiry and hostname. Do not send it through browser session/CSRF or legacy agent/standby authorization; the handler does its own zone-generation-scoped bearer authorization. Preserve listener connection/read-header limits and add the normal admission/rate limits. The handler limits concurrent requests to four and its response write deadline to ten seconds.

The handler returns only the exact applied snapshot, never a newer draft. No CA/private key or whole-node state is transmitted. Authorized bodies are sensitive configuration and can include operator-authored literal values; use local `${secret:...}` references for credentials that must not be distributed. Import/export redaction is a separate workflow.

Local authenticated zone administrators call:

- `GrantZoneSync(zoneID, subscriberNode)` => one-time plaintext capability, store securely on the subscriber; only its SHA-256 hash is stored at source. Reissuing rotates it. Its scope is source zone ID + source generation + subscriber name, not an agent permission.
- `RevokeZoneSync(zoneID, subscriberNode)` => deny subsequent requests. No inbound remote mutation endpoint exists.

Source must explicitly `RebindZone(..., ZoneBinding{SourceNode: localMName})` and `ApplyZone` before serving. Changing source generation invalidates all previous grants; changing draft serial does not publish until apply.

## Subscriber wiring (required)

1. Create a local draft using ImportZone (even an empty valid SOA is enough). Import metadata NEVER establishes trust.
2. Explicit authorized `RebindZone` with local ID/current revision and all of:
   - `SourceNode`: expected remote node name
   - `Fingerprint`: exact 64 hexadecimal SHA-256 CA fingerprint (no `sha256:` prefix)
   - `SourceZoneID`: expected remote zone UUID
   - `SourceGeneration`: expected remote generation
3. Keep endpoint and capability in local operator settings/secret storage, not imported configuration. Endpoint is a TLS server origin (existing NormalizeServer semantics, default port 8077).
4. `SyncZone(ctx, localZoneID, endpoint, token)` performs one bounded fetch and validated apply.
5. For ongoing sync, own one blocking `RunZoneSync(ctx, localZoneID, ZoneSyncOptions{Endpoint: ..., Token: ..., Interval: ...})` worker per zone. Default 5 minutes; supported 1 second to 1 hour. Launch with the application lifetime, cancel AND join before stopping App.Run or replacing credentials/endpoint. Failures retry with 1s..1m capped exponential backoff; request timeout 10s. The worker is deliberately NOT started automatically, because no endpoint/token persistence or server lifecycle owner has yet been wired.

Rebinding increments the local generation and clears the authenticated remote serial checkpoint. The request captures the entire local revision before network IO; apply rejects any intervening rebind/edit. Remote ID, generation, source name, exact body hash and SOA serial must match; equal serial/different body and backwards/oversized serial jumps are rejected. A newly bound source may start at a lower serial: the old source checkpoint no longer grants authority.

SQLite stores the exact last-good applied snapshot before scheduling. Missing local dependencies or rejected response preserve it, including restart. Draft and SQLite are separate storage boundaries: SQLite failure after draft persistence can leave a newer draft, but running/restart snapshot stays last-good; exact retry can complete safely. No response acknowledgement claims execution/delivery.

## NODE behavior and remaining gap

Zone NODE records/monitor assignments are authorization for THIS full local application, not remote-agent routing. Explicit monitor assignments run only if they contain local base SOA MName. Empty assignments retain existing zone-local behavior. Other assigned monitors are omitted from this node's runtime; a locally assigned child requiring a nonlocal parent is rejected. Base-file legacy NODE/agent semantics are unchanged.

No shared-observation exchange or voting has been added. A monitor assigned to several nodes currently runs/checks/notifies independently on each authorized node. There is no identity/definition/freshness-matched observation collection, shared insufficient-evidence UI, cross-node deduplication, or observation replay suppression. Missing source/node evidence is not converted into a target failure by this new path. Discovery remains separate from configuration polling.

## Evidence and release boundary

`e2e/zone_sync_test.go:TestZoneSyncSourceLossAndSecondaryRestart` uses independent real applications/SQLite/PKIs, actual pinned authenticated TLS loopback transfer, real probes/log sender, source shutdown, secondary offline restart, forbidden-assignment exclusion, and problem/recovery delivery. It does not prove the production route is mounted or workers are started by the server. Those wiring tasks and real-server verification remain mandatory. No commit is authorized here; parent owns the user's three-final-round gate.
