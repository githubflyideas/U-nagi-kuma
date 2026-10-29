# Unagikuma

<p align="center"><img src="docs/images/logo.png" alt="Unagikuma" width="240"></p>

<div dir="rtl">

**اپ ٹائم جانچنے کا ایک اور ٹول: u(ptime) + nagi(os) + kuma(Uptime Kuma)۔**

آسان، اپنے سرور پر چلنے والی مانیٹرنگ: آپ کی ویب سائٹ یا سرور بند ہوتے ہی فوراً پتا چل جائے، بغیر فضول شور کے۔

</div>

[English](README.md) · [中文](README.zh.md) · [हिन्दी](README.hi.md) · [Español](README.es.md) · [العربية](README.ar.md) · [Français](README.fr.md) · [বাংলা](README.bn.md) · [Português](README.pt.md) · [Русский](README.ru.md) · [Bahasa Indonesia](README.id.md) · **اردو** · [Deutsch](README.de.md) · [日本語](README.ja.md) · [Tiếng Việt](README.vi.md) · [한국어](README.ko.md)

![Unagikuma کے ویب انٹرفیس کا ایک مختصر جائزہ](docs/images/tour.gif)

<div dir="rtl">

## Unagikuma کیوں؟

- **تقریباً 30 سیکنڈ میں تیار۔** ایک فائل ڈاؤن لوڈ کریں، چلائیں اور براؤزر کھولیں۔
- **صرف ایک فائل، اور کچھ انسٹال نہیں کرنا۔** Linux، macOS، Windows اور FreeBSD پر چلتا ہے۔
- **ضروری چیزوں کی جانچ۔** ویب سائٹس، سرورز، پورٹس، پنگ، DNS، SSL سرٹیفکیٹ اور ڈومین کی میعاد۔
- **الرٹ وہیں جہاں آپ ہیں۔** ای میل، Telegram، Slack، Discord، Feishu، DingTalk، WeCom، ویب ہُکس اور بہت کچھ۔
- **ایک مسئلہ، الرٹس کا سیلاب نہیں۔** اگر آپ کا اپنا نیٹ ورک بند ہو جائے تو Unagikuma اسے پہچان لیتا ہے اور ہر مانیٹر کے لیے الگ الرٹ نہیں بھیجتا۔
- **اسٹیٹس پیجز** تاکہ آپ کے صارفین ہمیشہ باخبر رہیں۔
- **آپ کے ساتھ بڑھتا ہے۔** ضرورت ہو تو کئی جگہوں سے جانچ کریں (پیش نظارہ)۔
- **منتقل ہونا آسان۔** اپنے مانیٹرز Uptime Kuma یا Nagios سے لے آئیں۔
- **آپ کی زبان میں۔** انٹرفیس 15 زبانوں میں دستیاب ہے۔

</div>

<div dir="rtl">

## فوری آغاز

**1.** [Releases](https://github.com/githubflyideas/unagikuma/releases/latest) سے اپنے سسٹم کے لیے فائل ڈاؤن لوڈ کریں اور اسے کھولیں (ان پیک کریں)۔

**2.** اسے چلائیں:

</div>

```sh
./unagikuma
```

<div dir="rtl">

Windows پر `unagikuma.exe` چلائیں۔

**3.** براؤزر میں http://localhost:8077 کھولیں اور اپنا ایڈمن اکاؤنٹ بنائیں۔

**4.** **مانیٹر شامل کریں** پر کلک کریں، کسی ویب سائٹ کا پتا لکھیں، بس ہو گیا۔

> کسی دوسرے کمپیوٹر سے کھول رہے ہیں؟ `http://<سرور-کا-پتا>:8077` پر جائیں اور وہ **سیٹ اپ ٹوکن** درج کریں جو پروگرام نے پہلی بار شروع ہوتے وقت دکھایا تھا۔

</div>

<div dir="rtl">

## Docker پسند ہے؟

</div>

```sh
docker run -d --name unagikuma -p 8077:8077 -v unagikuma-data:/data ghcr.io/githubflyideas/unagikuma:latest
```

<div dir="rtl">

پھر http://localhost:8077 کھولیں۔ **سیٹ اپ ٹوکن** آپ کو `docker logs unagikuma` سے مل جائے گا۔

</div>

<div dir="rtl">

## Uptime Kuma سے منتقلی

Uptime Kuma بند کریں، پھر پہلے دیکھ لیں (کچھ نہیں بدلتا):

</div>

```sh
./unagikuma import kuma --dry-run /path/to/uptime-kuma/data/kuma.db
```

<div dir="rtl">

اگر سب ٹھیک لگے تو امپورٹ کریں:

</div>

```sh
./unagikuma import kuma /path/to/uptime-kuma/data/kuma.db
```

<div dir="rtl">

رپورٹ بتاتی ہے کہ کیا امپورٹ ہوا، کسے دیکھنا ضروری ہے اور کیا امپورٹ نہیں ہو سکا، اور کیا کرنا ہے۔ یہ `data/import-reports/` میں بھی محفوظ ہوتی ہے۔ دوبارہ چلانے پر صرف نئی چیزیں شامل ہوتی ہیں۔

</div>

<div dir="rtl">

## Nagios سے منتقلی

اسے `nagios.cfg` دیں اور پہلے دیکھ لیں (کچھ نہیں بدلتا):

</div>

```sh
./unagikuma import nagios --dry-run /etc/nagios4/nagios.cfg
```

<div dir="rtl">

اگر سب ٹھیک لگے تو امپورٹ کریں:

</div>

```sh
./unagikuma import nagios /etc/nagios4/nagios.cfg
```

<div dir="rtl">

رپورٹ بتاتی ہے کہ کیا امپورٹ ہوا، کسے دیکھنا ضروری ہے اور کیا امپورٹ نہیں ہو سکا، اور کیا کرنا ہے۔ یہ `data/import-reports/` میں بھی محفوظ ہوتی ہے۔ دوبارہ چلانے پر صرف نئی چیزیں شامل ہوتی ہیں۔

</div>

<div dir="rtl">

## پاس ورڈ بھول گئے؟

جس مشین پر Unagikuma چل رہا ہے وہاں چلائیں:

</div>

```sh
./unagikuma reset-password admin
```

<div dir="rtl">

نیا پاس ورڈ دو بار لکھیں (یہ دکھائی نہیں دے گا)۔ اس کی جگہ ایک بے ترتیب پاس ورڈ کے لیے `--generate` شامل کریں۔ Docker کے ساتھ:

</div>

```sh
docker exec -it unagikuma /unagikuma reset-password admin
```

<div dir="rtl">

## مزید جانیں

- [صارف گائیڈ (انگریزی میں)](docs/manual.md) — باقی سب کچھ، قدم بہ قدم
- [تبدیلیوں کی فہرست](CHANGELOG.md)

</div>

<div dir="rtl">

## لائسنس

Apache License 2.0، دیکھیں [LICENSE](LICENSE)۔

</div>
