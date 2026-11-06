# Unagikuma

<p align="center"><img src="docs/images/IMG_8128.jpeg" alt="Unagikuma" width="240"></p>

**Yet another uptime check tool: u(ptime) + nagi(os) + kuma(Uptime Kuma).**

Friendly, self-hosted monitoring: know the moment your websites and servers go down, without the noise.

**English** · [中文](README.zh.md) · [हिन्दी](README.hi.md) · [Español](README.es.md) · [العربية](README.ar.md) · [Français](README.fr.md) · [বাংলা](README.bn.md) · [Português](README.pt.md) · [Русский](README.ru.md) · [Bahasa Indonesia](README.id.md) · [اردو](README.ur.md) · [Deutsch](README.de.md) · [日本語](README.ja.md) · [Tiếng Việt](README.vi.md) · [한국어](README.ko.md)

![The monitor list in the Paper tone](docs/images/monitors-paper.png)

## Why Unagikuma

- **One machine is enough.** Download one file, run it, open your browser: monitoring and alerts work in about 30 seconds. A second machine or a cluster is something you add later, not something you need to start.
- **One file, nothing else to install.** Works on Linux, macOS, Windows and FreeBSD.
- **Checks what matters.** Websites, servers, ports, ping, DNS, SSL certificates and domain expiry.
- **Alerts where you already are.** Email, Telegram, Slack, Discord, Feishu, DingTalk, WeCom, webhooks and more.
- **One problem, not a flood of alerts.** If your own network goes down, Unagikuma notices and doesn't page you for every single monitor.
- **Status pages** to keep your users in the loop.
- **Grows with you.** Add a second machine or a whole cluster in a minute, with a join code from the web page.
- **Five tones from white to black.** Paper, Mist, Slate, Graphite and Ink, with Nagios-style status colours that read at a glance.
- **Easy to switch.** Bring your monitors over from Uptime Kuma or Nagios.
- **Speaks your language.** The interface comes in 15 languages.

## Quick start

**1.** Download the file for your system from [Releases](https://github.com/githubflyideas/unagikuma/releases/latest) and unpack it.

**2.** Run it:

```sh
./unagikuma
```

On Windows, run `unagikuma.exe`.

**3.** Open http://localhost:8077 in your browser and create your admin account.

**4.** Click **Add monitor**, enter a website address, and you're done.

> Opening it from another computer? Use `http://<server-address>:8077` and enter the **Setup token** that the program printed when it first started.

## One machine, two machines, a cluster

> **Have your cake and eat it too.** Most tools make you choose: a small one that is easy to start but stays on one box, or a big one that scales but needs a database, agents and an afternoon of setup. Unagikuma is both. Start with one file on one machine; when you want more, join more machines to it. Same program, same page, nothing to migrate.

Unagikuma is built for one machine first. Everything works there: checks, alerts, status pages, history. More machines only add more points of view.

### One machine: ready at once

Run the file and add a monitor. There is nothing else to set up: no database, no agents, no second server. This is how most people use it, and it is enough on its own.

![The overview: where things break, from this machine to each target](docs/images/overview.png)

### Two machines: each one watches on its own

Put a second server somewhere else, for example in another office or with another provider. On the first server open **Settings › Nodes › Join code**; on the second, **Settings › Nodes › Join cluster** and paste the code. The second server gets a copy of the configuration straight away and starts checking at once.

- **Both servers check and alert independently.** If one of them goes down, the other keeps working and keeps sending alerts.
- **No guessing.** When two servers lose sight of each other, neither one can tell which side is cut off, so neither declares the other dead. Both keep checking and alerting on their own.
- **Every message names its site** (`[Tokyo] …`), so you always know where an alert came from.

### A cluster: every site, one picture

Add as many sites as you like the same way. Each site still checks and alerts on its own, so losing any one of them, the main server included, never stops monitoring. What the cluster adds is context: every monitor and every alert shows what the other sites see right now.

![A failing monitor with each site's result](docs/images/detail-sites.png)

- **"Down everywhere" or "down from here"?** The observations from every site tell you at a glance whether a service is really down or only one network path is broken. Results that are out of date are marked as such.
- **A site that goes quiet is reported once,** after one minute (adjustable), with a recovery notice when it is back. A main server that restarts does not set off a wave of "site unreachable" messages.
- **Changes reach every site within seconds,** and each site can keep monitors of its own, such as the router in that office.

![The nodes page: every site, its round trip and its configuration](docs/images/nodes.png)

## Alert floods: one problem, one message

A monitoring tool that pages you a hundred times for one broken cable gets muted, and then it is useless. Unagikuma treats a flood of alerts as a bug to prevent:

- **Your own network first.** When this machine loses its own internet connection, Unagikuma sees that its targets all failed together and sends one message about the network instead of one per monitor. When the network is back, the hidden monitors are checked again gradually before anything is reported.
- **Parents before children.** When a host is down, the services on it are not reported one by one.
- **At most three alerts a minute per channel.** Anything beyond that is collected into one summary at the end of the minute, worst first. New problems have their own allowance, so reminders never delay them, but a hundred services failing together still arrive as one summary, not a hundred messages.
- **Good news is never held back.** Recoveries and escalations always go out at once, so you never wait for the message that tells you to stop.
- **Flapping is summarised.** A service that goes up and down is reported as one incident with a count, not as a stream of problems and recoveries.
- **Planned work stays quiet.** Maintenance windows silence what you said you would be working on.

Every limit can be adjusted under **Settings › Tunables**.

## Moving from Uptime Kuma

Stop Uptime Kuma, then look first (nothing is changed):

```sh
./unagikuma import kuma --dry-run /path/to/uptime-kuma/data/kuma.db
```

If it looks right, import:

```sh
./unagikuma import kuma /path/to/uptime-kuma/data/kuma.db
```

The report lists what was imported, what needs a look and what could not be imported, with what to do. It is also saved in `data/import-reports/`. Running it again only adds what is new.

## Moving from Nagios

Point it at `nagios.cfg` and look first (nothing is changed):

```sh
./unagikuma import nagios --dry-run /etc/nagios4/nagios.cfg
```

If it looks right, import:

```sh
./unagikuma import nagios /etc/nagios4/nagios.cfg
```

The report lists what was imported, what needs a look and what could not be imported, with what to do. It is also saved in `data/import-reports/`. Running it again only adds what is new.

## Forgot your password?

On the machine where Unagikuma runs:

```sh
./unagikuma reset-password admin
```

Type the new password twice (it is not shown). Add `--generate` to get a random one instead.

## Learn more

- [User manual](docs/manual.md) — everything else, step by step
- [Changelog](CHANGELOG.md)

## License

Apache License 2.0, see [LICENSE](LICENSE).
