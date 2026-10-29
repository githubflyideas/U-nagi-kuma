# Unagikuma

<p align="center"><img src="docs/images/logo.png" alt="Unagikuma" width="240"></p>

**Ещё один инструмент проверки доступности: u(ptime) + nagi(os) + kuma(Uptime Kuma).**

Простой мониторинг на вашем собственном сервере: узнавайте сразу, когда сайт или сервер упал, без лишнего шума.

[English](README.md) · [中文](README.zh.md) · [हिन्दी](README.hi.md) · [Español](README.es.md) · [العربية](README.ar.md) · [Français](README.fr.md) · [বাংলা](README.bn.md) · [Português](README.pt.md) · **Русский** · [Bahasa Indonesia](README.id.md) · [اردو](README.ur.md) · [Deutsch](README.de.md) · [日本語](README.ja.md) · [Tiếng Việt](README.vi.md) · [한국어](README.ko.md)

![Короткий обзор веб-интерфейса Unagikuma](docs/images/tour.gif)

## Почему Unagikuma

- **Готов к работе примерно за 30 секунд.** Скачайте один файл, запустите его и откройте браузер.
- **Один файл, больше ничего ставить не нужно.** Работает в Linux, macOS, Windows и FreeBSD.
- **Следит за главным.** Сайты, серверы, порты, ping, DNS, SSL-сертификаты и срок действия доменов.
- **Оповещения туда, где вы уже есть.** Почта, Telegram, Slack, Discord, Feishu, DingTalk, WeCom, вебхуки и многое другое.
- **Одна проблема, а не лавина оповещений.** Если пропала связь у вас самих, Unagikuma это заметит и не станет присылать оповещение по каждому монитору.
- **Страницы статуса**, чтобы ваши пользователи всегда были в курсе.
- **Растёт вместе с вами.** При необходимости проверяйте из нескольких мест (предварительная версия).
- **Легко переехать.** Перенесите мониторы из Uptime Kuma или Nagios.
- **Говорит на вашем языке.** Интерфейс доступен на 15 языках.

## Быстрый старт

**1.** Скачайте файл для своей системы со страницы [Releases](https://github.com/githubflyideas/unagikuma/releases/latest) и распакуйте его.

**2.** Запустите:

```sh
./unagikuma
```

В Windows запустите `unagikuma.exe`.

**3.** Откройте в браузере http://localhost:8077 и создайте учётную запись администратора.

**4.** Нажмите **Добавить монитор**, введите адрес сайта — готово.

> Открываете с другого компьютера? Перейдите на `http://<адрес-сервера>:8077` и введите **Токен настройки**, который программа показала при первом запуске.

## Удобнее через Docker?

```sh
docker run -d --name unagikuma -p 8077:8077 -v unagikuma-data:/data ghcr.io/githubflyideas/unagikuma:latest
```

Затем откройте http://localhost:8077. **Токен настройки** можно посмотреть командой `docker logs unagikuma`.

## Переезд с Uptime Kuma

Остановите Uptime Kuma и сначала посмотрите (ничего не меняется):

```sh
./unagikuma import kuma --dry-run /path/to/uptime-kuma/data/kuma.db
```

Если всё в порядке, импортируйте:

```sh
./unagikuma import kuma /path/to/uptime-kuma/data/kuma.db
```

Отчёт показывает, что импортировано, что стоит проверить и что импортировать не удалось, и что с этим делать. Он также сохраняется в `data/import-reports/`. Повторный запуск добавляет только новое.

## Переезд с Nagios

Укажите `nagios.cfg` и сначала посмотрите (ничего не меняется):

```sh
./unagikuma import nagios --dry-run /etc/nagios4/nagios.cfg
```

Если всё в порядке, импортируйте:

```sh
./unagikuma import nagios /etc/nagios4/nagios.cfg
```

Отчёт показывает, что импортировано, что стоит проверить и что импортировать не удалось, и что с этим делать. Он также сохраняется в `data/import-reports/`. Повторный запуск добавляет только новое.

## Забыли пароль?

На машине, где работает Unagikuma:

```sh
./unagikuma reset-password admin
```

Введите новый пароль дважды (он не отображается). Добавьте `--generate`, чтобы получить случайный пароль. С Docker:

```sh
docker exec -it unagikuma /unagikuma reset-password admin
```

## Подробнее

- [Руководство пользователя (на английском)](docs/manual.md) — всё остальное, шаг за шагом
- [История изменений](CHANGELOG.md)

## Лицензия

Apache License 2.0, см. [LICENSE](LICENSE).
