# Local-outage test rig

Runs Unagikuma inside a network namespace and cuts its uplink, to check that
a host losing its own network sends one message, not a flood. Linux only;
needs `sudo` for the namespaces.

```sh
go build -o /tmp/p0rig ./scripts/p0/rig
CGO_ENABLED=0 go build -o bin/unagikuma ./cmd/unagikuma
RIG=/tmp/p0rig LOGS=/tmp/p0 scripts/p0/harness.sh up
BIN=$PWD/bin/unagikuma LOGS=/tmp/p0 scripts/p0/scenarios.sh 2   # 1-13, see the script
RIG=/tmp/p0rig LOGS=/tmp/p0 scripts/p0/harness.sh down
```

`harness.sh` builds three namespaces (this host, a router, "the internet"
with HTTP targets, a counting webhook and a DNS server) and can cut the
uplink silently (`cut`/`restore`), break DNS only (`dns-cut`), stop the
gateway answering ping (`gw-ping off`) or take one target down
(`host-down IP`). `scenarios.sh N` runs one scenario and prints every
message the webhook received.

## Last results (2026-10-30)

Messages are what the webhook received for the whole scenario.

| # | Scenario | Messages |
|---|---|---|
| 1 | 3-minute cut, 9 monitors, default timings | 0 (shorter than `localnet.notify-after`) |
| 2 | 3-minute cut, short timings 10s/5s/2 | 1 summary |
| 3 | a monitor down before the cut | its alert before, then 1 summary naming it |
| 4 | a target breaking during the cut | 1 summary + 1 alert for it |
| 5 | 1000 monitors, 10% failing, then a cut | 3 alerts + 1 summary |
| 6 | kill -9 during the cut, restarted during it | 1 summary, also naming the crash |
| 7 | only 3 monitors | 1 summary |
| 8 | flapping uplink, 5 × (25 s down, 20 s up) | 1 summary |
| 9 | gateway that ignores ping | 1 summary |
| 10 | DNS resolver down, address checks fine | 1 early alert + resolver notice, its recovery and summary |
| 11 | process stopped for 90 s | 0 |
| 12 | `localnet.max-hold=1m`, 3-minute cut | 1 summary, also naming the release |
| 13 | herd after the cut (1000 monitors) | peak 2.9× the steady rate for 2 s |

A wall-clock jump is not part of the rig (it would change the machine's
clock); it is covered by the engine's unit tests.
