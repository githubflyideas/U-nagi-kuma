# Unagikuma

<p align="center"><img src="docs/images/logo.png" alt="Unagikuma" width="240"></p>

**Noch ein Werkzeug zur Verfügbarkeitsprüfung: u(ptime) + nagi(os) + kuma(Uptime Kuma).**

Einfaches, selbst gehostetes Monitoring: Sie wissen sofort, wenn Ihre Websites oder Server ausfallen – ohne Alarmflut.

[English](README.md) · [中文](README.zh.md) · [हिन्दी](README.hi.md) · [Español](README.es.md) · [العربية](README.ar.md) · [Français](README.fr.md) · [বাংলা](README.bn.md) · [Português](README.pt.md) · [Русский](README.ru.md) · [Bahasa Indonesia](README.id.md) · [اردو](README.ur.md) · **Deutsch** · [日本語](README.ja.md) · [Tiếng Việt](README.vi.md) · [한국어](README.ko.md)

![Ein kurzer Rundgang durch die Weboberfläche von Unagikuma](docs/images/tour.gif)

## Warum Unagikuma

- **In etwa 30 Sekunden startklar.** Eine Datei herunterladen, starten, Browser öffnen.
- **Eine Datei, sonst nichts zu installieren.** Läuft unter Linux, macOS, Windows und FreeBSD.
- **Prüft, was zählt.** Websites, Server, Ports, Ping, DNS, SSL-Zertifikate und den Ablauf von Domains.
- **Alarme dort, wo Sie ohnehin sind.** E-Mail, Telegram, Slack, Discord, Feishu, DingTalk, WeCom, Webhooks und mehr.
- **Ein Problem, keine Alarmflut.** Fällt Ihr eigenes Netzwerk aus, merkt Unagikuma das und schickt nicht für jeden einzelnen Monitor einen Alarm.
- **Statusseiten**, damit Ihre Nutzer immer Bescheid wissen.
- **Wächst mit.** Bei Bedarf von mehreren Standorten aus prüfen (Vorschau).
- **Einfacher Umstieg.** Übernehmen Sie Ihre Monitore aus Uptime Kuma oder Nagios (Vorschau).
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

## Lieber mit Docker?

```sh
docker run -d --name unagikuma -p 8077:8077 -v unagikuma-data:/data ghcr.io/githubflyideas/unagikuma:latest
```

Öffnen Sie dann http://localhost:8077. Das **Einrichtungstoken** finden Sie mit `docker logs unagikuma`.

## Mehr erfahren

- [Benutzerhandbuch (auf Englisch)](docs/manual.md) — alles Weitere, Schritt für Schritt
- [Änderungsprotokoll](CHANGELOG.md)

## Lizenz

Apache License 2.0, siehe [LICENSE](LICENSE).
