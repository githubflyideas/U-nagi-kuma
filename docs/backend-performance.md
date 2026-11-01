# Reproducible backend resource baseline

## Commands

From the repository root, using Python 3 with `psutil` installed and an already cached Go toolchain/dependencies:

```sh
python scripts/backend-performance.py --go C:/Users/fengyongxiao.LAPTOP-05HDB6FR/toolchains/go/bin/go.exe --baseline-source test-results/baseline-source
C:/Users/fengyongxiao.LAPTOP-05HDB6FR/toolchains/go/bin/go.exe test ./internal/app -run '^$' -bench '^BenchmarkCalendarWindow$' -benchmem -count=3 -benchtime=500ms
```

`test-results/baseline-source` is the existing export of commit `9e7b8c1d856b11bfb5541134ef8230875986fa27`. The harness rebuilds both baseline and working tree using identical flags:

```text
CGO_ENABLED=0 GOPROXY=off GOSUMDB=off
go build -trimpath -buildvcs=false -ldflags "-s -w -X main.version=performance" -o <executable> ./cmd/unagikuma
```

It resolves the installed Go toolchain before disabling module network access. The old bootstrap executable is Go 1.22.12; the actual cached toolchain is Go 1.27.3. Setting GOSUMDB=off on the bootstrap executable initially prevented auto-toolchain checksum verification; using the resolved cached toolchain fixed the build without downloading anything.

To measure existing binaries without rebuilding, omit `--baseline-source` and optionally pass `--baseline` and `--current`. This mode records embedded build information but callers must confirm equivalent build flags. The application itself needs no Python or psutil; these are measurement-tool dependencies only.

## Evidence and method

- `test-results/backend-performance.json`: complete raw observations, binary SHA-256/build metadata, live file inventories, every API timing, process CPU deltas, and environment.
- `test-results/backend-calendar-benchmark.txt`: three repeated isolated Go benchmark samples per case.
- `scripts/backend-performance.py`: reusable runner; assertions reject unsuccessful responses or incorrect fixture counts.
- `internal/app/oncall_performance_test.go`: isolated calendar and Next benchmarks, with segment-count correctness checks.

Windows 11 build 26200, amd64, Intel i5-1235U, 10 physical / 12 logical CPUs, 16,847,527,936 bytes RAM, Go 1.27.3, Python 3.14.7, psutil 7.2.2. Three independent processes per build per scenario (12 processes total), alternating build order. Each has two seconds settle and ten seconds idle observation sampled at approximately 500 ms. Each receives a fresh temporary data directory under the configured scratch directory, binds only an OS-assigned loopback port, and is terminated and cleaned afterward. No running user service, user configuration, external provider or monitor target is contacted.

Empty scenario uses the application's genuine first-run SOA-only configuration without account setup. Representative scenario contains 24 log-only member channels, six daily UTC rotation groups and six explicit policies, zero monitors, zero incidents, zero overrides. The policies API also returns two implicit policies; the runner verifies the six explicitly configured policies. No outbound providers are configured. API setup creates a disposable account with a random password. Four authenticated GET endpoints each receive five warmups and 50 measured sequential requests per process over one persistent HTTP/1.1 loopback connection. Calendar covers November 1–29, 2026. These are end-to-end client timings, not isolated handler CPU times.

## Measured results

RSS is the median of the three per-process median working-set measurements. API timings are the median of three per-process medians (each from 50 requests), not pooled percentiles.

| Metric | Baseline | Current |
|---|---:|---:|
| Executable bytes | 21,039,104 | 21,084,160 |
| Empty RSS (MiB) | 26.414 | 25.949 |
| Representative idle RSS (MiB) | 26.430 | 26.445 |
| Empty initialized live data bytes, observed range | 579,196–579,200 | 579,200–579,204 |
| Empty process CPU seconds over 10 s, three runs | 0 / 0 / 0 | 0.078125 / 0 / 0 |
| Representative process CPU seconds over 10 s, three runs | 0 / 0 / 0 | 0.015625 / 0 / 0 |
| GET channels median ms | 2.4002 | 2.6787 |
| GET policies median ms | 2.3519 | 2.2694 |
| GET oncall median ms | 3.6477 | 3.7391 |
| GET oncall/calendar median ms | 14.4963 | 14.9782 |

Binary growth is 45,056 bytes (+0.214%). Includes the working-tree embedded UI/security changes at build time; this is not a backend-code-only size attribution. Binary hashes in the JSON identify the measured artifacts even if other work continues afterward.

All 12 observations had **zero idle data-file growth and zero handle-count growth**. The live footprint includes SQLite WAL and SHM: representative empty baseline core WAL 428,512 bytes and history WAL 74,192 bytes, plus two 32,768-byte SHM files. Tiny total-byte variations arise in generated certificate files. File sizes are logical lengths, not filesystem allocated blocks or post-checkpoint size. Windows process stop is forced, so do not interpret these as graceful-shutdown disk measurements.

CPU increments are quantized (observed 15.625 ms units); a reported zero means below accounting resolution over this interval, not literally no instructions. The short runs cannot establish absence of long-term leaks, history retention problems, or loaded monitoring costs. Post-setup RSS is temporarily inflated by account setup/password hashing; the JSON keeps pre/post-request observations but they must not be treated as isolated API heap growth. Shared-host scheduling/power state and the relatively broad per-run timing ranges preclude claiming small deltas as regressions or improvements.

## Confirmed hotspot: eager 4,000-shift calendar expansion

No production code was changed. The existing calendar implementation in `internal/app/oncall_calendar.go:103` calls `r.Next(from, 4000, loc)` before breaking when the requested end date is reached. `internal/oncall/oncall.go:241–248` eagerly allocates and fills all 4,000 shifts. Thus even a one-day calendar request builds roughly eleven years of daily shifts for each rotation.

Isolated benchmark (three repetitions, one daily UTC rotation):

| Case | Observed ns/op range | Bytes/op | Allocs/op |
|---|---:|---:|---:|
| calendar, 1 day | 1,845,293–1,965,371 | 262,424–262,466 | 7 |
| calendar, 28 days | 1,731,522–1,890,452 | 271,400 | 16 |
| calendar, 92 days | 1,755,765–2,002,875 | 301,864 | 20 |
| Next(29), control only | 11,984–15,636 | 2,048 | 1 |
| Next(4000), current inner call | 1,440,401–1,567,404 | 262,144 | 1 |

The median Next(4000) control takes about 119 times the CPU time and 128 times the allocation bytes of Next(29). This is an isolated operation comparison, **not a promised end-to-end speedup**. The nearly invariant one-/28-/92-day calendar cost supports the eager-expansion diagnosis. Both implicated source files are byte-identical to the baseline export, so this is pre-existing rather than introduced by the UI/security edits. Per-request allocation churn is demonstrated; persistent memory leakage is not.

A prospective fix should generate shifts only until the required end boundary, preserving whole first/last shifts, arbitrary rotation lengths, DST, overrides and the 92-day limit. Do not simply replace 4,000 with 29. Root cause is reported for review before any production modification. Existing targeted on-call/calendar tests passed with the added benchmark file.

## Limits

No race run: no GCC/cgo in this environment. No CPU affinity, power profile control, production incident history, concurrency/load saturation, network monitors, external providers, multi-hour soak or filesystem block accounting. Three fresh samples are sufficient to establish a reproducible small-instance baseline, not a production capacity claim. Results under `test-results/` and binaries under `bin/` are ignored by Git; include the evidence explicitly when packaging a report.
