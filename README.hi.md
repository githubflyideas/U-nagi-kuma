# Unagikuma

<p align="center"><img src="docs/images/logo.png" alt="Unagikuma" width="240"></p>

**एक और अपटाइम जाँच टूल: u(ptime) + nagi(os) + kuma(Uptime Kuma)।**

आसान, सेल्फ़-होस्टेड मॉनिटरिंग: आपकी वेबसाइट या सर्वर बंद होते ही आपको पता चल जाए, बिना फ़ालतू शोर के।

[English](README.md) · [中文](README.zh.md) · **हिन्दी** · [Español](README.es.md) · [العربية](README.ar.md) · [Français](README.fr.md) · [বাংলা](README.bn.md) · [Português](README.pt.md) · [Русский](README.ru.md) · [Bahasa Indonesia](README.id.md) · [اردو](README.ur.md) · [Deutsch](README.de.md) · [日本語](README.ja.md) · [Tiếng Việt](README.vi.md) · [한국어](README.ko.md)

![Unagikuma वेब इंटरफ़ेस की एक छोटी झलक](docs/images/tour.gif)

## Unagikuma क्यों

- **लगभग 30 सेकंड में तैयार।** एक फ़ाइल डाउनलोड करें, चलाएँ और ब्राउज़र खोलें।
- **सिर्फ़ एक फ़ाइल, और कुछ इंस्टॉल नहीं करना।** Linux, macOS, Windows और FreeBSD पर चलता है।
- **ज़रूरी चीज़ों की जाँच।** वेबसाइट, सर्वर, पोर्ट, पिंग, DNS, SSL सर्टिफ़िकेट और डोमेन की समाप्ति तिथि।
- **अलर्ट वहीं, जहाँ आप हैं।** ईमेल, Telegram, Slack, Discord, Feishu, DingTalk, WeCom, वेबहुक और भी बहुत कुछ।
- **एक समस्या, अलर्ट की बाढ़ नहीं।** अगर आपका अपना नेटवर्क बंद हो जाए, तो Unagikuma इसे पहचान लेता है और हर मॉनिटर के लिए अलग-अलग अलर्ट नहीं भेजता।
- **स्टेटस पेज**, ताकि आपके यूज़र्स को हमेशा जानकारी मिलती रहे।
- **आपके साथ बढ़ता है।** ज़रूरत पड़ने पर कई जगहों से जाँच करें (प्रीव्यू)।
- **बदलना आसान।** अपने मॉनिटर Uptime Kuma या Nagios से ले आएँ (प्रीव्यू)।
- **आपकी भाषा में।** इंटरफ़ेस 15 भाषाओं में उपलब्ध है।

## जल्दी शुरू करें

**1.** [Releases](https://github.com/githubflyideas/unagikuma/releases/latest) से अपने सिस्टम के लिए फ़ाइल डाउनलोड करें और उसे अनपैक करें।

**2.** इसे चलाएँ:

```sh
./unagikuma
```

Windows पर `unagikuma.exe` चलाएँ।

**3.** ब्राउज़र में http://localhost:8077 खोलें और अपना एडमिन अकाउंट बनाएँ।

**4.** **मॉनिटर जोड़ें** पर क्लिक करें, वेबसाइट का पता डालें, बस हो गया।

> किसी दूसरे कंप्यूटर से खोल रहे हैं? `http://<सर्वर-पता>:8077` पर जाएँ और वह **सेटअप टोकन** डालें जो प्रोग्राम ने पहली बार शुरू होते समय दिखाया था।

## Docker पसंद है?

```sh
docker run -d --name unagikuma -p 8077:8077 -v unagikuma-data:/data ghcr.io/githubflyideas/unagikuma:latest
```

फिर http://localhost:8077 खोलें। **सेटअप टोकन** आपको `docker logs unagikuma` में मिल जाएगा।

## और जानें

- [यूज़र मैनुअल (अंग्रेज़ी में)](docs/manual.md) — बाक़ी सब कुछ, क़दम-दर-क़दम
- [बदलावों की सूची (Changelog)](CHANGELOG.md)

## लाइसेंस

Apache License 2.0, देखें [LICENSE](LICENSE)।
