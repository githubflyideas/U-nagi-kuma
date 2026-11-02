# Extracted Windows package acceptance

**Recorded outcome:** the final full tagged suite passed: 12 top-level tests, 8 browser-component subtests, zero failures/skips, in 345.921 seconds. Five top-level tests exercised the extracted executable; six exercised source/in-process code; one grouped the eight browser fixtures. The final report is `C:/Users/fengyongxiao.LAPTOP-05HDB6FR/package-acceptance/v1.5.7-rc.1/final-verification/report.json`; earlier failures remain preserved and explained below. The coordination manifest contains the unchanged archive/binary hashes and absolute paths.

This is **candidate v1.5.7-rc.1**, not a final or published release. The local runner builds Windows/amd64 only, preserving the Windows branch of `scripts/dist.sh`: `CGO_ENABLED=0 GOOS=windows GOARCH=amd64 GOARM=7`, `go build -trimpath -ldflags "-s -w -X main.version=v1.5.7-rc.1"`, the top-level versioned directory, executable, LICENSE, every README*.md, generated THIRD_PARTY_LICENSES.txt, both manuals, and docs/spec/monitors-bind.md. Python ZIP replaces the `zip` CLI without changing the required file set. This does not claim bit-for-bit reproduction of a remote CI ZIP.

`release.yml` calls `scripts/dist.sh "$TAG"`; its manual dispatch validates stable versions only. This local candidate bypasses neither a publication gate nor tags anything: it does **not** invoke that publishing workflow, upload artifacts, commit, or push.

## Repeat

From Git Bash at repository root, put the intended Go toolchain on PATH and set `GOTOOLCHAIN=go1.27.3`. Set TMPDIR, TEMP, and TMP to your Hermes scratch directory (not system temp). Node, Playwright and its Chromium browser must already be installed. Then run:

```sh
python scripts/package-acceptance-windows.py --version v1.5.7-rc.1 --output C:/path/to/durable/package-acceptance/new-run
```

Use a fresh durable output directory. The runner refuses to overwrite an archive. Its retained `extracted-*` directory is a fresh extraction scratch workspace, not a production installation and not automatically deleted. The archive, extracted binary, stage, logs, JSON test events, screenshot evidence, SHA256SUMS and report stay outside ephemeral temp. Tests use disposable data directories and loopback targets; no Windows service is installed. Candidate documentation screenshots are the existing repository images: final screenshots are a separate capture using this exact extracted executable.

`docs/package-acceptance-candidate.json` is the coordination manifest. It and the durable `report.json` contain absolute artifact paths, hashes, invocation arguments, individual command exit codes, test results, scoped coverage and limitations. Read `status`, not just the presence of artifact paths. Failed acceptance preserves the package and diagnostics.

## Strict external-executable opt-in

```sh
UNAGIKUMA_E2E_BIN='C:/path/to/extracted/unagikuma.exe' \
UNAGIKUMA_E2E_UI=1 UNAGIKUMA_E2E_OUT='C:/path/to/evidence' \
go test -tags e2e -count=1 -timeout=25m -json ./e2e
```

`TestMain` checks whether the variable is **present**, not merely nonempty. Explicit empty, nonexistent, directory, nonexecutable, failing-version, or unexpected-version-output paths fail before tests and never rebuild. Valid paths are made absolute and version-probed with a timeout; the exact path and runtime version are printed. With the variable absent, the pre-existing source-build mode remains available and is explicitly logged. The supplied executable is never removed by TestMain.

Regression test:

```sh
go test -tags e2e -c -o C:/path/to/e2e.test.exe ./e2e
python scripts/package-acceptance-harness-test.py C:/path/to/e2e.test.exe
```

The black-box regression exercises empty, missing, directory and nonexecutable paths. Before the harness change all four silently built from source and returned PASS (retained `harness-red.log`); afterward all four reject the opt-in (retained `harness-green.log`). Successful candidate execution is the positive integration case.

## Retest without rebuilding

After a harness-only correction, retain the original report and all original logs. Verify the existing archive/binary hashes and rerun the full suite without rebuilding:

```sh
python scripts/package-acceptance-retest.py --report C:/path/to/initial/report.json --label harness-fixed
```

The label must name a new subdirectory. Its report links back to the initial report; the coordination manifest points to the latest completed result. A green retest never erases the initial failed run.

### Windows harness diagnosis in this candidate run

The first full run failed in standby setup with a transport-level status 0 while the runtime also reported process stalls. The existing helper does not preserve the transport error, so this first symptom alone does not establish a product defect.

An unchanged isolated `TestFailover -count=3` then reproduced a separate deterministic failure three times: after successful promotion the Windows process exited as documented, but the generic test assumed Unix in-process reexec and waited for a process that no longer existed. `cmd/unagikuma/reexec_windows.go` deliberately exits 3 for a supervisor. The harness now verifies exit 3 and restart intent, restarts exactly once, checks intent consumption, and retains all existing credential, monitor, term and old-master retirement assertions. This is an explicit **subprocess supervisor**, not SCM acceptance.

With supervision present, the old-master retirement assertion exposed a second harness scheduling boundary: `internal/app/app.go` invokes retirement at startup and then every minute. The startup call occurs before the old master is restarted, so a 60-second test deadline races the next attempt. Captured logs show retirement actually occurring just after that deadline. Only that wait now allows two one-minute intervals; the global default and all state assertions remain unchanged. See retained `failover-isolated.log`, `failover-supervised.log` and the retest report for actual results. No product source was edited and the original archive/binary were not rebuilt.

Additional diagnostics are preserved rather than hidden: `failover-fixed.log` records a transient failure of the strict version probe; the immediate direct native probe then succeeded in approximately 0.095 seconds with the same binary hash (`version-probe-diagnostic.json`). The `harness-fixed` full run passed all five extracted-executable tests but failed the source-only report browser fixture on its 2500 ms navigation timeout. The unchanged report fixture subsequently passed all nine of its tests in isolation (`report-ui-isolated-native.log`). A Git Bash direct Node invocation also failed with `stdin is not a tty`; launching the native Node process via Python avoided that shell-wrapper issue without modifying tests. `final-verification/report.json` records the final full-suite attempt separately. These observations do not prove an underlying product defect or erase earlier failures.

## Coverage boundaries

| Test | What actually runs |
|---|---|
| TestFailover | Extracted executable master/standby processes, real loopback failover and restoration |
| TestStandbyWindowsSupervisedRecovery | Extracted executable, real exit 3 and explicit subprocess restart; **not SCM installation/recovery** |
| TestUI | Chromium against extracted executable, UI and importer workflows; failure-only screenshot directory (empty on the final passing run) |
| TestZonesUI | Chromium against extracted executable, live zone workflow and loopback probe |
| TestImportAdaptersUI | Chromium against extracted executable, import preview/apply workflows |
| TestUIComponents and eight subtests | Source browser scripts with fixture/mocked APIs; **not external package backend coverage** |
| Workspace runtime/schedule and zone runtime/sync tests | Source engine/application in-process tests with loopback fixtures; **not extracted executable coverage** |

The command runs the complete tagged e2e package, including untagged tests selected within it. The report classifies results rather than equating all tagged tests with package coverage. The runner rejects any failed or skipped test and verifies that each expected external-executable test passed. Archive entries are checked against stage hashes after extraction, runtime version is checked, and archive/binary SHA-256 are checked again after tests.

No acceptance claim is made for Linux, macOS, FreeBSD, Windows ARM64, Docker, actual Windows SCM, production traffic, or every possible product behavior. A passing report means this existing Windows/amd64 acceptance suite passed against the specified candidate archive.
