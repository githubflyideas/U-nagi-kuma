# Current working-tree screenshots

These are **unreleased working-tree UI** captures, not a frozen v1.5.7 release claim. The application was rebuilt from source without overriding its version (`dev` is deliberately visible).

- Real Go executable and Playwright browser; no mocked APIs, injected CSS, altered DOM text, or composited product imagery.
- Disposable setup account, configuration and database, removed after capture. HTTP probes target only a loopback fixture: two healthy endpoints and one intentional 503. A passive PUSH monitor waits for a heartbeat.
- Every notification destination is a local log channel. Automatic daily reporting and local-network probes are disabled. Demo names are illustrative, not production customers or incidents.
- Midnight and paper dashboards use a 1440 × 1000 viewport. The onboarding card is dismissed using its actual Hide button. The on-call screen uses the actual Week control. Other pages are full-page captures; the mobile viewport is 390 × 844.
- PNGs retain their original pixels and dimensions; only lossless compression was applied. No synthetic version or status values were inserted.

## Reproduce

From the repository root with Go, Node and Playwright Chromium installed:

```sh
node scripts/capture-readme.mjs /path/to/go /path/to/disposable-scratch
```

The script rebuilds with `CGO_ENABLED=0 go build -p 1 -trimpath`, starts a loopback-only server, creates the demo through real configuration/setup, verifies the report API, captures six images, records provenance and stops/removes the disposable runtime. Use a scratch directory with enough free space for Go's build and one executable. Rotations use UTC so the fixture does not require host IANA timezone data.

`capture-manifest.json` records source revision, dirty state, tracked product diff hash, binary hash, build invocation, capture time, dimensions and loopback/data boundaries. The tracked diff check catches concurrent tracked-source edits during build; it is not a complete frozen-source archive and does not fingerprint untracked source files. Final release imagery must be recaptured after all runtime integration is frozen.

`documentation-verification.json` records image-link, unchanged installation-command and pixel-preservation checks. Re-running capture replaces the PNGs and capture manifest; repeat optimization/link/visual checks before updating this verification record.

## Integration boundary

Zone synchronization, invitations, voting and recovery documentation must be reviewed against the final integrated runtime and its tests. These images do not demonstrate distributed consensus, full HA or script upload. Keep preview wording until those acceptance criteria pass. No production data, external notification provider, downloaded release executable or existing E2E script was modified.
