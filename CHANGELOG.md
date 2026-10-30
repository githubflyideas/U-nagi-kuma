# Changelog

## Unreleased

**New**
- Moving from Uptime Kuma or Nagios is no longer a preview. `unagikuma import kuma|nagios` prints a clear report (imported / needs a look / not imported, each with what to do) and saves it as text and Markdown in `<data>/import-reports/`. `--dry-run` checks everything without changing anything; running it again adds only what is new.
- Uptime Kuma: 1.23 and 2.x databases (also while it runs), nested groups, maintenance windows, status pages, push URLs that keep working, ntfy, Gotify, Pushover, Teams, PagerDuty and more notification types, passwords and tokens stored as secrets. On a new installation the first Uptime Kuma account becomes the administrator.
- Nagios: point it at `nagios.cfg` and the files it names and `resource.cfg` are read; quoted Debian plugin arguments, passive checks (push monitors), DNS, NTP and SNMP checks are understood; plugins come over paused until they are in the check-scripts directory.
- The imported records go in one tidy block at the end of `monitors.bind`, in sections headed by where they came from, aligned like `unagikuma fmt`.
- `unagikuma reset-password` asks for the new password twice without showing it, lists the users when the name is wrong, and `--generate` makes a random one.

- Incident reports: each incident has a report of what led up to it, how it was detected, who was told when and on which channel (and what was held or merged, and why), and how it recovered, with whether this host was watching all along. Open it from **Report** on the Problems page, download it as Markdown, HTML or JSON, or write it with `unagikuma report incident ID`.
- Every decision about an alert (queued, sent, failed, held, released, merged, dropped, suppressed, acknowledged) is recorded, so a missing or late alert can be explained afterwards.
- Problem notices say what kind of failure it is: the service itself, a network segment, not certain yet, or this host's network. Webhooks get it as `class`.
- A channel sends at most 3 alerts a minute (`notify.backstop-max`, `notify.backstop-window`); the rest come as one summary at the end of the minute. New problems, escalations and system notices always go out.
- The audit log is tamper-evident: `unagikuma audit verify` and **Check integrity** find changed, inserted or removed entries. It also records sign-ins, backups, restores, imports, password resets and what the system did by itself, highlights high-risk entries, filters by risk, address and time, and exports CSV or JSON (`unagikuma audit export`).
- `unagikuma reset-password --revoke-tokens` also revokes that user's API tokens.

**Fixed**
- A local outage whose last reply and first failure came at the same instant was not recognised, and its failures were alerted.
- A problem that began while the disk was full got no recovery notice.
- The local-outage summary gives the time the network came back, not the time that was confirmed a minute later.
- `unagikuma check` now finds channel settings the server would refuse.
- A monitor added to a group from the web UI is aligned like that group's records.

## v1.5.1 — 2026-10-29

**Upgrading from v1.0.0**
- The database gets a table for on-call stand-ins on the first start; v1.0.0 then refuses to open it, so make a backup first (`unagikuma backup`) if you may want to go back.
- Announced changes, status pages and the daily report moved from Alerting to **Notices & status**; bookmarks to their old addresses still work.

**New**
- On call: `rotate=daily|weekly|Nd|Nh` on a contact group makes it a rotation that notifies only whoever is on call, and tries the others in turn only when that person cannot be reached; `handoff="mon 09:00"` sets when shifts change and `start=` the date from which the first member is on call. The **On call** page shows who is on call now and the next shifts, creates rotations, arranges stand-ins (someone covers for a while; the order stays), and measures MTTA and MTTR over `oncall.stats-window` (30 days), counting each incident for whoever was on call when it opened.
- The Problems page lists setup problems that keep alerts from reaching anyone: no channel, a default policy without channels, recipients missing from the contacts, broken check scripts. Its menu entry gets a red dot while there are any.
- The monitor list: tick monitors (or all of them) to check them now or delete them together; each row has an Edit button, and clicking a row opens the monitor's page. **Add monitor** and **Quick add monitors** sit together. `POST /api/v1/monitors/bulk-delete` deletes several monitors in one configuration change.
- The internet gateway of *Where it breaks* is set under **Settings › General** (8.8.8.8 by default, empty turns it off); its label on the path view links there.
- The sidebar lists Monitors before Problems, and On call and **Notices & status** get their own entries.
- **Configuration › monitors.bind** keeps only the copy button; the error banner offers to view the configuration rather than to edit it.
- A new logo: the eel and the bear.

## v1.0.0 — 2026-10-29

The first stable release. The `monitors.bind` syntax is frozen as version 1 (see `docs/spec/monitors-bind.md`): later 1.x releases read every file this one accepts the same way.

**Upgrading from v0.2.0**
- Make a backup first (`unagikuma backup`, or Settings › Backup) if you may want to go back: the history moves to a new layout on the first start, which takes a moment on large installations, and older versions then refuse to open it. A copy of the previous `core.db` is kept as `core.db.bak-N`.
- Monitors get a stable id. Renaming one in the web UI or the API keeps its history, incidents and push token; when renaming by hand in the file, write `was=OLD` once.
- The container image is `ghcr.io/githubflyideas/unagikuma`.
- Addresses used by e-mail, SMS and voice channels get a red dot until they are in the new contact directory: **Alerting › Contacts › Import them** adds them all at once. They are still sent to meanwhile.
- Uptime shows "—" with the share watched until a monitor has been watched for `uptime.min-coverage` (10%) of a window, instead of a figure from a few minutes of data.
- The "unhandled" count now includes regional and low-priority incidents and no longer counts UNKNOWN monitors, so the sidebar, the overview and the problems page always agree.
- One recipient that does not exist no longer fails a whole e-mail; SMS and voice channels send number by number.

**New in 1.0**
- Contact directory (**Alerting › Contacts**): channels only send to people in it; someone marked as left stops receiving alerts and their login and API tokens stop working; addresses refused as nonexistent `contacts.bounce-threshold` times (3) are no longer sent to; unknown addresses are flagged with a red dot on the channel, the policies using it and the Alerting menu. Channels can name people as `@handle`. New addresses are offered for the directory when a channel is saved, and `unagikuma check` reports the same problems.
- Announced changes (**Alerting › Changes**): planned work is announced through a policy or channels, reminded before it starts (`changes.reminders`), holds the monitors it touches in maintenance, can post a status page notice, and ends with a receipt (done, rolled back, cancelled). Searchable history with CSV export; the notice templates can be replaced.
- `DOMAIN` checks watch when a domain's registration expires (RDAP, or WHOIS where a registry has none); **Monitors › Expiry dates** lists every domain and certificate.
- Status pages render their first screen on the server: they read at once, without JavaScript and in search results. Status pages can be created from the UI.
- Changing a failing check (timeout, keyword, target) keeps its incident instead of closing it and paging again for the same outage.
- Acknowledgements are kept as a history with who, when, through what and the note; the first acknowledgement time is kept.
- API: every record has an etag for `If-Match` (412 when someone else changed it), creates accept `Idempotency-Key`, monitor lists page with `limit`/`cursor` in stable id order, and `/monitors/id:N` addresses a monitor by id.
- Writing back from the UI keeps option order, quoting and column alignment; `$VERSION 1`; `unagikuma fmt [--check]` and `unagikuma check --json`; URLs with a query are always targets.
- `/metrics` describes the monitoring host too: rounds started, dropped and late, history writes, database size, memory, live connections. Every engine event is also one structured log line.
- Nodes announce their check types: a node too old for a check is not sent it and is shown as needing an upgrade.
- The raw history shrinks to `storage.low-disk-raw` while the disk is nearly full; script directories given relative to the working directory work.
- The first-run guide adds registering who receives alerts and publishing a status page.
- The monitor form: the check type is a dropdown grouped by kind; host and port are separate fields, and an empty port uses the type's default without writing it into the target; DNS checks have their own fields for the name to look up, the record type and the expected answer; the group field suggests the existing groups.
- **Configuration › monitors.bind** shows the file read-only, with copy and download; monitors, channels and policies are changed on their own pages. **Import** shows Uptime Kuma and Nagios as two sources to pick from.
- *Where it breaks* draws the internet gateway (`localnet.exit-target`) as a small mark under the internet targets. It is pinged only once an internet target fails; while they all answer it is shown as not checked. The line to the internal targets shows as working when the gateway cannot be judged but those targets answer.

**Also since v0.2.0**
- A first-run guide on the overview: add the first monitors in one box, add a channel, put it in the default policy; each step ticks itself off.
- The command palette (Ctrl+K): type an address to add it, `host:?` for the UDP keywords, `ack NAME`, `silence NAME 1h`, `check NAME`, or part of a name to open a monitor.
- The monitor drawer says why a monitor did or did not notify: maintenance, its parent is down, this host's own outage, only some nodes failing, soft state, acknowledged, silenced, the notification limit, a policy without channels.
- The monitor page header no longer overlaps on phones.
- Service checks: `REDIS` (login, PONG, optional role=master/replica), `POSTGRES` (md5 or SCRAM login, SELECT 1, ssl=yes), `MYSQL` (native or caching_sha2 login, ping, ssl=yes), `GRPC` (the standard health check), and `DOCKER` (a container on each probing node: running and healthy). Without credentials the database checks prove the server answers. Uptime Kuma's Redis, PostgreSQL, MySQL, Docker and gRPC monitors are imported as these (without passwords).
- DNS checks can require an exact answer set with `expect-all=`, so an added or replaced record fails.
- Time periods: `office IN PERIOD "mon-fri 09:00-18:00; sat 10:00-14:00"`, and `period=office` (or `period=!office`) on a channel to use it only inside (or outside) them.
- Contact groups: `ops IN CHANNEL group members=feishu,oncall-sms` notifies all members at once and succeeds when any of them does.
- Escalation: `escalate=30m:leader,2h:director+sms` on a policy notifies more channels while an incident stays open and unacknowledged.
- Event handlers: `handler="restart.sh $SERVICESTATE$ $SERVICESTATETYPE$"` on a monitor runs a script from the check-scripts directory on each state change, with the Nagios macros and `UNAGIKUMA_*` variables; only administrators may set it.
- SMS and voice calls through Aliyun, Tencent Cloud and Twilio.
- Status pages: announcements with severity and resolution, upcoming maintenance, e-mail subscriptions with double opt-in (`subscribe=`), an Atom feed, and an own host name (`domain=`) that serves nothing but the page.
- Licence quotas: a licence that limits nodes or accounts refuses adding more beyond it (when adding a node, a machine joining, creating an account or importing accounts); nothing already there is switched off. Settings › Registration shows the usage. Without a licence there is no limit.
- A user manual in English and Chinese (`docs/manual.md`, also in the release archives), and a shorter README with a tour of the UI.
- Fixed: the overview and the monitor list stayed blank once a channel had delivered notifications (a missing import in the channel statistics).
- A standby whose copy of the master's state fails tries again after `cluster.standby-retry` (30 seconds, three times) instead of waiting for the next sync.
- End-to-end tests in CI: browser tests of the web UI (Playwright) and a master failover with real processes (`go test -tags e2e ./e2e/`).
- Nagios import (`unagikuma import nagios DIR`, or Configuration › Import): hosts, services, commands, contacts, contact groups, time periods and event handlers, with templates resolved. Common plugins become native checks, others EXEC checks running the same plugin; services depend on their host.

## v0.2.0 — 2026-10-28

- The event log and the monitor timeline read in the UI language: events record a message code and its values, and the UI writes the text. Events recorded by earlier versions keep their English text.
- Known issue: the overview and the monitor list go blank once a channel has delivered notifications; fixed in v0.3.0.

## v0.1.1 — 2026-10-28

**Upgrading from v0.1.0**
- The default port is now **8077** (8066 collided with another common tool). Pass `--listen :8066` to keep the old one; agents installed with the old address need their `--server` URL updated.
- Built with Go 1.27.3; no reachable known vulnerabilities in the dependencies.
- API: latency is reported as `latency_ms` and captured packets as `request_hex`/`response_hex`.

**Fixes**
- Secret values no longer appear in evidence, details or the packet viewer, wherever they occur in the request or response.
- A host started without network no longer sends one alert per monitor; a dead site on a working network still alerts at once.
- New, redefined or resumed PUSH monitors get a full period before they are late.
- Status page `select=` accepts the same selectors as maintenance windows.
- URLs with a query string (`?a=b`) can be saved from the UI and the API.
- Paused and maintenance states show the same everywhere; a slow monitor that fails no longer sends "degraded ended" first.
- Quorum percentages, durations and numeric options refuse NaN, infinities, negative and overflowing values.

**New**
- `/healthz` for load balancers and containers, `/metrics` in the Prometheus format.
- A new console look, dark by default: the overview shows where a failure is on the way out (node → gateway → uplink → internet gateway → targets) for every probe node, a one-line verdict, every monitor as a honeycomb (or as a vote matrix with several nodes), what needs attention and the alerts held back this week. The sidebar folds to icons. Clicking a monitor opens it in a side drawer (how the round was decided, its path, what happened, its record); the monitor list has a honeycomb view; phones get the path as a list; vote verdicts are translated.
- A check path on each monitor: this host → gateway → internet exit (8.8.8.8 by default, `localnet.exit-target`) → target, so a broken uplink is told apart from a broken site. The exit is for display only and never holds alerts.
- Registration: the audit log can be viewed with a licence; everything else stays free.
- Script checks (Nagios plugin compatible), Uptime Kuma database import, master handover with a warm standby, daily status report — all experimental in this release.

## v0.1.0 — 2026-10-23

First public preview. One binary, one machine, everything included.

**Monitoring**
- HTTP(S) checks with status, keyword, JSON path and certificate-expiry assertions; TCP with TLS and send/expect; ICMP ping (unprivileged when the system allows it); UDP checks that speak the real protocol of DNS, NTP, SNMP v1/v2c (and v3 discovery), TFTP, ONC RPC, time and syslog by keyword, plus raw send/expect.
- Every check reports its depth (L1 sent, L2 port answers, L3 service answered correctly) and a specific reason when it fails.
- Push monitors compatible with Uptime Kuma push URLs.
- Soft/hard states with retries; incidents with acknowledgement, silences, reopen windows and notification limits.
- Detection of problems on the monitoring host itself (gateway, uplink, DNS, sleep or stalls): affected checks are held instead of alerting on every target, and a summary is sent afterwards.

**Probe nodes (optional)**
- Run `unagikuma agent` on other machines to check the same targets from several places; a single machine needs none of it.
- Each round is decided by a quorum of the nodes, so one site's broken link does not page you; a failure seen from only some nodes is shown as partial and, if it lasts, becomes one low-priority regional alert.
- Agents connect out to the master over TLS on the same port, pin its certificate and use a per-node token that can be rotated or revoked. They receive only the checks they run; secrets stay in memory.
- Nodes that drop out are reported, and judgement continues with the remaining ones.

**Notifications**
- Feishu/Lark, DingTalk, WeCom, Telegram, Slack, Discord, email (SMTP) and webhooks, with presets for Bark, Gotify, ntfy, PagerDuty, Opsgenie, Pushover, Matrix, Mattermost, Microsoft Teams, Google Chat, PushPlus and ServerChan.
- Policies with fallback or broadcast chains, repeats, delays and limits; deliveries survive restarts and are logged with their result.
- External heartbeat URL so you also hear about it when the monitoring host goes down.

**Configuration and UI**
- `monitors.bind`: one readable text file, editable in the browser or by hand, validated before it is applied, with history and one-click restore.
- Web UI with overview, problems, monitors with heartbeat bars and live updates, latency charts, configuration editor, channels and policies, tunables, API tokens and encrypted secrets.
- Public status pages and SVG badges.
- 15 languages for the UI and notifications, with right-to-left layouts for Arabic and Urdu.

**Platforms**
- Static binaries for Linux (amd64, arm64, armv7, 386, riscv64), macOS (amd64, arm64), Windows (amd64, arm64) and FreeBSD (amd64).
