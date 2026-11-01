# Bounded Checkmate / Gatus imports

These adapters are wired through **Configuration → Import**, the existing admin-only preview/apply API, and the CLI. Kuma and Nagios remain available. This is a conservative HTTP subset, not complete cross-product migration.

```sh
unagikuma import checkmate --dry-run --data ./data monitors.json
unagikuma import gatus --dry-run --data ./data config.yaml
# Review the report, then repeat without --dry-run to apply.
```

API: POST the original file bytes to `/api/v1/import/checkmate` or `/api/v1/import/gatus` to preview; add `?apply=1` to apply. Existing authentication, administrator authorization and CSRF requirements apply. API previews return `items`, `notes`, credential-free generated `text`, and `blocked: true` if any item cannot be safely imported. Apply independently reparses/rechecks the input and rejects blocked imports before secrets/configuration writes. The UI disables Apply for blocked previews. CLI blocked apply prints the report and fails. Dry-run still writes reports, as before.

## Supported subset

| Source | Accepted file and mapping |
|---|---|
| Checkmate | **bluewave-labs/Checkmate Settings → Export monitors**, downloaded `monitors.json` **array**; not API envelopes, database dumps, or another project named Checkmate. HTTP only. Names, groups, method, whole-second millisecond intervals, active/paused state, TLS verification flag, bearer `secret`, and 2xx **plus** `customUpCodes` are mapped. `proxyMode` must be `none`; status-window size must be 1, threshold 1–100. Timeout is 30 seconds. |
| Gatus | One YAML document containing `endpoints`. HTTP/HTTPS only, exactly one literal `[STATUS] == N` condition (100–599). Names, groups, interval (default 60s), enabled (default true), method (default GET), literal body and headers are mapped. `client.timeout` (default 10s), `client.insecure`, and `client.ignore-redirect` are mapped. |

Intervals/timeouts must be whole seconds from 1 second through 24 hours. Names are converted to record identifiers and retained in `title`; different monitor names may use the **same address** without deduplication. Any collision with another generated or existing record name **blocks apply**: explicitly rename the source item rather than silently overwriting, skipping, merging or renaming. Reimport is therefore safe but not a successful no-op for these two adapters.

## Fail-closed limits

- At most **1 MiB**, 1 document, 1,000 endpoints/monitors, nesting depth 32 and 20,000 YAML nodes. Duplicate/non-string keys, anchors, aliases, merges, custom tags and malformed schemas are rejected with sanitized errors.
- No environment expansion, file reads, directory traversal, include loading, external secret lookup or remote requests. Unresolved `$...`, `${...}`, template and known Gatus runtime placeholders block the monitor. YAML is parsed as data, never evaluated.
- URLs containing user information, **any query string**, fragments or placeholders are deliberately blocked so tokens cannot leak through target previews/logs. Move those checks manually using destination secret controls.
- All imported Gatus header values/body and Checkmate bearer credentials become encrypted destination secrets through the existing secret store. Previews/reports contain secret references only. Source token/credential values are never copied into diagnostics.
- Unsupported Gatus conditions (including AND/OR lists, body/JSON assertions, response-time predicates, DNS/TCP/ICMP/SSH conditions), endpoint/client keys, OAuth, proxies, TLS file references, alerts, suites, external endpoints, includes and top-level settings block apply. There is no silent fallback to HTTP 2xx.
- Checkmate non-HTTP types, advanced matching, inherited/custom proxy, geographic checks, rolling status windows, unresolved notification/tag IDs and unknown monitor fields block apply. The source export does not contain the related configuration needed to reconstruct these safely.
- Known source observations, IDs, timestamps and hardware metadata are discarded. Users, tenants, team membership, accounts, status history, push tokens, notification channels and source authority are **never** created.

## Scope and limitations

The existing import route is an administrator operation writing the **global monitors.bind configuration**, with optional destination `$SCOPE`/`area`. An area is **not tenant isolation**. This feature does not introduce tenant-scoped activation or claim tenant safety. Until authorized tenant destination plumbing is available, use only the existing trusted-admin global workflow. Checkmate/Gatus cannot grant area-manager authority.

HTTP client implementations differ: Checkmate's transport retries, redirect/cookie handling and status aggregation are not a complete runtime clone. The adapter maps the listed request fields and success status predicate only; validate behavior before production use. Notifications, global settings and non-HTTP probes require explicit manual migration, not deletion of warnings to obtain a falsely equivalent check.

Pinned upstream commits and exact primary schema/export references are in `internal/importer/testdata/ADAPTERS.md`. Synthetic fixtures are not claims of compatibility with every upstream version. Tests cover parsing, generated configuration validation, CLI dry-run/apply, real localhost API preview/apply with credential masking and rejected writes, and Chromium against the disposable real server. Imported integration fixtures are disabled: tests do not send real probes or notifications.

Verification commands:

```sh
go test -p 1 ./internal/importer ./internal/server ./cmd/unagikuma ./internal/i18n
go test -p 1 -tags e2e ./e2e -run '^TestImportAdaptersUI$' -count=1 -v
```
