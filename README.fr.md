# Unagikuma

<p align="center"><img src="docs/images/logo.png" alt="Unagikuma" width="240"></p>

**Encore un outil de surveillance de disponibilité : u(ptime) + nagi(os) + kuma(Uptime Kuma).**

Une supervision auto-hébergée et simple : sachez dès qu’un site ou un serveur tombe, sans être noyé sous les alertes.

[English](README.md) · [中文](README.zh.md) · [हिन्दी](README.hi.md) · [Español](README.es.md) · [العربية](README.ar.md) · **Français** · [বাংলা](README.bn.md) · [Português](README.pt.md) · [Русский](README.ru.md) · [Bahasa Indonesia](README.id.md) · [اردو](README.ur.md) · [Deutsch](README.de.md) · [日本語](README.ja.md) · [Tiếng Việt](README.vi.md) · [한국어](README.ko.md)

![Une courte visite de l’interface web d’Unagikuma](docs/images/tour.gif)

## Pourquoi Unagikuma

- **Prêt en 30 secondes environ.** Téléchargez un fichier, lancez-le, ouvrez votre navigateur.
- **Un seul fichier, rien d’autre à installer.** Fonctionne sous Linux, macOS, Windows et FreeBSD.
- **Surveille l’essentiel.** Sites web, serveurs, ports, ping, DNS, certificats SSL et expiration des noms de domaine.
- **Des alertes là où vous êtes.** E-mail, Telegram, Slack, Discord, Feishu, DingTalk, WeCom, webhooks et plus encore.
- **Un problème, pas une avalanche d’alertes.** Si c’est votre propre réseau qui tombe, Unagikuma s’en rend compte et ne vous alerte pas pour chaque sonde.
- **Des pages de statut** pour tenir vos utilisateurs informés.
- **Grandit avec vous.** Surveillez depuis plusieurs sites quand vous en avez besoin (aperçu).
- **Facile à adopter.** Récupérez vos sondes depuis Uptime Kuma ou Nagios.
- **Parle votre langue.** L’interface existe en 15 langues.

## Démarrage rapide

**1.** Téléchargez le fichier correspondant à votre système depuis [Releases](https://github.com/githubflyideas/unagikuma/releases/latest) et décompressez-le.

**2.** Lancez-le :

```sh
./unagikuma
```

Sous Windows, lancez `unagikuma.exe`.

**3.** Ouvrez http://localhost:8077 dans votre navigateur et créez votre compte administrateur.

**4.** Cliquez sur **Ajouter une sonde**, saisissez l’adresse d’un site, et c’est tout.

> Vous l’ouvrez depuis un autre ordinateur ? Allez sur `http://<adresse-du-serveur>:8077` et saisissez le **Jeton d’installation** affiché par le programme lors de son premier démarrage.

## Vous préférez Docker ?

```sh
docker run -d --name unagikuma -p 8077:8077 -v unagikuma-data:/data ghcr.io/githubflyideas/unagikuma:latest
```

Ouvrez ensuite http://localhost:8077. Le **Jeton d’installation** s’affiche avec `docker logs unagikuma`.

## Venir d'Uptime Kuma

Arrêtez Uptime Kuma, puis regardez d'abord (rien n'est modifié) :

```sh
./unagikuma import kuma --dry-run /path/to/uptime-kuma/data/kuma.db
```

Si tout est bon, importez :

```sh
./unagikuma import kuma /path/to/uptime-kuma/data/kuma.db
```

Le rapport indique ce qui a été importé, ce qu'il faut vérifier et ce qui n'a pas pu l'être, avec quoi faire. Il est aussi enregistré dans `data/import-reports/`. Relancer la commande n'ajoute que ce qui est nouveau.

## Venir de Nagios

Indiquez votre `nagios.cfg` et regardez d'abord (rien n'est modifié) :

```sh
./unagikuma import nagios --dry-run /etc/nagios4/nagios.cfg
```

Si tout est bon, importez :

```sh
./unagikuma import nagios /etc/nagios4/nagios.cfg
```

Le rapport indique ce qui a été importé, ce qu'il faut vérifier et ce qui n'a pas pu l'être, avec quoi faire. Il est aussi enregistré dans `data/import-reports/`. Relancer la commande n'ajoute que ce qui est nouveau.

## Mot de passe oublié ?

Sur la machine où tourne Unagikuma :

```sh
./unagikuma reset-password admin
```

Tapez le nouveau mot de passe deux fois (il ne s'affiche pas). Ajoutez `--generate` pour en obtenir un au hasard. Avec Docker :

```sh
docker exec -it unagikuma /unagikuma reset-password admin
```

## En savoir plus

- [Manuel d’utilisation (en anglais)](docs/manual.md) — tout le reste, étape par étape
- [Journal des modifications](CHANGELOG.md)

## Licence

Apache License 2.0, voir [LICENSE](LICENSE).
