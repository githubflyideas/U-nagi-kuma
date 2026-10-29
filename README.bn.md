# Unagikuma

<p align="center"><img src="docs/images/logo.png" alt="Unagikuma" width="240"></p>

**আরও একটি আপটাইম পরীক্ষার টুল: u(ptime) + nagi(os) + kuma(Uptime Kuma)।**

সহজ, নিজের সার্ভারে চালানো মনিটরিং: আপনার ওয়েবসাইট বা সার্ভার বন্ধ হলেই সঙ্গে সঙ্গে জানুন, বাড়তি ঝামেলা ছাড়াই।

[English](README.md) · [中文](README.zh.md) · [हिन्दी](README.hi.md) · [Español](README.es.md) · [العربية](README.ar.md) · [Français](README.fr.md) · **বাংলা** · [Português](README.pt.md) · [Русский](README.ru.md) · [Bahasa Indonesia](README.id.md) · [اردو](README.ur.md) · [Deutsch](README.de.md) · [日本語](README.ja.md) · [Tiếng Việt](README.vi.md) · [한국어](README.ko.md)

![Unagikuma ওয়েব ইন্টারফেসের একটি ছোট পরিচিতি](docs/images/tour.gif)

## কেন Unagikuma

- **প্রায় ৩০ সেকেন্ডে প্রস্তুত।** একটি ফাইল ডাউনলোড করুন, চালান, তারপর ব্রাউজার খুলুন।
- **একটিই ফাইল, আর কিছু ইনস্টল করতে হয় না।** Linux, macOS, Windows ও FreeBSD-তে চলে।
- **যা দরকার তা-ই দেখে।** ওয়েবসাইট, সার্ভার, পোর্ট, পিং, DNS, SSL সার্টিফিকেট আর ডোমেনের মেয়াদ।
- **অ্যালার্ট পৌঁছায় যেখানে আপনি আছেন।** ইমেইল, Telegram, Slack, Discord, Feishu, DingTalk, WeCom, ওয়েবহুক ও আরও অনেক কিছু।
- **একটি সমস্যা, অ্যালার্টের বন্যা নয়।** আপনার নিজের নেটওয়ার্ক বন্ধ হলে Unagikuma তা বুঝতে পারে এবং প্রতিটি মনিটরের জন্য আলাদা অ্যালার্ট পাঠায় না।
- **স্ট্যাটাস পেজ**, যাতে আপনার ব্যবহারকারীরা সবসময় খবর পান।
- **আপনার সঙ্গে বাড়ে।** দরকার হলে একাধিক জায়গা থেকে পরীক্ষা করুন (প্রিভিউ)।
- **বদলানো সহজ।** Uptime Kuma বা Nagios থেকে আপনার মনিটরগুলো নিয়ে আসুন (প্রিভিউ)।
- **আপনার ভাষায়।** ইন্টারফেসটি ১৫টি ভাষায় পাওয়া যায়।

## দ্রুত শুরু

**1.** [Releases](https://github.com/githubflyideas/unagikuma/releases/latest) থেকে আপনার সিস্টেমের ফাইলটি ডাউনলোড করে আনপ্যাক করুন।

**2.** চালান:

```sh
./unagikuma
```

Windows-এ `unagikuma.exe` চালান।

**3.** ব্রাউজারে http://localhost:8077 খুলুন এবং আপনার অ্যাডমিন অ্যাকাউন্ট তৈরি করুন।

**4.** **মনিটর যোগ করুন**-এ ক্লিক করুন, একটি ওয়েবসাইটের ঠিকানা দিন, ব্যস, হয়ে গেল।

> অন্য কম্পিউটার থেকে খুলছেন? `http://<সার্ভারের-ঠিকানা>:8077`-এ যান এবং প্রোগ্রামটি প্রথমবার চালুর সময় যে **সেটআপ টোকেন** দেখিয়েছিল সেটি দিন।

## Docker পছন্দ?

```sh
docker run -d --name unagikuma -p 8077:8077 -v unagikuma-data:/data ghcr.io/githubflyideas/unagikuma:latest
```

তারপর http://localhost:8077 খুলুন। **সেটআপ টোকেন** পাবেন `docker logs unagikuma` দিয়ে।

## আরও জানুন

- [ব্যবহারকারী নির্দেশিকা (ইংরেজিতে)](docs/manual.md) — বাকি সবকিছু, ধাপে ধাপে
- [পরিবর্তনের তালিকা (Changelog)](CHANGELOG.md)

## লাইসেন্স

Apache License 2.0, দেখুন [LICENSE](LICENSE)।
