# Unagikuma

<p align="center"><img src="docs/images/logo.png" alt="Unagikuma" width="240"></p>

**又一个可用性监控工具:u(ptime) + nagi(os) + kuma(Uptime Kuma)。**

简单好用的自托管监控:网站和服务器一出问题,马上就知道,而且不吵人。

[English](README.md) · **中文** · [हिन्दी](README.hi.md) · [Español](README.es.md) · [العربية](README.ar.md) · [Français](README.fr.md) · [বাংলা](README.bn.md) · [Português](README.pt.md) · [Русский](README.ru.md) · [Bahasa Indonesia](README.id.md) · [اردو](README.ur.md) · [Deutsch](README.de.md) · [日本語](README.ja.md) · [Tiếng Việt](README.vi.md) · [한국어](README.ko.md)

![Unagikuma 网页界面快速导览](docs/images/tour.gif)

## 为什么选择 Unagikuma

- **大约 30 秒就能用上。** 下载一个文件,运行它,打开浏览器即可。
- **只有一个文件,不用装别的。** 支持 Linux、macOS、Windows 和 FreeBSD。
- **该查的都能查。** 网站、服务器、端口、Ping、DNS、SSL 证书,以及域名到期时间。
- **告警发到你常用的地方。** 邮件、Telegram、Slack、Discord、飞书、钉钉、企业微信、Webhook 等。
- **一个问题,不会告警刷屏。** 如果是你自己的网络断了,Unagikuma 会察觉到,不会为每个监控项都给你发一遍告警。
- **状态页**,让你的用户随时了解服务状态。
- **随你一起成长。** 需要时可以从多个地点同时检测(预览功能)。
- **轻松迁移。** 可以把 Uptime Kuma 或 Nagios 里的监控搬过来(预览功能)。
- **说你的语言。** 界面支持 15 种语言。

## 快速上手

**1.** 从 [Releases](https://github.com/githubflyideas/unagikuma/releases/latest) 下载适合你系统的文件并解压。

**2.** 运行:

```sh
./unagikuma
```

Windows 上运行 `unagikuma.exe`。

**3.** 在浏览器打开 http://localhost:8077,创建管理员账号。

**4.** 点击 **添加监控项**,填入网址,就完成了。

> 从另一台电脑打开?访问 `http://<服务器地址>:8077`,并输入程序首次启动时显示的 **初始化令牌**。

## 想用 Docker?

```sh
docker run -d --name unagikuma -p 8077:8077 -v unagikuma-data:/data ghcr.io/githubflyideas/unagikuma:latest
```

然后打开 http://localhost:8077。**初始化令牌** 可以用 `docker logs unagikuma` 查看。

## 了解更多

- [用户手册](docs/manual.zh-CN.md) — 其他所有内容,一步一步讲清楚
- [更新日志](CHANGELOG.md)

## 许可证

Apache License 2.0,详见 [LICENSE](LICENSE)。
