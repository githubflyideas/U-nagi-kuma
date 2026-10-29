# Unagikuma user manual

How to install, configure and run Unagikuma. The [README](../README.md) has the short version.
**Contents**

- [Quick start](#quick-start)
- [monitors.bind](#monitorsbind)
- [Alerting](#alerting)
- [Status pages](#status-pages)
- [Probe nodes](#probe-nodes)
- [Users](#users)
- [Registration and quotas](#registration-and-quotas)
- [Moving from Uptime Kuma](#moving-from-uptime-kuma)
- [Command line](#command-line)
- [Data and backups](#data-and-backups)
- [API](#api)
- [Troubleshooting](#troubleshooting)

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

## monitors.bind

```text
; serial  interval  retry  timeout  attempts
@ IN SOA unagikuma. admin.example.com. ( 2026102301 1m 15s 5s 3 )

$SET notify.repeat=30m

; notification channels and the policy that uses them
ops      IN CHANNEL feishu url=${secret:ops_webhook}
mail     IN CHANNEL email host=smtp.example.com port=587 tls=starttls username=alerts@example.com password=${secret:smtp} from=alerts@example.com to=oncall@example.com
default  IN POLICY chain=ops,mail mode=fallback max=3 repeat=30m

; monitors: name [interval] IN TYPE target key=value...
www      IN HTTP https://www.example.com/ title="Website" keyword="Welcome" cert-days=14 group=public
api  30s IN HTTP https://api.example.com/health json-path=$.status json-expect=ok
db       IN TCP  10.0.0.5:5432 title="Postgres"
gw       IN PING 192.168.1.1
time     IN UDP  10.0.0.1:ntp authorized=yes
switch   IN UDP  10.0.0.2:snmp community=${secret:snmp_ro}
backup   IN PUSH period=24h grace=2h
company  IN DOMAIN example.com days=45 critical-days=10

; a public status page at /status/main
main     IN STATUSPAGE "Service status" select="group:public"
```

- After editing the file by hand, **increase the serial**; Unagikuma then validates and applies it. Saving a monitor, channel or policy in the web UI raises the serial for you. **Configuration › monitors.bind** shows the file read-only, with a copy button; edit it on the server. A file with errors is never applied: the last good version keeps running and the UI shows what is wrong.
- Credentials never need to be in the file: store them under **Settings › Secrets** and refer to them as `${secret:name}`. Values from the environment can be used as `${env:NAME}` when the server is started with `--allow-env NAME`.
- Monitors are grouped into hosts by the address they check; `device=core-sw1` groups monitors of several addresses under one name. A host with a PING monitor is UP or DOWN by that ping.
- `parent=gw` (or `parent=sw1,gw`) declares a dependency: while a parent is down, a failing child is shown as UNREACHABLE and not notified. If the child is still down once its parents are back, it is notified as usual.
- Maintenance windows hold notifications for the monitors they select; that time does not count against uptime, and a monitor still down when the window closes is notified then:

  ```text
  sunday  IN MAINT "Server room work" select="tag:site:bj" every=sun start=02:00 duration=2h notify=yes
  upgrade IN MAINT "Core switch upgrade" select="cidr:10.0.0.0/24" at="2026-11-01 22:00" duration=3h
  backups IN MAINT "Nightly backup" select="group:db" cron="30 2 * * 1-5" duration=40m
  ```

  `select=` takes fields that only depend on the configuration: `name`, `title`, `type`, `svc`, `port`, `host`, `cidr`, `tag`, `group`, `policy`, `node`, `area`, plus free text; terms are AND-ed, commas mean "any of", `-` negates, `/…/` is a regular expression.
- `DOMAIN` watches when a domain's registration expires, asking the registry over RDAP (or WHOIS where there is no RDAP), every 6 hours by default and at most hourly. Inside `days=` (30) it is degraded, inside `critical-days=` (7) it fails; if no registry answers it is UNKNOWN, never down. **Monitors › Expiry dates** lists every domain and TLS certificate, soonest first.
- `unagikuma keywords` lists the UDP keywords and what each check proves.
- Every day at 10:00 (`notify.daily-report-at`) a status report lists the hosts that are DOWN and UP and every service that is not OK; **Notices & status › Daily report** previews it and sends it on demand.

### Script checks

> Experimental.

`EXEC` runs a check script using the Nagios plugin conventions: exit status 0 is OK, 1 WARNING (shown as DEGRADED), 2 CRITICAL, 3 UNKNOWN; the first output line is the status text and performance data after `|` is kept as metrics. Existing plugins and in-house scripts work unchanged:

```text
disk-db1  IN EXEC check_disk args="-w 20% -c 10% -p /data" device=db1
load-db1  IN EXEC check_load args="-w 5,4,3 -c 10,8,6" device=db1
nrpe-web  IN EXEC "check_nrpe -H $HOSTADDRESS$ -c check_users" address=10.0.0.21
```

- Scripts are looked up only in the check-scripts directory of the node that runs the check: `<data>/check-scripts` on a server, `/etc/unagikuma/check-scripts` (Windows: `%ProgramData%\unagikuma\check-scripts`) on an agent, or `--script-dir dir1,dir2`. The directory is created on first start with a README and an example (`check_example`). The configuration names a file there, never a path, and nothing goes through a shell.
- Each node runs its own copy, so copy scripts to every node that runs the check (scp, Ansible…); Unagikuma does not upload them. New script checks start **paused**: enabling one checks that every node running it has the script, and refuses with the list of nodes that lack it. **Settings › Check scripts** shows each check's script on each node (present, missing, not executable, another version) and the sidebar shows a red count while an enabled script check cannot run somewhere.
- Plugins get a minimal environment (`PATH`, `UNAGIKUMA_MONITOR`, `UNAGIKUMA_ADDRESS`, `UNAGIKUMA_TIMEOUT`), not the server's. `$HOSTADDRESS$`, `$SERVICEDESC$` and `$TIMEOUT$` are replaced in arguments. At the timeout the plugin and everything it started are killed and the check is CRITICAL.
- Only administrators can add EXEC checks or change what they run. `probe.exec-concurrency` limits plugins running at once per node.

## Alerting

Notifications go through **channels** (where a message is sent) and **policies** (which channels, how often, and what happens next). Both are records in `monitors.bind` and have forms under **Alerting**.

```text
ops-im    IN CHANNEL feishu url=${secret:feishu_ops}
oncall    IN CHANNEL aliyun-sms access-key-id=LTAI… access-key-secret=${secret:ali} sign-name=Ops template-code=SMS_123 phones=13800000000
manager   IN CHANNEL email host=smtp.example.com from=alerts@example.com to=boss@example.com
office    IN PERIOD "mon-fri 09:00-18:00; sat 10:00-12:00"
team      IN CHANNEL group members=ops-im,oncall
night-sms IN CHANNEL aliyun-sms … period=!office
default   IN POLICY chain=team,night-sms mode=all repeat=30m max=3 escalate=30m:manager,2h:manager+oncall
```

- **Policies.** `chain=` lists channels; `mode=fallback` (default) tries them in order until one succeeds, `mode=all` sends to all. `repeat=`, `max=`, `delay=`, `reopen-window=`, `ack-expire=`, `recovery=no` and `notify-degraded=yes` shape what is sent. A monitor uses `policy=NAME`, or the `default` policy.
- **Time periods.** `PERIOD` records are weekly rules (`mon-fri 09:00-18:00`, ranges past midnight such as `22:00-07:00`, `24:00` for end of day). `period=office` on a channel uses it only inside the period, `period=!office` only outside it. A skipped channel is not a failure: the next one in a fallback chain is tried.
- **Contact groups.** A `group` channel notifies all its `members=` at once; it succeeds if any member does. Members keep their own time periods.
- **On-call rotations.** `rotate=daily|weekly|Nd|Nh` makes a group a rotation: it notifies only the member whose shift it is, and the others in turn only when that one cannot be reached. `handoff="mon 09:00"` sets when shifts change (a day only for whole weeks), `start=` the date from which the first member is on call. Give each person one channel (their SMS, their chat) and list them in shift order:

  ```text
  oncall IN CHANNEL group members=li-sms,wang-sms,zhao-sms rotate=weekly handoff="mon 09:00" start=2026-11-02
  ```

  The **On call** page shows who is on call now and the next shifts, creates rotations, and takes stand-ins: someone covers for a while, alerts go to them first, and the order of the shifts stays. It also measures MTTA and MTTR over `oncall.stats-window` (30 days), counting each incident for whoever was on call when it opened.
- **Escalation.** `escalate=DELAY:channel[+channel],…` on a policy notifies more channels while an incident stays open and unacknowledged: after 30 minutes the manager, after two hours also the on-call phone. Acknowledging stops it.
- **Summaries say what kind of problem it is.** When many notices queue up at once (`notify.coalesce-queue`, `notify.coalesce-wait`) they are replaced by a summary, and the summary tells services from the network:
  - services that fail on their own come first, as "please act now", for their owners;
  - `notify.group-min` (3) or more monitors of one IP segment failing at the network level together are reported as a fault of that segment, its switch or uplink; monitors failing to resolve their names together, as a DNS fault — "please contact the network team".

  `network=netadmin` on a policy sends the network part to its own channels (the network team) and leaves the services to the chain.
- **When this host loses its network** it cannot notify anyone, so nothing is sent while it lasts and nothing queued meanwhile is sent afterwards. Failures that began just before the outage was recognised (`localnet.backdate`) and that nobody was told about are taken back. When the network is back every monitor is re-checked, and one message says how long the host was blind and what is still failing; outages shorter than `localnet.notify-after` (5 minutes) are only recorded. A recovery is sent only for problems someone actually received. To be told *while* the host is offline, use an external heartbeat (`--heartbeat-url`) or a second node.
- **SMS and voice calls.** `aliyun-sms`, `aliyun-voice`, `tencent-sms`, `tencent-voice`, `twilio-sms` and `twilio-voice` channels. Template parameters default to the monitor name and the status; `params="{{.Monitor}}|{{.StatusText}}"` changes them. Keys are stored as secrets when saved from the UI.
- **Event handlers.** `handler="restart-web $SERVICESTATE$ $SERVICESTATETYPE$"` on a monitor runs a script from the check-scripts directory on each state change, Nagios style, with `UNAGIKUMA_MONITOR`, `UNAGIKUMA_STATE`, `UNAGIKUMA_STATE_TYPE`, `UNAGIKUMA_ATTEMPT`, `UNAGIKUMA_ADDRESS` and `UNAGIKUMA_REASON` in its environment. It runs on the master, one at a time per monitor, for at most `engine.handler-timeout`; the result is in the event log. Only administrators may set it.
- **Contacts.** **Alerting › Contacts** lists everyone who may receive alerts, with their e-mail addresses and phone numbers. Channels are checked against it:
  - an address of someone marked as **left** is no longer sent to, and their linked login and API tokens stop working;
  - an address refused as nonexistent (unknown mailbox, invalid number) `contacts.bounce-threshold` times (3) is no longer sent to; one bad recipient never stops a message to the others;
  - an address that is not in the directory is still sent to, but the channel, the policies using it and the **Alerting** menu get a red dot saying what to check.

  Write `to=@li` or `phones=@li,@oncall` to use a person's addresses instead of typing them. Saving a channel with new addresses offers to add them to the directory; **Import them** adds every address your channels already use. `unagikuma check` reports the same problems when it can read the data directory.
- **Why was I (not) notified?** The monitor drawer's **Notifications** section says it in words: maintenance, a parent is down, this host's own outage, only some nodes failing, still SOFT, acknowledged, silenced, the notification limit, or a policy without channels. **Alerting › Policies › Predict** simulates an outage against a policy.

### Announced changes

**Notices & status › Changes** announces planned work — a switch upgrade, a database migration — to the people it concerns:

- The announcement goes out at once through a policy (or straight to chosen channels), with reminders before the start (`changes.reminders`, `24h,1h`; only those that come after the announcement), a notice when it starts and one if it runs past its end without being closed.
- The monitors matched by its filter (`group:core`, `tag:site:bj`, …) are in maintenance during the window: failures are not notified and not counted against uptime. No `MAINT` record is needed.
- It can put a maintenance notice on a status page, which is closed with the outcome.
- It ends with a receipt: **Done** or **Rolled back** with a note, or **Cancelled** before it starts. Moving the window tells everyone the new time and starts the reminders again.
- The history can be searched and exported as CSV. The notices are templates you can change; the built-in ones are in English and Chinese.

### How things are counted

- **Unhandled problems** are open incidents of any kind — including regional and low-priority ones — that nobody has acknowledged or silenced. The sidebar, the overview, the problems page and `/metrics` (`unagikuma_unhandled_problems`) all use this one definition. A monitor that cannot be judged (UNKNOWN) has no incident; it is counted under its status.
- **Uptime** is up time ÷ (up + down time). DEGRADED (slow) counts as up, and its share is reported separately. Maintenance windows and gaps without checks count as neither. Time spent confirming a failure (SOFT) is held back: it counts as down if the failure is confirmed and as up if the check recovers first, so a monitor that fails from the start shows 0%.
- **Coverage.** A window's uptime is shown only once the monitor was watched for at least `uptime.min-coverage` (10%) of it; until then the UI shows “—” and how much was watched, so a monitor added an hour ago does not claim a 30-day figure.
- **Changing a check** that is failing — a timeout, an expected keyword, even the target — keeps its open incident and checks again right away. If it still fails, nobody is paged a second time; if it passes, the usual recovery is sent. A check with no open problem starts over as PENDING.
- **Acknowledgements** are kept as a history: acknowledging again adds an entry with its note instead of replacing the first one, and the time of the first acknowledgement is kept.

## Status pages

```text
public IN STATUSPAGE "Example Inc. status" select="group:public" subscribe=mailer domain=status.example.com
```

- A page shows the selected monitors with their uptime over `windows=` (24h, 7d, 30d, 90d), at `/status/NAME`. Targets and reasons are never shown.
- **Announcements** are written under **Notices & status › Status pages**: a title, a message and a severity (notice, minor, major, maintenance); open ones are shown on top, resolved ones under *Past notices* for `statuspage.resolved-shown` (7 days). Maintenance windows of the page's monitors within the next week are listed too.
- **Subscriptions.** With `subscribe=CHANNEL` (an e-mail channel) visitors can subscribe by e-mail. They confirm with a link first, and every message has an unsubscribe link. When you publish or update an announcement, tick *E-mail subscribers*. Requests are limited per address (`statuspage.subscribe-rate`) and unconfirmed subscriptions are capped (`statuspage.max-pending`). Every page also has an Atom feed at `/status/NAME/feed.atom`.
- **Own host name.** With `domain=status.example.com`, requests for that name get the page at `/` (and `/feed.atom`); the console and its API are not reachable under it. Point the name at the server and put a TLS proxy in front, or give the server a certificate for it.
- Badges: `/badge/MONITOR/status.svg`, `uptime.svg` and `latency.svg` for monitors on a public page.

## Probe nodes

> Experimental: the agent protocol and the handover may still change between v0.x releases.

A single machine is a complete setup. To check the same targets from more places, declare nodes and run agents there:

```sh
unagikuma node add bj --title "Beijing office" --server https://master.example.com:8077
```

This adds `bj IN NODE "Beijing office"` to `monitors.bind`, issues a token for `bj` and prints the commands to start the agent (foreground, systemd or PowerShell). The web UI does the same under **Settings › Nodes**. On the node:

```sh
unagikuma agent --server https://master.example.com:8077 --node bj \
  --token-file /etc/unagikuma/agent.token --ca-fingerprint sha256:…
```

- Agents connect out to the master's port (8077 by default), so they can sit behind NAT. The connection is TLS; the agent pins the master's own CA by the fingerprint in the command. Put the master behind a TCP (not HTTP) proxy if you need one in between.
- By default every monitor is checked from every node. Targets that only exist at one site go in a section:

  ```text
  bj  IN NODE "Beijing office" region=north
  $SCOPE region:north policy=bj-oncall   ; checked by north nodes only, paged to bj-oncall
  bj-core IN PING 10.10.0.1
  $SCOPE *                               ; back to every node
  ```

  Other nodes never receive these monitors. `nodes=bj,sh` (or `nodes=region:north`) on a single monitor does the same and wins over the section. A round is decided by `quorum=` (majority by default); nodes that are offline lower the quorum instead of blocking the verdict.
- A failure seen from some nodes only is shown as partial; if it lasts `partial-escalate` (30 minutes) it becomes one low-priority regional alert. A node that stays disconnected is reported under its `policy=`.
- Or join from the web, without copying commands: on the master, **Settings › Nodes › Join code** gives a one-time code (valid for `cluster.join-code-ttl`, 1 hour); on the other server, **Settings › Nodes › Join cluster** takes it. That server then runs as a node of the master. Its own monitors are deleted, or, if you choose, imported to the master paused in a section named after it, to review and resume there.
- `--emergency-webhook URL` (or `feishu:URL`, `dingtalk:URL`, ...) on an agent sends one message when the master stays unreachable for `cluster.emergency-after` (3 minutes) and one when it is back: the master cannot report its own outage.
- `unagikuma node list`, `node token NAME` (new token, the old one stops working) and `node revoke NAME` manage tokens; a revoked agent is disconnected within one ping interval.

### Moving the master role

There is no election: which server is the master is your decision, and
moving it is one click.

1. Join the server that should be able to take over as a node, and give it
   a **standby address** under Nodes › Edit (`bj IN NODE standby=https://10.0.0.2:8077`).
   It now keeps a copy of the master's state — databases, `monitors.bind`,
   the secret key and the cluster CA — refreshed every
   `cluster.standby-sync` (5 minutes) and after each configuration change.
   Every node learns the standby's address.
2. **Planned** (maintenance, new hardware): Nodes › **Make master** on the
   standby. It takes the current state and restarts as master; the old
   master stops checking and refuses changes; nodes move over by
   themselves. Users, tokens, secrets and the CA come along, so nothing has
   to be joined again.
3. **Master lost**: on the standby, Settings › Nodes › **Take over as
   master** (or stop it and run `unagikuma promote`). Nodes move to it once
   the old master is unreachable.

Every handover raises a **term**. Nodes follow the highest term they have
seen, and a replaced master that comes back is told by the new one (with a
notice signed by the cluster CA) or by the first node that reaches it,
and stops checking — two masters never alert side by side. Taking over
after a loss is always done by a person: without a quorum, an automatic
takeover would split the cluster whenever the network splits.

### As a Windows service

In an Administrator PowerShell, in the folder of `unagikuma.exe`:

```powershell
.\unagikuma.exe service install -- server --data C:\ProgramData\unagikuma --log-file C:\ProgramData\unagikuma\unagikuma.log
.\unagikuma.exe service uninstall
```

The service starts automatically and is restarted if it stops. Agents install the same way (the node's install command shows it).

## Users

**Settings › Users** manages accounts: administrators do everything; operators acknowledge problems and edit monitors and maintenance windows; viewers only read. An operator can be limited to some `$SCOPE` sections (`region:north`, `bj`): they then change only what is written in those sections and not the shared configuration (channels, policies, the raw file).

**Settings › Audit** lists every change to accounts, roles and sections (before and after), tokens, secrets (names only), nodes and the configuration, with the person or token, the section, and the source address; configuration edits made in the file on disk are listed too, and so are failed logins. `GET /api/v1/audit?actor=&action=&area=` serves the same to administrators; entries are kept for `retention.audit` (two years).

Reading the audit log needs a registered installation; see [Registration and quotas](#registration-and-quotas).

## Registration and quotas

Without a licence everything works except reading the audit log, and nothing is ever limited. Install the `license.json` you received under **Settings › Registration**; a trial licence is issued for the installation number shown there.

A licence may limit the number of nodes (the master included) and of accounts. Within the limit nothing changes; beyond it, adding a node, a joining machine, a new account or an imported account is refused with a message saying so. Nothing already in use is switched off, for example after a smaller licence replaces a larger one; the registration page shows the usage against each limit.

## Moving from Uptime Kuma or Nagios

The import adds what Unagikuma can run and lists everything else with the reason and what to do instead. It never changes or deletes monitors that are already here, so running it twice is safe. Always look first with `--dry-run`.

**From Uptime Kuma** (1.23 and 2.x). Stop Uptime Kuma, then:

```sh
unagikuma import kuma --dry-run /path/to/uptime-kuma/data/kuma.db
unagikuma import kuma /path/to/uptime-kuma/data/kuma.db
```

A JSON backup from Uptime Kuma 1.x works too, but has no accounts, maintenance windows or status pages. What comes over: HTTP, keyword, JSON query, TCP port, ping, DNS, push (the push URLs keep working: only the server address changes), Docker, gRPC, Redis, PostgreSQL and MySQL monitors with their intervals, retries, groups (nested groups become `Parent / Child`), tags and paused state; notifications (Feishu, DingTalk, WeCom, Telegram, Slack, Discord, webhook, SMTP, ntfy, Gotify, Pushover, Teams, PagerDuty, Matrix, Mattermost, Google Chat, Bark, ServerChan, PushPlus, Opsgenie) as channels with their credentials stored as secrets; weekly, monthly, cron and one-time maintenance windows; status pages; and accounts with their passwords (on a new installation the first one becomes the administrator). Monitors Unagikuma cannot run (MQTT, MongoDB, SQL Server, RADIUS, game servers, upside-down mode, …) are listed with a suggested replacement.

**From Nagios** (and Icinga 1, Naemon). Point it at `nagios.cfg`, so the files it names (`cfg_file=`, `cfg_dir=`, `resource_file=`) are read the way Nagios reads them:

```sh
unagikuma import nagios --dry-run /etc/nagios4/nagios.cfg
unagikuma import nagios /etc/nagios4/nagios.cfg
```

A folder or single `.cfg` files work too. Templates are resolved; hosts become ping checks that their services depend on (`parent=`); `check_http`, `check_tcp`, `check_ssh`, `check_dns`, `check_dig`, `check_ntp`, `check_snmp` (uptime), `check_mysql`, `check_pgsql` and similar plugins become built-in checks; other plugins become script checks that start paused until you copy the plugin into the check-scripts directory. `$USERn$` values from `resource.cfg` become secrets, passive-only services become push monitors, contacts with an e-mail address become e-mail channels, contact groups become group channels, and time periods become `PERIOD` records.

**The report.** Both print a summary (imported / needs a look / not imported), a table of what was imported, and for each item that needs a look or was not imported the reason and what to do. The same report is saved as text and Markdown in `<data>/import-reports/`. The new records go in one block at the end of `monitors.bind`, in sections headed by where they came from; `title=` keeps the original names. The web UI does the same under **Configuration › Import**. Options: `--area SECTION` puts the monitors in a section, `--role` sets the role of imported accounts, `--report DIR` saves the report elsewhere. Exit status: 0 done (or nothing new), 1 failed (nothing changed), 2 wrong usage.

## Forgot your password?

On the machine where Unagikuma runs (it may keep running):

```sh
unagikuma reset-password --data /path/to/data admin
```

Type the new password twice (it is not shown), or use `--generate` to get a random one printed once. Everyone signed in as that user is signed out. If the user name is wrong, the command lists the existing users.

## Command line

```text
unagikuma [server] [flags]       run the monitor and its web UI (default)
unagikuma agent [flags]          run a probe node for a master
unagikuma node add|list|token|revoke
                                manage probe nodes and their tokens
unagikuma check [file]           validate a monitors.bind file
unagikuma serial [bump] [file]   print or increase the serial
unagikuma probe <target> [k=v]   run one check now, e.g. "udp 10.0.0.1:ntp"
unagikuma keywords [dialect]     list the UDP keywords
unagikuma backup [--passphrase-file F] [--history]
                                write a backup archive
unagikuma restore ARCHIVE        restore a backup
unagikuma reset-password [user]  set a new password (--generate makes one)
unagikuma version
```

Main server flags: `--data DIR` (or `UNAGIKUMA_DATA`), `--listen ADDR` (or `UNAGIKUMA_LISTEN`, default `:8077`), `--tls-cert`/`--tls-key`, `--base-url` for links in notifications, `--heartbeat-url` for an external dead man's switch (for example healthchecks.io), `--trusted-proxy CIDR,...` when running behind a reverse proxy. The port answers plain HTTP and TLS; with `--tls-cert` the UI uses your certificate and plain requests are redirected to https.

## Data and backups

The data directory holds `monitors.bind`, `core.db` (users, incidents, notification log, configuration history, encrypted secrets), `history.db` (check results and uptime) and `unagikuma.key`, the key that encrypts the secrets. `cluster-ca.*` and `cluster.*` are the certificate authority agents pin; keep them with the key so agents keep trusting a restored master. 

A backup is an archive made while the server runs, from **Settings › System** or the command line:

```sh
unagikuma backup --history                         # databases and monitors.bind
unagikuma backup --passphrase-file pass.txt        # encrypted, also the key and the cluster CA
unagikuma restore --passphrase-file pass.txt unagikuma-20261101-0300.nkbackup
```

With a passphrase the archive is encrypted (argon2id, AES-256-GCM) and a restored master keeps its secrets and its agents. Without one, keep `unagikuma.key` (or `UNAGIKUMA_SECRET_KEY`) separately. Restore into a stopped installation; `--force` replaces an existing one.

## API

Everything the UI does goes through a REST API under `/api/v1`. Create a token under **Settings › API tokens** and send it as `Authorization: Bearer nk_…`; tokens are scoped to read, ack, write or admin.

- **Concurrent edits.** Every monitor, channel, policy, node and maintenance window the API returns carries an `etag`. Send it back as `If-Match` when changing or deleting the record: if someone changed that record in the meantime the request fails with `412` and code `record_changed`, and nothing is overwritten. Changes to other records never conflict. Without `If-Match` the last write wins.
- **Safe retries.** A `POST` with an `Idempotency-Key` header (any unique string, at most 255 characters) is applied once; repeating it within `api.idempotency-ttl` (24 hours) returns the first answer with `Idempotent-Replayed: true`. Failed requests are not remembered, so they can be retried with the same key.
- **Pages.** `GET /api/v1/monitors?limit=100` returns monitors in id order with a `next_cursor`; pass it as `cursor=` for the next page until it is `null`. Without `limit` the whole list comes back sorted for display.
- **Stable ids.** Every monitor has an `id` that survives renames; `/api/v1/monitors/id:42` addresses it wherever a name is accepted.
- The file format itself is specified in [monitors.bind syntax](spec/monitors-bind.md).

## Troubleshooting

- **Everything turned red at once.** Look at *Where it breaks* on the overview: if this host's gateway or uplink fails, the problem is on this side, and alerts for the targets are held until it is back. The Internet gateway under the internet targets (`localnet.exit-target`, 8.8.8.8) is pinged only once an internet target fails, to tell a broken site from a broken way out; while they all answer it shows as not checked. Change its address under **Settings › General** (empty turns it off).
- **A monitor is down but nobody was told.** Open it and read *Notifications* in the drawer; the event log shows each delivery with its error, and **Alerting › Channels › Test** sends a test message.
- **Ping checks say "privilege".** Allow unprivileged ping (`net.ipv4.ping_group_range`) or give the binary `CAP_NET_RAW`; see Quick start.
- **A script check stays paused.** Enabling it lists the nodes that lack the script; **Settings › Check scripts** shows each node's copy.
- **An edit to monitors.bind is ignored.** Increase the serial; a file with errors is never applied, and the UI shows the error with its line.
- **The standby has no copy.** **Settings › Nodes** shows when it last fetched the master's state and the last error; it retries after `cluster.standby-retry` and on every configuration change.
