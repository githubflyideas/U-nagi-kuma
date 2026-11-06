# Unagikuma

<p align="center"><img src="docs/images/IMG_8128.jpeg" alt="Unagikuma" width="240"></p>

**Encore un outil de surveillance de disponibilité : u(ptime) + nagi(os) + kuma(Uptime Kuma).**

Une supervision auto-hébergée et simple : sachez dès qu’un site ou un serveur tombe, sans être noyé sous les alertes.

[English](README.md) · [中文](README.zh.md) · [हिन्दी](README.hi.md) · [Español](README.es.md) · [العربية](README.ar.md) · **Français** · [বাংলা](README.bn.md) · [Português](README.pt.md) · [Русский](README.ru.md) · [Bahasa Indonesia](README.id.md) · [اردو](README.ur.md) · [Deutsch](README.de.md) · [日本語](README.ja.md) · [Tiếng Việt](README.vi.md) · [한국어](README.ko.md)

![La liste des moniteurs dans le ton Papier](docs/images/monitors-paper.png)

## Pourquoi Unagikuma

- **Prêt en 30 secondes environ.** Téléchargez un fichier, lancez-le, ouvrez votre navigateur.
- **Un seul fichier, rien d’autre à installer.** Fonctionne sous Linux, macOS, Windows et FreeBSD.
- **Surveille l’essentiel.** Sites web, serveurs, ports, ping, DNS, certificats SSL et expiration des noms de domaine.
- **Des alertes là où vous êtes.** E-mail, Telegram, Slack, Discord, Feishu, DingTalk, WeCom, webhooks et plus encore.
- **Un problème, pas une avalanche d’alertes.** Si c’est votre propre réseau qui tombe, Unagikuma s’en rend compte et ne vous alerte pas pour chaque sonde.
- **Des pages de statut** pour tenir vos utilisateurs informés.
- **Grandit avec vous.** Ajoutez une seconde machine ou tout un cluster en une minute quand vous en avez besoin, avec un code depuis la page web.
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

## Une machine, deux machines, un cluster

> **Le beurre et l'argent du beurre.** La plupart des outils vous font choisir : un petit, facile à lancer mais limité à une machine, ou un gros, qui passe à l'échelle mais demande une base de données, des agents et un après-midi d'installation. Unagikuma est les deux. Commencez avec un seul fichier sur une seule machine ; quand vous voulez plus, joignez-y d'autres machines. Même programme, même interface, rien à migrer.

Unagikuma est d'abord conçu pour une seule machine. Tout y fonctionne : vérifications, alertes, pages de statut, historique. Ajouter des machines ne fait qu'ajouter des points de vue.

### Une machine : prêt tout de suite

Lancez le fichier et ajoutez un moniteur. Rien d'autre à installer : ni base de données, ni agents, ni second serveur. C'est ainsi que la plupart des gens l'utilisent, et cela suffit à lui seul.

![La vue d'ensemble : où ça casse, de cette machine jusqu'à chaque cible](docs/images/overview.png)

### Deux machines : chacune surveille de son côté

Placez un second serveur ailleurs, par exemple dans un autre bureau ou chez un autre hébergeur. Sur le premier, ouvrez **Paramètres › Nœuds › Code d'adhésion** ; sur le second, **Paramètres › Nœuds › Rejoindre un cluster** et collez le code. Le second serveur reçoit aussitôt une copie de la configuration et commence à vérifier immédiatement.

- **Les deux serveurs vérifient et alertent indépendamment.** Si l'un tombe, l'autre continue de fonctionner et d'envoyer des alertes.
- **Pas de devinettes.** Quand deux serveurs se perdent de vue, aucun ne peut savoir quel côté est coupé ; aucun ne déclare donc l'autre mort. Les deux continuent de vérifier et d'alerter chacun de leur côté.
- **Chaque message indique son site** (`[Tokyo] …`), pour savoir toujours d'où vient une alerte.

### Un cluster : tous les sites, une seule image

Ajoutez autant de sites que vous voulez de la même façon. Chaque site continue de vérifier et d'alerter seul ; en perdre un, y compris le serveur principal, n'arrête jamais la surveillance. Ce que le cluster ajoute, c'est le contexte : chaque moniteur et chaque alerte montrent ce que les autres sites voient en ce moment.

![Un moniteur en panne avec le résultat de chaque site](docs/images/detail-sites.png)

- **« En panne partout » ou « en panne d'ici » ?** Les observations de chaque site montrent d'un coup d'œil si un service est vraiment en panne ou si un seul chemin réseau est coupé. Les résultats périmés sont signalés comme tels.
- **Un site qui se tait est signalé une seule fois,** au bout d'une minute (réglable), avec un avis de rétablissement à son retour. Un redémarrage du serveur principal ne déclenche pas une vague de « site injoignable ».
- **Les changements atteignent tous les sites en quelques secondes,** et chaque site peut garder ses propres moniteurs, comme le routeur de ce bureau.

![La page des nœuds : chaque site, son aller-retour et sa configuration](docs/images/nodes.png)

## Avalanches d'alertes : un problème, un message

Un outil qui vous sonne cent fois pour un câble coupé finit en sourdine, et devient inutile. Unagikuma traite une avalanche d'alertes comme un bug à éviter :

- **D'abord votre propre réseau.** Quand cette machine perd sa propre connexion internet, Unagikuma voit que toutes ses cibles ont échoué ensemble et envoie un seul message sur le réseau au lieu d'un par moniteur. Quand le réseau revient, les moniteurs mis de côté sont revérifiés progressivement avant tout signalement.
- **Les parents avant les enfants.** Quand un hôte est en panne, ses services ne sont pas signalés un par un.
- **Au plus trois alertes par minute et par canal.** Le reste est regroupé en un seul résumé à la fin de la minute, le plus grave d'abord. Les nouveaux problèmes ont leur propre quota, donc les rappels ne les retardent jamais, mais cent services en panne ensemble arrivent quand même en un seul résumé, pas en cent messages.
- **Les bonnes nouvelles ne sont jamais retenues.** Les rétablissements et les escalades partent toujours immédiatement : vous n'attendez jamais le message qui vous dit d'arrêter.
- **Les oscillations sont résumées.** Un service qui monte et descend est signalé comme un seul incident avec un compteur.
- **Les travaux prévus restent silencieux.** Les fenêtres de maintenance font taire ce sur quoi vous avez annoncé travailler.

Chaque limite se règle dans **Paramètres › Paramètres ajustables**.

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

Tapez le nouveau mot de passe deux fois (il ne s'affiche pas). Ajoutez `--generate` pour en obtenir un au hasard.

## En savoir plus

- [Manuel d’utilisation (en anglais)](docs/manual.md) — tout le reste, étape par étape
- [Journal des modifications](CHANGELOG.md)

## Licence

Apache License 2.0, voir [LICENSE](LICENSE).
