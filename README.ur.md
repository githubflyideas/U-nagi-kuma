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
- **منتقل ہونا آسان۔** اپنے مانیٹرز Uptime Kuma یا Nagios سے لے آئیں (پیش نظارہ)۔
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

## مزید جانیں

- [صارف گائیڈ (انگریزی میں)](docs/manual.md) — باقی سب کچھ، قدم بہ قدم
- [تبدیلیوں کی فہرست](CHANGELOG.md)

</div>

<div dir="rtl">

## لائسنس

Apache License 2.0، دیکھیں [LICENSE](LICENSE)۔

</div>
