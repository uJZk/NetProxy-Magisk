# 节点与订阅

节点页和订阅页使用同一份持久数据。即使服务停止，也可以浏览、添加、编辑、导出、删除或更新内容。

## 本地配置

单个链接和本地文件都追加到固定的“本地配置”分组：

- 支持常见节点链接与节点文本。
- 支持 Clash YAML、SIP008 和 sing-box JSON。
- 导入不会覆盖已有节点；重复 tag 会自动生成稳定后缀。
- 本地节点可测速、编辑、导出和删除。

文件导入不会再按文件名创建额外的本地订阅组。

### sing-box 完整配置

sing-box JSON 既可以是只含 `outbounds` 的 Provider 文档，也可以是一份完整客户端配置。订阅返回完整配置时同样按下面的规则处理：

- 只读取 `outbounds` 与 `endpoints` 中的节点，`log`、`dns`、`inbounds`、`route`、`experimental` 等段落忽略。
- `direct`、`block`、`dns`、`selector`、`urltest` 等内置出站和分组不会导入。分组由 NetProxy 自己的 `Auto/<分组>` 与 `Select/<分组>` 提供，详见[策略分组配置教程](/config/policy-groups)。
- 节点上的 `domain_resolver` 指向来源配置 `dns` 段的服务器标签，`bind_interface`、`inet4_bind_address`、`inet6_bind_address`、`protect_path`、`netns`、`routing_mark` 绑定来源设备的网络环境。这些字段会被清除，改用模块自身的 DNS 与出站设置；`connect_timeout`、`tcp_fast_open` 这类可移植参数保留。
- 个别节点使用模块不支持的协议时，其余节点照常导入，未支持的节点单独列在导入结果里。

## 订阅

订阅页面可以配置：

- 名称、URL 和 User-Agent
- 自定义请求头与 HWID
- 包含、排除筛选规则
- 下载超时、TLS 校验与代理下载策略
- 自动更新周期

名称留空时，依次尝试响应中的 `Profile-Title`、下载文件名、URL 主机名，最后使用“订阅”。支持的响应信息还包括流量用量和到期时间。

订阅更新不依赖正式服务运行。下载、转换或校验失败时，上一版有效节点会继续保留；服务运行时会继续确认新 Provider 是否已在核心中生效，并在失败时记录待同步状态。

## 完整配置直通

不想用节点与订阅功能、只想让模块运行并自动更新自己那份 sing-box 配置时，使用直通模式。模块按周期下载整份配置，`sing-box check` 通过后原子替换，然后直接运行它。

```sh
su -c '/data/adb/modules/netproxy/netproxyctl config raw set --interval 21600 https://example.com/config.json'
su -c '/data/adb/modules/netproxy/netproxyctl config raw update'
su -c '/data/adb/modules/netproxy/netproxyctl config raw enable'
su -c '/data/adb/modules/netproxy/netproxyctl service restart'
```

`config raw show` 查看来源、周期、上次更新时间与失败原因；`config raw disable` 切回托管配置但保留下载；`config raw clear` 一并删除。选项必须写在参数前面，写在后面会直接报错而不是被静默忽略。

直通模式下：

- 加载的只有「你的配置 + Service API 文档」。模块不再生成 providers、outbounds 和 eBPF 入站，节点页、分应用、出站模式、Wi-Fi 自动切换和分组选择都不参与运行。
- **抓取入口由你的配置声明**。模块在 Android 上的透明代理入站类型是 `ebpf`，在配置里自己写即可；`tun` 也可以，但需要它自己完成路由接管。模块不会再注入一份 eBPF 入站——那会和你配置里已有的叠加成两个，tag 相同直接校验失败，tag 不同则两段 eBPF 程序争抢同一批 TC 挂载点。
- 因此分应用规则要写在你自己的 `ebpf` 入站里（`include_uid` / `exclude_uid`），`app` 命令组在直通模式下不参与运行。
- 一份没有任何 `inbounds` 的配置会在 `config raw update` 阶段被直接拒绝。它能通过 `sing-box check`、能正常 ready，但一个连接都不会被代理，日志里也没有错误。
- 你的配置**不能占用 `127.0.0.1:9090`**。模块只能通过这个端口上的 Service API 判断核心是否就绪，冲突会在 `config raw update` 的检查阶段直接失败并保留旧配置。
- 下载带 ETag 与 Last-Modified 条件请求，内容没变不重写文件。下载或检查失败时保留上一版可用配置，并在 15 分钟后重试，而不是等满一个更新周期。

自动更新由 Worker 驱动，和订阅一样不需要核心在运行；只要设置了地址就会调度，因此可以先拉取确认检查通过，再切换到直通模式。

## 自动与手动选择

每个非空分组提供：

- `Auto/<分组>`：由 URLTest 自动选择。
- `Select/<分组>`：手动选择具体节点。

界面中选中 `Auto` 时，只标记 Auto，不会同时把其内部当前节点标成手动选择。手动节点在订阅更新后消失时会回到同组 Auto，不会静默切到直连。

需要在全部订阅节点之上再建立地区或业务选择器时，请参阅[策略分组配置教程](/config/policy-groups)。自定义策略组属于 sing-box 出站配置，不会改变节点页中的 Catalog 分组。

## 测速

服务运行时，测速使用当前核心的 Service API。服务停止时，模块会启动不含透明代理入站的临时 sing-box 会话完成测速，结束后清理临时进程和文件，不改变正式服务状态与当前选择。

## CLI 示例

```sh
su -c '/data/adb/modules/netproxy/netproxyctl node add "vless://..."'
su -c '/data/adb/modules/netproxy/netproxyctl node import /sdcard/Download/nodes.json'
su -c '/data/adb/modules/netproxy/netproxyctl sub add https://example.com/sub'
su -c '/data/adb/modules/netproxy/netproxyctl sub update <分组 ID>'
su -c '/data/adb/modules/netproxy/netproxyctl node use auto <分组 ID>'
su -c '/data/adb/modules/netproxy/netproxyctl node delay auto <分组 ID>'
```

订阅 URL、Header 和节点凭据属于敏感信息。脚本化配置自定义请求头时应使用 `--headers-file`，不要直接放进命令行。
