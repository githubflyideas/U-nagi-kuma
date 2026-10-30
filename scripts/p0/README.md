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
