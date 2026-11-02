# Unagikuma

<p align="center"><img src="docs/images/logo.png" alt="Unagikuma" width="240"></p>

**又一个可用性监控工具:u(ptime) + nagi(os) + kuma(Uptime Kuma)。**

简单好用的自托管监控:网站和服务器一出问题,马上就知道,而且不吵人。

[English](README.md) · **中文** · [हिन्दी](README.hi.md) · [Español](README.es.md) · [العربية](README.ar.md) · [Français](README.fr.md) · [বাংলা](README.bn.md) · [Português](README.pt.md) · [Русский](README.ru.md) · [Bahasa Indonesia](README.id.md) · [اردو](README.ur.md) · [Deutsch](README.de.md) · [日本語](README.ja.md) · [Tiếng Việt](README.vi.md) · [한국어](README.ko.md)

## 当前界面

> 使用解压后的 v1.5.7-rc.1 Windows 候选包运行真实本地后端，使用隔离、可丢弃的演示数据拍摄。通知仅写入本地日志；图中并非生产事故，也不是最终发布版截图。

![午夜蓝总览](docs/images/current/dashboard-midnight.png)

<details>
<summary>主题、告警渠道、值班轮换、日报与移动端</summary>

**暖纸色总览**

![暖纸色总览](docs/images/current/dashboard-paper.png)

**告警渠道**

![告警渠道](docs/images/current/alerts-midnight.png)

**值班日历**

![值班日历](docs/images/current/oncall-paper.png)

**日报——当前快照，并非过去 24 小时的历史统计**

![日报——当前快照，并非过去 24 小时的历史统计](docs/images/current/daily-report-midnight.png)

**移动端日报**

<img src="docs/images/current/mobile-paper.png" alt="移动端日报" width="390">

</details>

## 为什么选择 Unagikuma

- **大约 30 秒就能用上。** 下载一个文件,运行它,打开浏览器即可。
- **只有一个文件,不用装别的。** 支持 Linux、macOS、Windows 和 FreeBSD。
- **该查的都能查。** 网站、服务器、端口、Ping、DNS、SSL 证书,以及域名到期时间。
- **告警发到你常用的地方。** 邮件、Telegram、Slack、Discord、飞书、钉钉、企业微信、Webhook 等。
- **一个问题,不会告警刷屏。** 如果是你自己的网络断了,Unagikuma 会察觉到,不会为每个监控项都给你发一遍告警。
- **状态页**,让你的用户随时了解服务状态。
- **随你一起成长。** 需要时可以从多个地点同时检测(预览功能)。
- **轻松迁移。** 可以把 Uptime Kuma 或 Nagios 里的监控搬过来。
- **说你的语言。** 界面支持 15 种语言。
- **适合日常运维的控制台。** 提供深色、浅色、午夜蓝和暖纸色主题，并支持小屏幕响应式导航。
- **告警和值班集中管理。** 搜索通知渠道、查看策略引用，通过日历管理值班轮换、临时代班和换班。
- **语义明确的日报。** 预览当前服务与主机状态、查看待处理问题，并将报告加入发送队列。它是当前快照，不是过去 24 小时的可用率或历史统计。

> **预览功能边界：** 多地点监测仍为预览功能。Zone 同步、邀请和投票仍在集成中；本 README 不承诺完整 HA、自动共识／故障切换或脚本上传。上方图片展示解压后的 v1.5.7-rc.1 Windows 候选包，并不代表最终发布或多平台验收完成。

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

## 从 Uptime Kuma 迁移

先停止 Uptime Kuma，然后预览一下（不会做任何更改）：

```sh
./unagikuma import kuma --dry-run /path/to/uptime-kuma/data/kuma.db
```

确认无误后正式导入：

```sh
./unagikuma import kuma /path/to/uptime-kuma/data/kuma.db
```

报告会列出已导入的、需要查看的和无法导入的内容，并说明该怎么做。报告也会保存在 `data/import-reports/` 中。再次运行只会添加新的内容。

## 从 Nagios 迁移

指向 `nagios.cfg`，先预览一下（不会做任何更改）：

```sh
./unagikuma import nagios --dry-run /etc/nagios4/nagios.cfg
```

确认无误后正式导入：

```sh
./unagikuma import nagios /etc/nagios4/nagios.cfg
```

报告会列出已导入的、需要查看的和无法导入的内容，并说明该怎么做。报告也会保存在 `data/import-reports/` 中。再次运行只会添加新的内容。

## 忘记密码？

在运行 Unagikuma 的机器上执行：

```sh
./unagikuma reset-password admin
```

输入两次新密码（不会显示）。加上 `--generate` 可以改为自动生成一个随机密码。使用 Docker 时：

```sh
docker exec -it unagikuma /unagikuma reset-password admin
```

## 了解更多

- [用户手册](docs/manual.zh-CN.md) — 其他所有内容,一步一步讲清楚
- [更新日志](CHANGELOG.md)

## 许可证

Apache License 2.0,详见 [LICENSE](LICENSE)。
