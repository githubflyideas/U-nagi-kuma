# Unagikuma

<p align="center"><img src="docs/images/logo.png" alt="Unagikuma" width="240"></p>

单文件、自托管的监控系统：Nagios 的信息密度，Uptime Kuma 的易用性。

[English](README.md) · [用户手册](docs/manual.zh-CN.md) · [更新日志](CHANGELOG.md)

![网页界面一览：显示网络故障位置的总览、监控列表、监控项抽屉、命令面板、通知策略、monitors.bind 和公开状态页](docs/images/tour.gif)

## 为什么选 Unagikuma

- **一个文件，零依赖。** 静态编译的 Go 程序，内置网页界面和 SQLite。拷到机器上运行，打开浏览器即可。
- **可读、可追溯的配置。** 所有配置都在 `monitors.bind` 里，这是一个风格类似 DNS 区域文件的纯文本文件。可以在网页里改，也可以用编辑器改；每个生效过的版本都会保留，随时可以恢复。语法有[正式规范](docs/spec/monitors-bind.md)，1.x 版本内保持兼容。
- **懂协议的检查。** HTTP(S)（关键字、JSON、证书到期）、TCP（含 TLS）、ICMP ping，Redis、PostgreSQL、MySQL、gRPC 健康检查和 Docker 容器，DNS，域名注册到期，以及会说真实协议的 UDP 检查（`10.0.0.1:ntp`、`:snmp`、`:tftp`、`:syslog` 等）。Nagios 插件可以原样作为脚本检查运行。每种检查都标明它能证明到什么程度（L1 只确认已发出，L2 端口有响应，L3 服务正确回复）。
- **默认就安静。** 软/硬状态、依赖关系（`parent=`）、维护窗口、确认、静默和告警风暴汇总。如果是本机自己的网络、DNS 或时钟出了问题，Unagikuma 会识别出来并暂缓告警，而不是对每个目标都发一遍；每个监控项都会用文字说明为什么发了或没发通知。
- **够用也能扩展的告警。** 飞书、钉钉、企业微信、Telegram、Slack、Discord、邮件、带十几种预设的 Webhook，以及经阿里云、腾讯云、Twilio 发送的短信和语音电话。备用通道链、时间段、联系组、未确认故障的升级、事件处理脚本和完整的投递记录。
- **告警发给对的人。** 联系人目录让通知通道保持准确：有人离职就不再收到告警（登录也一并停用），退信的地址自动停发，未登记的地址会被标出来。值班表只通知当班的人，支持代班，并统计 MTTA/MTTR。计划内的变更可以提前通知、自动提醒、期间进入维护，结束时发送结果。
- **公开状态页**：公告、即将进行的维护、邮件订阅、Atom 订阅源和独立域名；兼容 Uptime Kuma 推送 URL 的**推送监控**；**徽章**。
- **需要时再加探测点。** 在其他机房或分支机构用同一个程序运行 agent，检查结果由多个节点投票决定；只有某一处看到的故障只会作为区域性提示。备用节点一键接管主节点角色。
- **迁移方便。** 可导入 Uptime Kuma 数据库和 Nagios 对象配置；在命令面板（Ctrl+K）里粘贴地址即可添加监控。
- **所有阈值都可调**，界面支持 **15 种语言**（English、中文、हिन्दी、Español、العربية、Français、বাংলা、Português、Русский、Bahasa Indonesia、اردو、Deutsch、日本語、Tiếng Việt、한국어），通知也一样。

探测节点与主节点切换、导入工具、脚本检查、段落、审计日志以及中英文以外的界面语言标记为实验性：可用且有测试，但后续可能调整。

## 快速开始

从 [Releases](../../releases) 下载对应平台的压缩包，解压后运行：

```sh
./unagikuma            # 监听 :8077，数据保存在 ./data
```

打开 `http://<主机>:8077/` 创建管理员账号。如果不是在服务器本机打开界面，需要填写首次启动时打印在日志里的一次性初始化令牌。

然后在界面里添加监控，或用**快速添加监控项**粘贴一批目标，也可以直接编辑 `data/monitors.bind`。

### 在容器中运行

```sh
docker run -d --name unagikuma -p 8077:8077 -v unagikuma-data:/data ghcr.io/githubflyideas/unagikuma:latest
```

镜像中只有这个静态程序，数据保存在 `/data`。ping 检查需要宿主允许非特权 ping（`net.ipv4.ping_group_range`，Docker 容器内默认允许），否则加上 `--cap-add NET_RAW --user 0`。

### 作为系统服务运行（Linux，systemd）

```ini
# /etc/systemd/system/unagikuma.service
[Unit]
Description=unagikuma monitoring
After=network-online.target
Wants=network-online.target

[Service]
ExecStart=/usr/local/bin/unagikuma server --data /var/lib/unagikuma
Restart=on-failure
DynamicUser=yes
StateDirectory=unagikuma
AmbientCapabilities=CAP_NET_RAW

[Install]
WantedBy=multi-user.target
```

只有在系统禁用了非特权 ping（`net.ipv4.ping_group_range`）时才需要 `CAP_NET_RAW`。两者都没有时，ping 检查会报告“本机权限不足”，而不会把目标判为故障。

## 接下来

其余内容见[用户手册](docs/manual.zh-CN.md)：

- [编写 monitors.bind](docs/manual.zh-CN.md#monitorsbind) 和[脚本检查](docs/manual.zh-CN.md#脚本检查)；
- [告警](docs/manual.zh-CN.md#告警)：通道、策略、时间段、联系组、升级、短信和语音、事件处理脚本；
- [状态页](docs/manual.zh-CN.md#状态页)、[探测节点与主节点切换](docs/manual.zh-CN.md#探测节点)、[用户与段落](docs/manual.zh-CN.md#用户)；
- [从 Uptime Kuma 或 Nagios 迁移](docs/manual.zh-CN.md#从-uptime-kuma-或-nagios-迁移)、[备份](docs/manual.zh-CN.md#数据与备份)、[命令行](docs/manual.zh-CN.md#命令行)和 [API](docs/manual.zh-CN.md#api)；
- [故障排查](docs/manual.zh-CN.md#故障排查)。

## 从源码构建

需要 Go 1.27.3 或更高版本，不需要 C 编译器：

```sh
make build     # bin/unagikuma
make test
make dist      # 在 dist/ 中生成所有平台的发布包
```

端到端测试把编译好的程序作为独立进程运行：`go test -tags e2e ./e2e/`（浏览器测试需要 Node.js 和 Playwright）。

## 许可证

Apache License 2.0，见 [LICENSE](LICENSE)。
