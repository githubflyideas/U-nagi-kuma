# Unagikuma

<p align="center"><img src="docs/images/IMG_8128.jpeg" alt="Unagikuma" width="240"></p>

<div dir="rtl">

**أداة أخرى لمراقبة جاهزية الخدمات: u(ptime) + nagi(os) + kuma(Uptime Kuma).**

**عصفوران بحجر واحد:** ملف واحد على جهاز واحد يعمل فورًا، وحين تريد المزيد ضمّ أجهزة أخرى فيصبح عنقودًا. البرنامج نفسه، ولا شيء تنقله.

مراقبة سهلة تستضيفها بنفسك: اعرف فورًا متى يتوقف موقعك أو خادمك، دون إزعاج.

</div>

[English](README.md) · [中文](README.zh.md) · [हिन्दी](README.hi.md) · [Español](README.es.md) · **العربية** · [Français](README.fr.md) · [বাংলা](README.bn.md) · [Português](README.pt.md) · [Русский](README.ru.md) · [Bahasa Indonesia](README.id.md) · [اردو](README.ur.md) · [Deutsch](README.de.md) · [日本語](README.ja.md) · [Tiếng Việt](README.vi.md) · [한국어](README.ko.md)

![قائمة المراقبات بدرجة «ورقي»](docs/images/monitors-paper.png)

<div dir="rtl">

## لماذا Unagikuma؟

- **جاهز خلال 30 ثانية تقريبًا.** نزّل ملفًا واحدًا، شغّله، ثم افتح المتصفح.
- **ملف واحد، ولا شيء آخر لتثبيته.** يعمل على Linux وmacOS وWindows وFreeBSD.
- **يراقب ما يهمك.** المواقع والخوادم والمنافذ وping وDNS وشهادات SSL وتاريخ انتهاء النطاقات.
- **تنبيهات حيث تكون.** البريد الإلكتروني وTelegram وSlack وDiscord وFeishu وDingTalk وWeCom وwebhooks وغيرها.
- **صفحات حالة** تُبقي مستخدميك على اطلاع.
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

## جهاز واحد، جهازان، عنقود

- **جهاز واحد:** شغّل الملف وأضف مراقبة، وانتهى الأمر. لا قاعدة بيانات ولا وكلاء.
- **جهازان:** الصق رمز انضمام في **الإعدادات › العقد**. يفحص كل جهاز وينبّه بنفسه، فإن توقف أحدهما لا تنقطع التنبيهات.
- **عنقود:** أضف ما تشاء من المواقع. كل تنبيه يعرض ما يراه كل موقع، لتفرّق بين «متوقف في كل مكان» و«متوقف من هنا فقط».

![مراقبة متعطلة مع نتيجة كل موقع](docs/images/detail-sites.png)

## لا سيل من التنبيهات

- **انقطعت شبكتك أنت؟** رسالة واحدة عن الشبكة، لا رسالة لكل مراقبة.
- **توقف مضيف؟** لا يُبلَّغ عن خدماته واحدةً واحدة.
- **ثلاثة تنبيهات على الأكثر في الدقيقة لكل قناة؛** والباقي يصل في ملخص واحد. إشعارات التعافي تُرسل دائمًا فورًا.

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

اكتب كلمة المرور الجديدة مرتين (لن تظهر). أضف `--generate` للحصول على كلمة مرور عشوائية بدلًا من ذلك.

</div>

<div dir="rtl">

## لمعرفة المزيد

- [دليل المستخدم (بالإنجليزية)](docs/manual.md) — كل ما تبقّى، خطوة بخطوة
- [سجل التغييرات](CHANGELOG.md)

</div>

<div dir="rtl">

## الترخيص

Apache License 2.0، راجع [LICENSE](LICENSE).

</div>
