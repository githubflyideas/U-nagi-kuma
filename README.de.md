# Unagikuma

<p align="center"><img src="docs/images/IMG_8128.jpeg" alt="Unagikuma" width="240"></p>

**Noch ein Werkzeug zur Verfügbarkeitsprüfung: u(ptime) + nagi(os) + kuma(Uptime Kuma).**

**Das Beste aus beiden Welten:** eine Datei auf einem Rechner läuft sofort; willst du mehr, nimm weitere Rechner dazu, und es wird ein Cluster. Dasselbe Programm, nichts zu migrieren.

Einfaches, selbst gehostetes Monitoring: Sie wissen sofort, wenn Ihre Websites oder Server ausfallen – ohne Alarmflut.

[English](README.md) · [中文](README.zh.md) · [हिन्दी](README.hi.md) · [Español](README.es.md) · [العربية](README.ar.md) · [Français](README.fr.md) · [বাংলা](README.bn.md) · [Português](README.pt.md) · [Русский](README.ru.md) · [Bahasa Indonesia](README.id.md) · [اردو](README.ur.md) · **Deutsch** · [日本語](README.ja.md) · [Tiếng Việt](README.vi.md) · [한국어](README.ko.md)

![Die Monitorliste im Ton Papier](docs/images/monitors-paper.png)

## Warum Unagikuma

- **In etwa 30 Sekunden startklar.** Eine Datei herunterladen, starten, Browser öffnen.
- **Eine Datei, sonst nichts zu installieren.** Läuft unter Linux, macOS, Windows und FreeBSD.
- **Prüft, was zählt.** Websites, Server, Ports, Ping, DNS, SSL-Zertifikate und den Ablauf von Domains.
- **Alarme dort, wo Sie ohnehin sind.** E-Mail, Telegram, Slack, Discord, Feishu, DingTalk, WeCom, Webhooks und mehr.
- **Statusseiten**, damit Ihre Nutzer immer Bescheid wissen.
- **Einfacher Umstieg.** Übernehmen Sie Ihre Monitore aus Uptime Kuma oder Nagios.
- **Spricht Ihre Sprache.** Die Oberfläche gibt es in 15 Sprachen.

## Schnellstart

**1.** Laden Sie die Datei für Ihr System unter [Releases](https://github.com/githubflyideas/unagikuma/releases/latest) herunter und entpacken Sie sie.

**2.** Starten:

```sh
./unagikuma
```

Unter Windows starten Sie `unagikuma.exe`.

**3.** Öffnen Sie http://localhost:8077 im Browser und legen Sie Ihr Administratorkonto an.

**4.** Klicken Sie auf **Monitor hinzufügen**, geben Sie die Adresse einer Website ein – fertig.

> Sie öffnen es von einem anderen Rechner aus? Rufen Sie `http://<Server-Adresse>:8077` auf und geben Sie das **Einrichtungstoken** ein, das das Programm beim ersten Start angezeigt hat.

## Ein Rechner, zwei Rechner, ein Cluster

- **Ein Rechner:** Datei starten, Monitor anlegen, fertig. Keine Datenbank, keine Agenten.
- **Zwei Rechner:** einen Beitrittscode unter **Einstellungen › Knoten** einfügen. Jeder Rechner prüft und alarmiert selbst; fällt einer aus, gehen die Alarme trotzdem raus.
- **Ein Cluster:** beliebig viele Standorte. Jeder Alarm zeigt, was jeder Standort sieht, damit du „überall down" von „nur von hier down" unterscheidest.

![Ein ausgefallener Monitor mit dem Ergebnis jedes Standorts](docs/images/detail-sites.png)

## Keine Alarmfluten

- **Eigenes Netz weg?** Eine Nachricht über das Netz, nicht eine pro Monitor.
- **Host ausgefallen?** Seine Dienste werden nicht einzeln gemeldet.
- **Höchstens drei Alarme pro Minute und Kanal;** der Rest kommt als eine Zusammenfassung. Entwarnungen gehen immer sofort raus.

## Umstieg von Uptime Kuma

Uptime Kuma beenden und zuerst ansehen (es wird nichts geändert):

```sh
./unagikuma import kuma --dry-run /path/to/uptime-kuma/data/kuma.db
```

Wenn alles passt, importieren:

```sh
./unagikuma import kuma /path/to/uptime-kuma/data/kuma.db
```

Der Bericht zeigt, was importiert wurde, was man prüfen sollte und was nicht importiert werden konnte, jeweils mit dem nächsten Schritt. Er wird auch in `data/import-reports/` gespeichert. Ein erneuter Aufruf fügt nur Neues hinzu.

## Umstieg von Nagios

Geben Sie `nagios.cfg` an und sehen Sie zuerst nach (es wird nichts geändert):

```sh
./unagikuma import nagios --dry-run /etc/nagios4/nagios.cfg
```

Wenn alles passt, importieren:

```sh
./unagikuma import nagios /etc/nagios4/nagios.cfg
```

Der Bericht zeigt, was importiert wurde, was man prüfen sollte und was nicht importiert werden konnte, jeweils mit dem nächsten Schritt. Er wird auch in `data/import-reports/` gespeichert. Ein erneuter Aufruf fügt nur Neues hinzu.

## Passwort vergessen?

Auf dem Rechner, auf dem Unagikuma läuft:

```sh
./unagikuma reset-password admin
```

Geben Sie das neue Passwort zweimal ein (es wird nicht angezeigt). Mit `--generate` bekommen Sie stattdessen ein zufälliges.

## Mehr erfahren

- [Benutzerhandbuch (auf Englisch)](docs/manual.md) — alles Weitere, Schritt für Schritt
- [Änderungsprotokoll](CHANGELOG.md)

## Lizenz

Apache License 2.0, siehe [LICENSE](LICENSE).
