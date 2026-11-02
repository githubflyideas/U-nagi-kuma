# Windows candidate package screenshots

These six captures use the **extracted v1.5.7-rc.1 Windows amd64 candidate package**, not a development working-tree rebuild or a final release. The genuine runtime version is `v1.5.7-rc.1`; no version override was used. The binary SHA-256 is `90f294cc8c5080c57c3847e986f115d1705b59d382304f5308628ab4c55ac2c5`, matching `docs/package-acceptance-candidate.json`.

- Real Go executable and Playwright browser; no mocked APIs, injected CSS, altered DOM text, or composited product imagery.
- Disposable setup account, configuration and database, removed after capture. HTTP probes target only a loopback fixture: two healthy endpoints and one intentional 503. A passive PUSH monitor waits for a heartbeat.
- Every notification destination is a local log channel. Automatic daily reporting and local-network probes are disabled. Demo names are illustrative, not production customers or incidents.
- Midnight and paper dashboards use a 1440 × 1000 viewport. The onboarding card is dismissed using its actual Hide button. The on-call screen uses the actual Week control. Other pages are full-page captures; the mobile viewport is 390 × 844.
- Image sizes and dimensions are checked against the new capture manifest. No lossless-optimization or before/after pixel-equality claim is made for this capture set. No synthetic version or status values were inserted.

## Reproduce

From the repository root with Go, Node and Playwright Chromium installed, set `UNAGIKUMA_E2E_BIN` to the extracted candidate executable and `UNAGIKUMA_EXPECT_VERSION` to `v1.5.7-rc.1` before running:

```sh
node scripts/capture-readme.mjs /path/to/go /path/to/disposable-scratch
```

With `UNAGIKUMA_E2E_BIN` set, the script uses the supplied executable without rebuilding it, starts a loopback-only server, creates the demo through real configuration/setup, verifies the report API, captures six images, records provenance and stops/removes the disposable runtime. Without that variable it rebuilds with `CGO_ENABLED=0 go build -p 1 -trimpath`; that fallback is not the source of these candidate captures. Use a scratch directory with enough free space for the disposable runtime (and Go's build and one executable if rebuilding). Rotations use UTC so the fixture does not require host IANA timezone data.

`capture-manifest.json` records source revision, dirty state, tracked product diff hash, binary hash, build invocation, capture time, dimensions and loopback/data boundaries. For this capture, `packagedBinary` is true and `buildInvocation` is `No build: supplied extracted package binary`. The dirty source state describes the capture checkout, not a source rebuild of this executable. The tracked diff check is not a complete frozen-source archive and does not fingerprint untracked source files. Any later final release must be checked against its own frozen artifact.

`documentation-verification.json` records image-link, unchanged fenced-command, image-size/dimension and candidate-binary hash checks for this capture set. Re-running capture replaces the PNGs and capture manifest; repeat these checks and visual review before updating this verification record. Historical compression and visual-review claims do not carry over automatically.

## Integration boundary

Zone synchronization, invitations, voting and recovery documentation must be reviewed against the final integrated runtime and its tests. These images do not demonstrate distributed consensus, full HA or script upload. Keep preview wording until those acceptance criteria pass. The candidate acceptance record covers Windows amd64 only: 12 top-level tests and 8 component subtests passed, but only five tests exercise the extracted executable; component tests use browser fixtures and runtime engine tests execute source in process. This is not full multi-platform acceptance, Windows SCM install/recovery acceptance, or an official tagged/published release. No production data, external notification provider or extracted candidate executable was modified by capture.
