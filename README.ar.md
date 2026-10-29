# Unagikuma

<p align="center"><img src="docs/images/logo.png" alt="Unagikuma" width="240"></p>

<div dir="rtl">

**أداة أخرى لمراقبة جاهزية الخدمات: u(ptime) + nagi(os) + kuma(Uptime Kuma).**

مراقبة سهلة تستضيفها بنفسك: اعرف فورًا متى يتوقف موقعك أو خادمك، دون إزعاج.

</div>

[English](README.md) · [中文](README.zh.md) · [हिन्दी](README.hi.md) · [Español](README.es.md) · **العربية** · [Français](README.fr.md) · [বাংলা](README.bn.md) · [Português](README.pt.md) · [Русский](README.ru.md) · [Bahasa Indonesia](README.id.md) · [اردو](README.ur.md) · [Deutsch](README.de.md) · [日本語](README.ja.md) · [Tiếng Việt](README.vi.md) · [한국어](README.ko.md)

![جولة قصيرة في واجهة Unagikuma](docs/images/tour.gif)

<div dir="rtl">

## لماذا Unagikuma؟

- **جاهز خلال 30 ثانية تقريبًا.** نزّل ملفًا واحدًا، شغّله، ثم افتح المتصفح.
- **ملف واحد، ولا شيء آخر لتثبيته.** يعمل على Linux وmacOS وWindows وFreeBSD.
- **يراقب ما يهمك.** المواقع والخوادم والمنافذ وping وDNS وشهادات SSL وتاريخ انتهاء النطاقات.
- **تنبيهات حيث تكون.** البريد الإلكتروني وTelegram وSlack وDiscord وFeishu وDingTalk وWeCom وwebhooks وغيرها.
- **مشكلة واحدة، لا سيل من التنبيهات.** إذا انقطعت شبكتك أنت، يلاحظ Unagikuma ذلك ولا يرسل لك تنبيهًا عن كل مراقبة.
- **صفحات حالة** تُبقي مستخدميك على اطلاع.
- **ينمو معك.** راقب من عدة مواقع عندما تحتاج إلى ذلك (معاينة).
- **الانتقال سهل.** انقل مراقباتك من Uptime Kuma أو Nagios.
- **يتحدث لغتك.** الواجهة متاحة بـ 15 لغة.

</div>

<div dir="rtl">

## البدء السريع

**1.** نزّل الملف المناسب لنظامك من [Releases](https://github.com/githubflyideas/unagikuma/releases/latest) وفُكّ ضغطه.

**2.** شغّله:

</div>

```sh
./unagikuma
```

<div dir="rtl">

على Windows شغّل `unagikuma.exe`.

**3.** افتح http://localhost:8077 في المتصفح وأنشئ حساب المسؤول.

**4.** انقر على **إضافة مراقبة**، وأدخل عنوان موقع، وانتهى الأمر.

> تفتحه من جهاز آخر؟ استخدم `http://<عنوان-الخادم>:8077` وأدخل **رمز الإعداد** الذي عرضه البرنامج عند تشغيله أول مرة.

</div>

<div dir="rtl">

## تفضّل Docker؟

</div>

```sh
docker run -d --name unagikuma -p 8077:8077 -v unagikuma-data:/data ghcr.io/githubflyideas/unagikuma:latest
```

<div dir="rtl">

ثم افتح http://localhost:8077. ستجد **رمز الإعداد** عبر الأمر `docker logs unagikuma`.

</div>

<div dir="rtl">

## الانتقال من Uptime Kuma

أوقف Uptime Kuma، ثم ألقِ نظرة أولًا (لن يتغير شيء):

</div>

```sh
./unagikuma import kuma --dry-run /path/to/uptime-kuma/data/kuma.db
```

<div dir="rtl">

إذا بدا كل شيء صحيحًا، استورد:

</div>

```sh
./unagikuma import kuma /path/to/uptime-kuma/data/kuma.db
```

<div dir="rtl">

يعرض التقرير ما تم استيراده وما يحتاج إلى مراجعة وما تعذّر استيراده، مع ما يجب فعله. ويُحفظ أيضًا في `data/import-reports/`. تشغيله مرة أخرى يضيف الجديد فقط.

</div>

<div dir="rtl">

## الانتقال من Nagios

وجّهه إلى `nagios.cfg` وألقِ نظرة أولًا (لن يتغير شيء):

</div>

```sh
./unagikuma import nagios --dry-run /etc/nagios4/nagios.cfg
```

<div dir="rtl">

إذا بدا كل شيء صحيحًا، استورد:

</div>

```sh
./unagikuma import nagios /etc/nagios4/nagios.cfg
```

<div dir="rtl">

يعرض التقرير ما تم استيراده وما يحتاج إلى مراجعة وما تعذّر استيراده، مع ما يجب فعله. ويُحفظ أيضًا في `data/import-reports/`. تشغيله مرة أخرى يضيف الجديد فقط.

</div>

<div dir="rtl">

## نسيت كلمة المرور؟

على الجهاز الذي يعمل عليه Unagikuma:

</div>

```sh
./unagikuma reset-password admin
```

<div dir="rtl">

اكتب كلمة المرور الجديدة مرتين (لن تظهر). أضف `--generate` للحصول على كلمة مرور عشوائية بدلًا من ذلك. مع Docker:

</div>

```sh
docker exec -it unagikuma /unagikuma reset-password admin
```

<div dir="rtl">

## لمعرفة المزيد

- [دليل المستخدم (بالإنجليزية)](docs/manual.md) — كل ما تبقّى، خطوة بخطوة
- [سجل التغييرات](CHANGELOG.md)

</div>

<div dir="rtl">

## الترخيص

Apache License 2.0، راجع [LICENSE](LICENSE).

</div>
