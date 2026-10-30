# Unagikuma 用户手册

安装、配置和使用 Unagikuma。简要介绍见 [README](../README.zh.md)。
**目录**

- [快速开始](#快速开始)
- [monitors.bind](#monitorsbind)
- [告警](#告警)
- [状态页](#状态页)
- [探测节点](#探测节点)
- [用户](#用户)
- [注册与配额](#注册与配额)
- [从 Uptime Kuma 迁移](#从-uptime-kuma-迁移)
- [命令行](#命令行)
- [数据与备份](#数据与备份)
- [API](#api)
- [故障排查](#故障排查)

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

只有在系统禁用了非特权 ping（`net.ipv4.ping_group_range`）时才需要 `CAP_NET_RAW`。两者都没有时（例如默认安装的 Debian 或 Ubuntu），ping 检查会改为连接 TCP 443、80、22 端口：对方接受或拒绝连接都说明它在线，结果会注明“通过 TCP 检查”。三个端口都没有回应时，检查会报告“本机权限不足”，而不会把目标判为故障。`fallback=off` 可以关闭 TCP 检查。

## monitors.bind

```text
; 序列号  间隔  重试  超时  尝试次数
@ IN SOA unagikuma. admin.example.com. ( 2026102301 1m 15s 5s 3 )

$SET notify.repeat=30m

; 通知通道，以及使用它们的策略
ops      IN CHANNEL feishu url=${secret:ops_webhook}
mail     IN CHANNEL email host=smtp.example.com port=587 tls=starttls username=alerts@example.com password=${secret:smtp} from=alerts@example.com to=oncall@example.com
default  IN POLICY chain=ops,mail mode=fallback max=3 repeat=30m

; 监控：名称 [间隔] IN 类型 目标 key=value...
www      IN HTTP https://www.example.com/ title="官网" keyword="Welcome" cert-days=14 group=public
api  30s IN HTTP https://api.example.com/health json-path=$.status json-expect=ok
db       IN TCP  10.0.0.5:5432 title="Postgres"
gw       IN PING 192.168.1.1
time     IN UDP  10.0.0.1:ntp authorized=yes
switch   IN UDP  10.0.0.2:snmp community=${secret:snmp_ro}
backup   IN PUSH period=24h grace=2h
company  IN DOMAIN example.com days=45 critical-days=10

; 公开状态页，地址为 /status/main
main     IN STATUSPAGE "服务状态" select="group:public"
```

- 手工编辑后请**增大序列号**，Unagikuma 会校验并自动生效。在网页里保存监控、通道或策略时序列号会自动增加。**配置 › monitors.bind** 只读显示该文件，可一键复制；需要直接修改请在服务器上编辑。有错误的文件不会生效：上一个正确的版本继续运行，界面会指出错在哪里。
- 凭据不必写进文件：在 **设置 › 密钥** 中保存，然后用 `${secret:名称}` 引用。用 `--allow-env NAME` 启动时，也可以用 `${env:NAME}` 引用环境变量。
- 监控项按检查的地址归入主机；`device=core-sw1` 可以把不同地址的监控项归到同一个名字下。有 PING 监控的主机，其 UP / DOWN 由这个 ping 决定。
- `parent=gw`（或 `parent=sw1,gw`）声明依赖关系：父项故障期间，失败的子项显示为“不可达”，不发通知；父项恢复后子项仍然故障，再照常通知。
- 维护窗口期间，选中的监控项不发通知，这段时间也不计入可用率；窗口结束时仍然故障的监控项会在那时通知：

  ```text
  sunday  IN MAINT "机房维护" select="tag:site:bj" every=sun start=02:00 duration=2h notify=yes
  upgrade IN MAINT "核心交换机升级" select="cidr:10.0.0.0/24" at="2026-11-01 22:00" duration=3h
  backups IN MAINT "夜间备份" select="group:db" cron="30 2 * * 1-5" duration=40m
  ```

  `select=` 只能使用由配置决定的字段：`name`、`title`、`type`、`svc`、`port`、`host`、`cidr`、`tag`、`group`、`policy`、`node`、`area`，以及自由文本；多个条件同时满足，逗号表示“任一”，`-` 表示取反，`/…/` 表示正则表达式。
- HTTPS 检查以及带 `tls=yes` 的 TCP 检查会同时监控证书：剩余不到 `probe.cert-warn-days`（21）天为降级，不到 `probe.cert-critical-days`（7）天为故障；已过期的证书一律判为故障，设置了 `insecure=yes` 也一样。监控项上的 `cert-days=`、`cert-critical-days=` 可单独设置阈值（写 0 表示关闭）。
- `DOMAIN` 监控域名注册的到期时间，通过 RDAP（没有 RDAP 时用 WHOIS）向注册局查询，默认每 6 小时检查一次，最频繁每小时一次。剩余不到 `days=`（30）天为降级，不到 `critical-days=`（7）天为故障；注册局都无法查询时为 UNKNOWN，不会判为故障。**监控 › 到期时间** 按到期先后列出所有域名和 TLS 证书。
- `unagikuma keywords` 列出所有 UDP 关键字及其检查深度。
- 每天 10:00（`notify.daily-report-at`）发送状态日报：列出宕机与正常的主机，以及所有异常服务；在 **通告和状态 › 状态日报** 中可预览和立即发送。

### 脚本检查

> 实验性功能。

`EXEC` 按 Nagios 插件约定运行检查脚本：退出码 0 为正常，1 为 WARNING（显示为降级），2 为 CRITICAL，3 为 UNKNOWN；输出第一行是状态文字，`|` 之后的性能数据记为指标。已有插件和公司自写脚本无需修改即可使用：

```text
disk-db1  IN EXEC check_disk args="-w 20% -c 10% -p /data" device=db1
load-db1  IN EXEC check_load args="-w 5,4,3 -c 10,8,6" device=db1
nrpe-web  IN EXEC "check_nrpe -H $HOSTADDRESS$ -c check_users" address=10.0.0.21
```

- 脚本只从执行检查的那个节点的 check-scripts 目录中查找：服务器为 `<data>/check-scripts`，探测节点为 `/etc/unagikuma/check-scripts`（Windows：`%ProgramData%\unagikuma\check-scripts`），也可用 `--script-dir dir1,dir2` 指定。首次启动时自动创建该目录，并放入说明文件和示例脚本（`check_example`）。配置里只能写该目录中的文件名，不能写路径，也不经过 shell。
- 每台节点运行自己的那份脚本，需要自行复制到每台执行该检查的节点（scp、Ansible 等），Unagikuma 不负责上传。新建的脚本检查默认**暂停**：启用时会检查每台相关节点是否都有该脚本，缺失则拒绝并列出缺少的节点。**设置 › 检查脚本** 显示每个检查在每台节点上的脚本状态（就绪、缺失、不可执行、版本不同）；只要有已启用的脚本检查在某台节点无法运行，左侧栏就会显示红色计数。
- 脚本只拿到最小环境变量（`PATH`、`UNAGIKUMA_MONITOR`、`UNAGIKUMA_ADDRESS`、`UNAGIKUMA_TIMEOUT`），不继承服务器的环境。参数中的 `$HOSTADDRESS$`、`$SERVICEDESC$`、`$TIMEOUT$` 会被替换。超时后脚本及其启动的所有进程都会被终止，检查记为 CRITICAL。
- 只有管理员可以添加 EXEC 检查或修改其运行内容。`probe.exec-concurrency` 限制每个节点同时运行的插件数。

## 告警

通知经过**通道**（消息发到哪里）和**策略**（用哪些通道、发多少次、之后怎么办）。两者都是 `monitors.bind` 中的记录，在 **告警** 页面也有表单。

```text
ops-im    IN CHANNEL feishu url=${secret:feishu_ops}
oncall    IN CHANNEL aliyun-sms access-key-id=LTAI… access-key-secret=${secret:ali} sign-name=运维 template-code=SMS_123 phones=13800000000
manager   IN CHANNEL email host=smtp.example.com from=alerts@example.com to=boss@example.com
office    IN PERIOD "mon-fri 09:00-18:00; sat 10:00-12:00"
team      IN CHANNEL group members=ops-im,oncall
night-sms IN CHANNEL aliyun-sms … period=!office
default   IN POLICY chain=team,night-sms mode=all repeat=30m max=3 escalate=30m:manager,2h:manager+oncall
```

- **策略。** `chain=` 列出通道；`mode=fallback`（默认）依次尝试直到有一个成功，`mode=all` 全部发送。`repeat=`、`max=`、`delay=`、`reopen-window=`、`ack-expire=`、`recovery=no`、`notify-degraded=yes` 决定发送什么。监控项用 `policy=名称` 指定策略，否则使用 `default`。
- **时间段。** `PERIOD` 记录是按周的规则（`mon-fri 09:00-18:00`，可跨午夜如 `22:00-07:00`，`24:00` 表示当天结束）。通道上写 `period=office` 表示只在时间段内使用，`period=!office` 表示只在时间段外使用。被跳过的通道不算失败：备用链会接着尝试下一个。
- **联系组。** `group` 通道同时通知所有 `members=` 成员，任一成员成功即算成功。成员各自的时间段仍然有效。
- **值班轮换。** 在 group 通道上写 `rotate=daily|weekly|Nd|Nh`，它就成为值班表：只通知当班的成员，联系不上时才按顺序通知其他人。`handoff="mon 09:00"` 设置交接时间（只有按整周轮换时才能指定星期几），`start=` 设置第一个成员开始值班的日期。每人一个通道（他的短信或 IM），按排班顺序列出：

  ```text
  oncall IN CHANNEL group members=li-sms,wang-sms,zhao-sms rotate=weekly handoff="mon 09:00" start=2026-11-02
  ```

  **值班** 页面显示当前谁在值班和之后的班次，可以新建值班表、安排代班：有人临时顶班，告警先发给他，原有排班顺序不变。页面还统计最近 `oncall.stats-window`（30 天）的 MTTA 和 MTTR，每起故障算在它发生时的值班人名下。
- **升级。** 策略上的 `escalate=时长:通道[+通道],…` 在故障持续未确认时追加通知：30 分钟后通知主管，2 小时后再加上值班电话。确认后即停止。
- **汇总会说明是哪类问题。** 短时间内积压很多通知时（`notify.coalesce-queue`、`notify.coalesce-wait`），会合并成汇总，汇总区分业务和网络：
  - 各自独立出错的业务排在最前面，写明"请尽快处理"，给业务负责人；
  - 同一 IP 网段内 `notify.group-min`（3）个以上监控同时出现网络层故障，判定为该网段、交换机或上联的问题；多个监控同时无法解析域名，判定为 DNS 问题——写明"请联系网管"。

  在策略上写 `network=netadmin`，网络类的部分会单独发给这些通道（网管），业务部分仍走原来的链。
- **监控主机自己断网时**，它无法通知任何人：断网期间不发送，期间积压的消息事后也不补发。在识别出断网之前不久（`localnet.backdate`）开始、且还没通知到任何人的故障会被撤回。网络恢复后会立即重新检测所有监控，只发一条消息，说明断了多久、现在还有哪些异常；短于 `localnet.notify-after`（5 分钟）的断网只记录、不通知。只有真正送达过故障通知的问题，才会再发恢复通知。若要在断网*期间*就收到通知，请配置外部心跳（`--heartbeat-url`）或增加第二个节点。
- **短信和语音电话。** 通道类型 `aliyun-sms`、`aliyun-voice`、`tencent-sms`、`tencent-voice`、`twilio-sms`、`twilio-voice`。模板参数默认是监控名和状态，可用 `params="{{.Monitor}}|{{.StatusText}}"` 修改。在网页中保存时密钥会存为加密密钥。
- **事件处理脚本。** 监控项上的 `handler="restart-web $SERVICESTATE$ $SERVICESTATETYPE$"` 会在每次状态变化时运行 check-scripts 目录中的脚本（Nagios 风格），环境变量含 `UNAGIKUMA_MONITOR`、`UNAGIKUMA_STATE`、`UNAGIKUMA_STATE_TYPE`、`UNAGIKUMA_ATTEMPT`、`UNAGIKUMA_ADDRESS`、`UNAGIKUMA_REASON`。脚本在主节点上运行，每个监控项同时只运行一个，最长 `engine.handler-timeout`；结果写入事件日志。只有管理员可以设置。
- **联系人。** **告警 › 联系人** 列出所有可能接收告警的人及其邮箱和手机号，通知通道会对照它检查：
  - 标记为**已离职**的人，不再向其地址发送，关联的登录账号和 API 令牌也一并失效；
  - 被判定为不存在（邮箱不存在、号码无效）达到 `contacts.bounce-threshold`（3）次的地址不再发送；一个收件人有问题不会影响发给其他人；
  - 不在目录中的地址仍会发送，但该通道、使用它的策略和**告警**菜单会亮红点，并说明要检查什么。

  在通道里写 `to=@li` 或 `phones=@li,@oncall` 即可引用某人的地址，不用逐个填写。保存通道时如果有新地址，会提示把它们登记到目录；**一键导入**会把通道里已在用的地址全部加入。`unagikuma check` 在能读取数据目录时也会报告这些问题。
- **为什么（没）收到通知？** 监控项抽屉的 **通知情况** 一栏用文字说明原因：维护中、父项故障、本机网络故障、只有部分节点失败、仍处于软状态、已确认、已静默、达到通知上限、策略没有通道。**告警 › 通知策略 › 预测** 可以模拟一次故障。

### 变更通知

**通告和状态 › 变更通知** 用来把计划内的工作（交换机升级、数据库迁移等）告诉相关的人：

- 发布后立即通过通知策略（或直接发到选定的通道）发出，开始前按 `changes.reminders`（`24h,1h`，只发晚于发布时间的提醒）提醒，开始时通知，超过结束时间仍未关闭时再提醒一次。
- 按筛选条件（`group:core`、`tag:site:bj` 等）选中的监控在时间窗内处于维护状态：故障不通知，也不计入可用率，不需要写 `MAINT` 记录。
- 可以同时在状态页发布维护公告，结束时随结果一起关闭。
- 结束时给出结果：**完成**或**已回滚**并附备注；开始前也可以**取消**。调整时间会把新时间通知所有人，并重新计算提醒。
- 历史记录可以搜索并导出 CSV。通知内容是可修改的模板，内置中英文两套。

### 数字是怎么算的

- **未处理问题**：没有人确认、也没有静默的未结束故障，包括区域性和低优先级故障。侧栏、总览、问题页和 `/metrics`（`unagikuma_unhandled_problems`）都按这同一个定义计数。无法判定（UNKNOWN）的监控没有故障单，按它的状态计数。
- **可用率**：正常时长 ÷（正常时长 + 故障时长）。DEGRADED（变慢）算正常，变慢的占比单独给出。维护窗口和没有检查的空档两边都不算。确认故障期间（SOFT）的时长先挂起：故障被确认就算故障，先恢复就算正常，所以从一开始就失败的监控显示 0%。
- **覆盖率**：监控在某个时间窗内被观测的时长达到 `uptime.min-coverage`（10%）后，才显示这个时间窗的可用率；在此之前界面显示“—”和已观测的比例，刚加一小时的监控不会给出 30 天的数字。
- **修改正在故障的检查**（超时、期望关键字，甚至目标）时，未结束的故障保留，并立即重新检查。仍然失败就不会再通知一次；检查通过则照常发恢复通知。没有未结束故障的检查从 PENDING 重新开始。
- **确认记录**按历史保存：再次确认会追加一条带备注的记录，不会覆盖第一次的；第一次确认的时间也会保留。

## 状态页

```text
public IN STATUSPAGE "Example 服务状态" select="group:public" subscribe=mailer domain=status.example.com
```

- 状态页在 `/status/名称` 显示所选监控项及其 `windows=`（24h、7d、30d、90d）可用率，从不显示目标地址和故障原因。
- **公告** 在 **通告和状态 › 状态页** 中发布：标题、内容和级别（通知、轻微故障、重大故障、维护）。未解决的显示在顶部，已解决的在“历史公告”中保留 `statuspage.resolved-shown`（7 天）。状态页还会列出其监控项未来一周内的维护窗口。
- **订阅。** 设置 `subscribe=通道`（邮件通道）后，访客可以用邮件订阅：先点击确认链接，每封邮件都带退订链接。发布或更新公告时勾选“邮件通知订阅者”即可。订阅请求按来源地址限速（`statuspage.subscribe-rate`），未确认的订阅有上限（`statuspage.max-pending`）。每个状态页还有 Atom 订阅源：`/status/名称/feed.atom`。
- **独立域名。** 设置 `domain=status.example.com` 后，访问该域名时在 `/`（以及 `/feed.atom`）提供状态页，控制台和 API 在该域名下不可访问。把域名解析到服务器，并在前面加 TLS 反向代理，或为该域名配置证书。
- 徽章：公开状态页上的监控项可使用 `/badge/监控项/status.svg`、`uptime.svg`、`latency.svg`。

## 探测节点

> 实验性功能：在 v0.x 版本之间，节点协议和主节点切换仍可能调整。

单机就是完整的部署。想从更多地点检查同样的目标时，声明节点并在那里运行 agent：

```sh
unagikuma node add bj --title "北京机房" --server https://master.example.com:8077
```

这条命令会在 `monitors.bind` 中加入 `bj IN NODE "北京机房"`，为 `bj` 生成令牌，并打印启动 agent 的命令（前台运行、systemd 服务或 PowerShell）。网页界面的**设置 › 节点**也能完成同样的操作。在节点上运行：

```sh
unagikuma agent --server https://master.example.com:8077 --node bj \
  --token-file /etc/unagikuma/agent.token --ca-fingerprint sha256:…
```

- Agent 主动连接 master 的端口（默认 8077），所以可以放在 NAT 后面。连接使用 TLS，agent 按命令中的指纹固定 master 自己的 CA。如果中间需要代理，请使用 TCP 代理而不是 HTTP 反向代理。
- 默认每个监控都由所有节点检查。只在某个地点存在的目标放进段落里：

  ```text
  bj  IN NODE "北京机房" region=north
  $SCOPE region:north policy=bj-oncall   ; 只由 north 区域的节点检查，告警发给 bj-oncall
  bj-core IN PING 10.10.0.1
  $SCOPE *                               ; 回到所有节点
  ```

  其他节点根本收不到这些监控项。单个监控上的 `nodes=bj,sh`（或 `nodes=region:north`）效果相同，并且优先于段落。每一轮由 `quorum=`（默认多数）决定；离线的节点会降低法定票数，而不会让判断卡住。
- 只有部分节点看到的故障显示为“局部”；持续 `partial-escalate`（30 分钟）后变成一条低优先级的区域性告警。长时间断开的节点按它的 `policy=` 报告。
- 也可以在网页上加入，不用复制命令：在 master 的 **设置 › 节点 › 加入码** 生成一次性代码（有效期 `cluster.join-code-ttl`，默认 1 小时），在另一台服务器的 **设置 › 节点 › 加入集群** 中粘贴。之后这台服务器作为 master 的节点运行。它原有的监控项会被删除；也可以选择导入到 master，放在以它命名的段落里并处于暂停状态，在 master 上检查后再恢复。
- 在 agent 上加 `--emergency-webhook URL`（或 `feishu:URL`、`dingtalk:URL` 等）后，master 持续 `cluster.emergency-after`（3 分钟）联系不上时，agent 会直接发一条通知，恢复时再发一条，因为 master 无法报告自己的故障。
- `unagikuma node list`、`node token 名称`（生成新令牌，旧令牌立即失效）和 `node revoke 名称` 用来管理令牌；被吊销的 agent 会在一个 ping 周期内断开。

### 主节点切换

没有选举：由谁当主节点由你决定，切换只需点一下。

1. 让准备接任的服务器以节点身份加入，并在 节点 › 编辑 中填写**备用地址**（`bj IN NODE standby=https://10.0.0.2:8077`）。此后它会保存一份主节点状态副本，包括数据库、`monitors.bind`、密钥和集群 CA。副本每隔 `cluster.standby-sync`（5 分钟）以及每次配置变更后刷新；所有节点都会知道这个备用地址。
2. **计划内切换**（维护、换机器）：在 节点 页对备用节点点击 **设为主节点**。它会接收当前状态并以主节点身份重启；原主节点停止检查并拒绝修改；各节点自动切换过去。用户、令牌、密钥和 CA 都会一起迁移，不需要重新加入。
3. **主节点丢失**：在备用节点的 设置 › 节点 中点击 **接任主节点**（或停止服务后执行 `unagikuma promote`）。原主节点不可达后，各节点会切换到它。

每次切换**任期号**加一。节点只跟随见过的最高任期；被替换的主节点重新上线后，会收到新主节点用集群 CA 签名的通知，或由第一个连上它的节点告知，随即停止检查，因此不会出现两个主节点同时告警。主节点丢失后的接任总是由人来操作：没有多数派机制的自动接管，会在网络分区时把集群一分为二。

### 作为 Windows 服务运行

以管理员身份打开 PowerShell，进入 `unagikuma.exe` 所在目录：

```powershell
.\unagikuma.exe service install -- server --data C:\ProgramData\unagikuma --log-file C:\ProgramData\unagikuma\unagikuma.log
.\unagikuma.exe service uninstall
```

服务会开机自启，异常退出后自动重启。Agent 的安装方式相同（节点的安装命令里有现成的命令）。

## 用户

**设置 › 用户** 管理账号：管理员拥有全部权限；操作员可以确认告警、编辑监控项和维护窗口；只读用户只能查看。操作员可以限定在部分 `$SCOPE` 段落（如 `region:north`、`bj`），只能修改这些段落里的内容，不能修改共享配置（通道、策略、配置文件原文）。

**设置 › 审计** 列出所有变更：账号、角色与段落权限（变更前后）、令牌、密钥（仅名称）、节点和配置，以及操作人或令牌、所在段落、来源地址；直接修改磁盘上配置文件的变更和失败的登录也会记录。管理员可通过 `GET /api/v1/audit?actor=&action=&area=` 查询；记录保留 `retention.audit`（两年）。

查看审计日志需要已注册的安装，见[注册与配额](#注册与配额)。

## 注册与配额

没有许可证时，除查看审计日志外所有功能都可用，也没有任何数量限制。在 **设置 › 注册** 中安装收到的 `license.json`；试用许可证按该页面显示的安装编号签发。

许可证可以限制节点数（含主节点）和账号数。在上限内一切照常；超出后，新增节点、新机器加入、新建账号或导入账号会被拒绝并说明原因。已经在用的不会被停用（例如用较小的许可证替换了较大的）；注册页面会显示各项的使用量和上限。

## 从 Uptime Kuma 或 Nagios 迁移

导入会加入 Unagikuma 能运行的内容，其余的逐条列出原因和替代做法。已有的监控项不会被修改或删除，所以重复运行是安全的。请先用 `--dry-run` 预览。

**从 Uptime Kuma 迁移**（1.23 和 2.x）。先停止 Uptime Kuma，然后：

```sh
unagikuma import kuma --dry-run /path/to/uptime-kuma/data/kuma.db
unagikuma import kuma /path/to/uptime-kuma/data/kuma.db
```

也可以导入 Uptime Kuma 1.x 导出的 JSON 备份，但其中没有账号、维护窗口和状态页。会迁移过来的有：HTTP、关键字、JSON 查询、TCP 端口、ping、DNS、推送（推送地址继续有效，只需换服务器地址）、Docker、gRPC、Redis、PostgreSQL 和 MySQL 监控，连同间隔、重试、分组（嵌套分组写成 `父组 / 子组`）、标签和暂停状态；通知（飞书、钉钉、企业微信、Telegram、Slack、Discord、Webhook、SMTP、ntfy、Gotify、Pushover、Teams、PagerDuty、Matrix、Mattermost、Google Chat、Bark、Server酱、PushPlus、Opsgenie）变成通道，凭据存为密钥；每周、每月、cron 和一次性的维护窗口；状态页；以及账号和原密码（新安装时第一个账号成为管理员）。Unagikuma 无法运行的监控（MQTT、MongoDB、SQL Server、RADIUS、游戏服务器、反向模式等）会列出并给出替代建议。

**从 Nagios 迁移**（也适用于 Icinga 1、Naemon）。指向 `nagios.cfg`，它引用的文件（`cfg_file=`、`cfg_dir=`、`resource_file=`）会像 Nagios 一样读取：

```sh
unagikuma import nagios --dry-run /etc/nagios4/nagios.cfg
unagikuma import nagios /etc/nagios4/nagios.cfg
```

也可以指定目录或单个 `.cfg` 文件。模板会被展开；主机变成 ping 检查，其服务以 `parent=` 依赖主机；`check_http`、`check_tcp`、`check_ssh`、`check_dns`、`check_dig`、`check_ntp`、`check_snmp`（运行时间）、`check_mysql`、`check_pgsql` 等插件变成内置检查；其他插件变成脚本检查，先处于暂停状态，把插件复制到 check-scripts 目录后再恢复。`resource.cfg` 中的 `$USERn$` 值存为密钥，只接收被动结果的服务变成推送监控，有邮箱的联系人变成邮件通道，联系人组变成组通道，时间段变成 `PERIOD` 记录。

**迁移报告。** 两种导入都会打印摘要（已导入 / 需要查看 / 未导入）、已导入内容的表格，以及每个需要查看或未导入项目的原因和处理方法。同样的报告以文本和 Markdown 格式保存在 `<data>/import-reports/`。新记录写在 `monitors.bind` 末尾的一个块中，按来源分节并加注释；`title=` 保留原名。网页界面 **配置 › 导入** 的效果相同。选项：`--area 段落` 把监控放入某个段落，`--role` 设置导入账号的角色，`--report 目录` 指定报告保存位置。退出码：0 完成（或没有新内容），1 失败（未做任何更改），2 用法错误。

## 忘记密码？

在运行 Unagikuma 的机器上执行（服务可以不停）：

```sh
unagikuma reset-password --data /path/to/data admin
```

输入两次新密码（不会显示），或用 `--generate` 生成一个随机密码并只显示一次。该用户的所有登录会话都会退出。如果用户名写错，命令会列出现有用户。

## 命令行

```text
unagikuma [server] [参数]          运行监控和网页界面（默认）
unagikuma agent [参数]             作为探测节点连接 master
unagikuma node add|list|token|revoke
                                  管理探测节点及其令牌
unagikuma check [文件]             校验 monitors.bind
unagikuma serial [bump] [文件]     显示或增大序列号
unagikuma probe <目标> [k=v]       立即执行一次检查，如 "udp 10.0.0.1:ntp"
unagikuma keywords [方言]          列出 UDP 关键字
unagikuma backup [--passphrase-file 文件] [--history]
                                  生成备份归档
unagikuma restore 归档             恢复备份
unagikuma reset-password [用户]    重设密码（--generate 自动生成）
unagikuma version
```

常用服务参数：`--data 目录`（或 `UNAGIKUMA_DATA`）、`--listen 地址`（或 `UNAGIKUMA_LISTEN`，默认 `:8077`）、`--tls-cert`/`--tls-key`、`--base-url`（通知中的链接）、`--heartbeat-url`（外部心跳，例如 healthchecks.io，本机宕机时也能收到提醒）、`--trusted-proxy CIDR,...`（部署在反向代理之后时）。同一个端口同时接受 HTTP 和 TLS；指定 `--tls-cert` 时界面使用你的证书，HTTP 请求会被重定向到 https。

## 数据与备份

数据目录包含 `monitors.bind`、`core.db`（用户、故障、通知记录、配置历史、加密的密钥）、`history.db`（检查结果和可用率）以及加密密钥用的 `unagikuma.key`。`cluster-ca.*` 和 `cluster.*` 是 agent 固定的证书颁发机构，请和密钥一起保管，这样恢复后的 master 仍然能被 agent 信任。

备份是服务器运行时生成的归档，可以在 **设置 › 系统** 或命令行完成：

```sh
unagikuma backup --history                         # 数据库和 monitors.bind
unagikuma backup --passphrase-file pass.txt        # 加密，并包含密钥和集群 CA
unagikuma restore --passphrase-file pass.txt unagikuma-20261101-0300.ukbackup
```

设置口令时归档会加密（argon2id、AES-256-GCM），恢复后的 master 仍然能读取密钥，agent 也继续信任它。不设口令时，请另外保管 `unagikuma.key`（或 `UNAGIKUMA_SECRET_KEY`）。请恢复到已停止的实例；`--force` 会覆盖已有的安装。

## API

界面的所有操作都通过 `/api/v1` 下的 REST API 完成。在 **设置 › API 令牌** 中创建令牌，以 `Authorization: Bearer uk_…` 发送；令牌权限分为只读、确认、写入和管理。

- **并发修改。** API 返回的每个监控、通知通道、策略、节点和维护窗口都带有 `etag`。修改或删除时把它放在 `If-Match` 头里带回：如果这期间别人改过这一项，请求会以 `412`、错误码 `record_changed` 失败，不会覆盖别人的修改。改动其他项不会冲突。不带 `If-Match` 时以最后一次写入为准。
- **安全重试。** 带 `Idempotency-Key` 头（任意唯一字符串，最长 255 个字符）的 `POST` 只执行一次；在 `api.idempotency-ttl`（24 小时）内重复发送，会返回第一次的结果并带 `Idempotent-Replayed: true`。失败的请求不会被记住，可以用同一个 key 重试。
- **分页。** `GET /api/v1/monitors?limit=100` 按 id 顺序返回监控，并给出 `next_cursor`；把它作为 `cursor=` 传入即可取下一页，直到它为 `null`。不带 `limit` 时返回按显示顺序排好的完整列表。
- **稳定 ID。** 每个监控都有一个改名后也不变的 `id`；凡是接受名称的地方都可以用 `/api/v1/monitors/id:42` 指定它。
- 配置文件格式本身见 [monitors.bind 语法](spec/monitors-bind.md)。

## 故障排查

- **所有监控同时变红。** 看总览里的“故障在哪一段”：如果本机网关或出口不通，问题在本机这一侧，对各目标的告警会暂缓，直到恢复。外网目标下方的外网网关（`localnet.exit-target`，8.8.8.8）只在有外网目标失败时才去 ping，用来区分是对方站点坏了还是本机出不去；外网目标都正常时显示为未检查。地址可在 **设置 › 常规** 中修改（留空关闭）。
- **监控项故障了但没人收到通知。** 打开它，看抽屉中的“通知情况”；事件日志里有每次投递及其错误，**告警 › 通知通道 › 发送测试** 可以发一条测试消息。
- **Ping 检查提示“通过 TCP 检查”或权限不足。** 本机不允许 Unagikuma 发送 ping。允许非特权 ping（`sudo sysctl -w net.ipv4.ping_group_range="0 2147483647"`）或给程序 `CAP_NET_RAW` 权限，见快速开始。
- **脚本检查一直是暂停状态。** 启用时会列出缺少脚本的节点；**设置 › 检查脚本** 显示每个节点上的脚本情况。
- **"监控数据无法写入"。** 数据目录持续 `storage.write-alert`（1 分钟）以上无法写入（磁盘已满、文件系统只读）。期间检查结果和事件暂存在内存中，空间恢复后会补写；界面顶部会显示红色横幅，`/healthz` 也会返回 `"status": "degraded"`。清理磁盘空间后，故障会自动恢复。
- **改了 monitors.bind 但没生效。** 请增大序列号；有错误的文件不会生效，界面会显示错误所在的行。
- **备用节点没有状态副本。** **设置 › 节点** 显示它上次拉取主节点状态的时间和最近一次错误；失败后会在 `cluster.standby-retry` 后重试，每次配置变更后也会重新拉取。
