# Unagikuma

<p align="center"><img src="docs/images/logo.png" alt="Unagikuma" width="240"></p>

Self-hosted monitoring in a single binary: the density of a Nagios console with the ease of Uptime Kuma.

[中文说明](README.zh-CN.md) · [User manual](docs/manual.md) · [Changelog](CHANGELOG.md)

![A tour of the web UI: the overview with where the network breaks, the monitor list, a monitor's drawer, the command palette, notification policies, monitors.bind and a public status page](docs/images/tour.gif)

## Why Unagikuma

- **One file, no dependencies.** A static Go binary with the web UI and SQLite built in. Copy it to a machine, run it, open the browser.
- **Readable, versioned configuration.** Everything lives in `monitors.bind`, a plain text file in the spirit of DNS zone files. Edit it in the browser or in your editor; every applied version is kept and can be restored. The syntax is [specified and stable](docs/spec/monitors-bind.md) across 1.x.
- **Checks that know the protocol.** HTTP(S) with keyword, JSON and certificate checks; TCP with TLS; ICMP ping; Redis, PostgreSQL, MySQL, gRPC health and Docker containers; DNS; domain registration expiry; and UDP checks that speak the real protocol of well-known services (`10.0.0.1:ntp`, `:snmp`, `:tftp`, `:syslog`, …). Nagios plugins run unchanged as script checks. Each check states how much it proves (L1 sent, L2 port answers, L3 service answered correctly).
- **Quiet by design.** Soft/hard states, dependencies (`parent=`), maintenance windows, acknowledgements, silences and storm summaries. When this host's own network, DNS or clock is the problem, Unagikuma notices and holds alerts instead of paging you for every target, and every monitor says in words why it did or did not notify.
- **Alerting that grows with you.** Feishu/Lark, DingTalk, WeCom, Telegram, Slack, Discord, e-mail, webhooks with a dozen presets, and SMS and voice calls through Aliyun, Tencent Cloud and Twilio. Fallback chains, time periods, contact groups, escalation of unacknowledged incidents, event handlers and a full delivery log.
- **Alerts reach the right people.** A contact directory keeps channels honest: someone who leaves stops getting alerts (and loses their login), addresses that bounce are dropped, unknown ones are flagged. On-call rotations page only whoever is on call, with stand-ins and MTTA/MTTR. Planned work is announced, reminded, held in maintenance and closed with a receipt.
- **Public status pages** with announcements, upcoming maintenance, e-mail subscriptions, an Atom feed and their own host name; **push monitors** compatible with Uptime Kuma push URLs; **badges**.
- **More vantage points when you want them.** Run the same binary as an agent in another office or data center; checks are then decided by a quorum of nodes, and a failure seen from only one site stays a quiet regional note. A standby takes over the master role with one click.
- **Easy to move in.** Import Uptime Kuma databases and Nagios object configurations; paste a list of addresses into the command palette (Ctrl+K).
- **Every threshold is a tunable**, and the UI speaks **15 languages** (English, 中文, हिन्दी, Español, العربية, Français, বাংলা, Português, Русский, Bahasa Indonesia, اردو, Deutsch, 日本語, Tiếng Việt, 한국어), notifications too.

Probe nodes and master handover, the importers, script checks, sections, the audit log and the translations other than English and Chinese are marked experimental: they work and are tested, but may change.

## Quick start

Download the archive for your platform from the [releases](../../releases), unpack it and run:

```sh
./unagikuma            # listens on :8077, keeps its data in ./data
```

Open `http://<host>:8077/` and create the admin account. When you open the UI from another machine, Unagikuma asks for the one-time setup token it printed to its log at first start.

Add a monitor in the UI, use **Quick add monitors** to paste a list of targets, or edit `data/monitors.bind`.

### In a container

```sh
docker run -d --name unagikuma -p 8077:8077 -v unagikuma-data:/data ghcr.io/githubflyideas/unagikuma:latest
```

The image holds only the static binary; data lives in `/data`. For ICMP checks the host must allow unprivileged ping (`net.ipv4.ping_group_range`, Docker's default inside containers), or add `--cap-add NET_RAW --user 0`.

### As a service (Linux, systemd)

```ini
# /etc/systemd/system/unagikuma.service
[Unit]
Description=unagikuma monitoring
After=network-online.target
Wants=network-online.target

[Service]
ExecStart=/usr/local/bin/unagikuma server --data /var/lib/unagikuma
Restart=on-failure
DynamicUser=yes
StateDirectory=unagikuma
AmbientCapabilities=CAP_NET_RAW

[Install]
WantedBy=multi-user.target
```

`CAP_NET_RAW` is only needed for ICMP ping on systems where unprivileged ping sockets are disabled (`net.ipv4.ping_group_range`). Without either, ping checks report that this host lacks the privilege rather than marking targets down.

## Next steps

The [user manual](docs/manual.md) covers everything else:

- [writing monitors.bind](docs/manual.md#monitorsbind) and [script checks](docs/manual.md#script-checks);
- [alerting](docs/manual.md#alerting): channels, policies, time periods, contact groups, escalation, SMS and voice, event handlers;
- [status pages](docs/manual.md#status-pages), [probe nodes and the master role](docs/manual.md#probe-nodes), [users and sections](docs/manual.md#users);
- [moving from Uptime Kuma or Nagios](docs/manual.md#moving-from-uptime-kuma-or-nagios), [backups](docs/manual.md#data-and-backups), the [command line](docs/manual.md#command-line) and the [API](docs/manual.md#api);
- [troubleshooting](docs/manual.md#troubleshooting).

## Building from source

Go 1.27.3 or later, no C compiler needed:

```sh
make build     # bin/unagikuma
make test
make dist      # release archives for all platforms in dist/
```

End-to-end tests run the built program as separate processes: `go test -tags e2e ./e2e/` (the browser tests need Node.js and Playwright).

## License

Apache License 2.0, see [LICENSE](LICENSE).
