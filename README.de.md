# Unagikuma

<p align="center"><img src="docs/images/IMG_8128.jpeg" alt="Unagikuma" width="240"></p>

**Noch ein Werkzeug zur Verfügbarkeitsprüfung: u(ptime) + nagi(os) + kuma(Uptime Kuma).**

Einfaches, selbst gehostetes Monitoring: Sie wissen sofort, wenn Ihre Websites oder Server ausfallen – ohne Alarmflut.

[English](README.md) · [中文](README.zh.md) · [हिन्दी](README.hi.md) · [Español](README.es.md) · [العربية](README.ar.md) · [Français](README.fr.md) · [বাংলা](README.bn.md) · [Português](README.pt.md) · [Русский](README.ru.md) · [Bahasa Indonesia](README.id.md) · [اردو](README.ur.md) · **Deutsch** · [日本語](README.ja.md) · [Tiếng Việt](README.vi.md) · [한국어](README.ko.md)

![Die Monitorliste im Ton Papier](docs/images/monitors-paper.png)

## Warum Unagikuma

- **In etwa 30 Sekunden startklar.** Eine Datei herunterladen, starten, Browser öffnen.
- **Eine Datei, sonst nichts zu installieren.** Läuft unter Linux, macOS, Windows und FreeBSD.
- **Prüft, was zählt.** Websites, Server, Ports, Ping, DNS, SSL-Zertifikate und den Ablauf von Domains.
- **Alarme dort, wo Sie ohnehin sind.** E-Mail, Telegram, Slack, Discord, Feishu, DingTalk, WeCom, Webhooks und mehr.
- **Ein Problem, keine Alarmflut.** Fällt Ihr eigenes Netzwerk aus, merkt Unagikuma das und schickt nicht für jeden einzelnen Monitor einen Alarm.
- **Statusseiten**, damit Ihre Nutzer immer Bescheid wissen.
- **Wächst mit.** Bei Bedarf in einer Minute einen zweiten Rechner oder einen ganzen Cluster hinzufügen, per Beitrittscode auf der Weboberfläche.
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

> **Das Beste aus beiden Welten.** Die meisten Werkzeuge lassen dich wählen: ein kleines, das sich leicht starten lässt, aber auf einem Rechner bleibt, oder ein großes, das skaliert, aber Datenbank, Agenten und einen Nachmittag Einrichtung braucht. Unagikuma ist beides. Fang mit einer Datei auf einem Rechner an; wenn du mehr willst, nimm weitere Rechner dazu. Dasselbe Programm, dieselbe Oberfläche, nichts zu migrieren.

Unagikuma ist zuerst für einen einzigen Rechner gebaut. Dort funktioniert alles: Prüfungen, Alarme, Statusseiten, Verlauf. Weitere Rechner bringen nur weitere Blickwinkel.

### Ein Rechner: sofort einsatzbereit

Datei starten, einen Monitor anlegen, fertig. Keine Datenbank, keine Agenten, kein zweiter Server. So nutzen es die meisten, und es reicht für sich allein.

![Die Übersicht: wo es hakt, von diesem Rechner bis zu jedem Ziel](docs/images/overview.png)

### Zwei Rechner: jeder wacht für sich

Stell einen zweiten Server woanders hin, etwa in ein anderes Büro oder zu einem anderen Anbieter. Öffne auf dem ersten **Einstellungen › Knoten › Beitrittscode**, auf dem zweiten **Einstellungen › Knoten › Cluster beitreten** und füge den Code ein. Der zweite Server erhält sofort eine Kopie der Konfiguration und beginnt gleich zu prüfen.

- **Beide Server prüfen und alarmieren unabhängig.** Fällt einer aus, arbeitet der andere weiter und schickt weiter Alarme.
- **Kein Raten.** Verlieren sich zwei Server aus den Augen, kann keiner wissen, welche Seite abgeschnitten ist; also erklärt keiner den anderen für tot. Beide prüfen und alarmieren weiter selbst.
- **Jede Nachricht nennt ihren Standort** (`[Tokyo] …`), damit du immer weißt, woher ein Alarm kommt.

### Ein Cluster: alle Standorte, ein Bild

Füge auf dieselbe Weise beliebig viele Standorte hinzu. Jeder Standort prüft und alarmiert weiterhin selbst; fällt einer aus, auch der Hauptserver, läuft die Überwachung weiter. Der Cluster bringt den Zusammenhang: Jeder Monitor und jeder Alarm zeigt, was die anderen Standorte gerade sehen.

![Ein ausgefallener Monitor mit dem Ergebnis jedes Standorts](docs/images/detail-sites.png)

- **„Überall down" oder „nur von hier aus down"?** Die Beobachtungen aller Standorte zeigen auf einen Blick, ob ein Dienst wirklich ausgefallen ist oder nur ein Netzweg gestört. Veraltete Ergebnisse sind als solche markiert.
- **Ein verstummter Standort wird einmal gemeldet,** nach einer Minute (einstellbar), mit einer Entwarnung, wenn er zurück ist. Ein Neustart des Hauptservers löst keine Welle von „Standort nicht erreichbar" aus.
- **Änderungen erreichen alle Standorte in Sekunden,** und jeder Standort kann eigene Monitore behalten, etwa den Router in dem Büro.

![Die Knotenseite: jeder Standort, seine Laufzeit und seine Konfiguration](docs/images/nodes.png)

## Alarmfluten: ein Problem, eine Nachricht

Ein Überwachungswerkzeug, das dich wegen eines kaputten Kabels hundertmal anpiept, wird stummgeschaltet und ist dann nutzlos. Unagikuma behandelt eine Alarmflut als Fehler, der verhindert werden muss:

- **Zuerst das eigene Netz.** Verliert dieser Rechner seine eigene Internetverbindung, erkennt Unagikuma, dass alle Ziele gemeinsam ausgefallen sind, und schickt eine Nachricht über das Netz statt einer pro Monitor. Ist das Netz zurück, werden die zurückgehaltenen Monitore nach und nach neu geprüft, bevor etwas gemeldet wird.
- **Eltern vor Kindern.** Ist ein Host ausgefallen, werden seine Dienste nicht einzeln gemeldet.
- **Höchstens drei Alarme pro Minute und Kanal.** Alles darüber wird am Ende der Minute zu einer Zusammenfassung, das Schlimmste zuerst. Neue Probleme haben ein eigenes Kontingent und werden nie von Erinnerungen ausgebremst, doch hundert gleichzeitig ausgefallene Dienste kommen trotzdem als eine Zusammenfassung an, nicht als hundert Nachrichten.
- **Gute Nachrichten werden nie zurückgehalten.** Entwarnungen und Eskalationen gehen immer sofort raus; du wartest nie auf die Nachricht, dass du aufhören kannst.
- **Flattern wird zusammengefasst.** Ein Dienst, der kommt und geht, wird als ein Vorfall mit Zähler gemeldet.
- **Geplante Arbeiten bleiben still.** Wartungsfenster bringen zum Schweigen, woran du angekündigt arbeitest.

Jede Grenze lässt sich unter **Einstellungen › Stellgrößen** anpassen.

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
