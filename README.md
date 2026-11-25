# NetProxy

面向 Android arm64 的独立 sing-box 模块，支持 Magisk、KernelSU 与 APatch。用户提供一份完整的 sing-box 配置，模块负责校验、运行及定时更新。

## 使用

打开模块 WebUI，在 **sing-box 配置** 中填写 URL 或导入本地文件，再打开总开关。开机自启独立设置。更新间隔以秒为单位，设为 0 可改为手动更新。清除下载缓存会移除 HTTP 缓存校验信息，保留当前配置和来源。

**sing-box 面板** 从原始配置读取 Service API 的 dashboard 或 Clash API 的 external_ui，使用对应监听地址打开。未配置时入口置灰。下载前端资源的 URL 仅供核心下载。

**MITM** 在指定 IPv4 地址提供 TLS 自定义 JSON 接口。启用时需提供匹配的证书链和私钥。客户端必须信任该证书，证书应覆盖实际访问的域名。设备匹配、目标域名、DNS 映射及黑白名单均在 sing-box 原始配置中管理。模块在回环接口配置目标地址，停用后清理本实例添加的地址。

MITM 导出为本项目 JSON 备份，包含私钥。导入支持本项目 JSON（含旧热点站点格式）及 ESP32 Repeater v1 YAML。ESP32 的证书和接口按段导入，缺省段保留。Wi-Fi、DNS 和站点域名仅属于参照格式。导入后可检查内容并保存。

**日志** 分为模块服务、配置更新、sing-box 与 MITM。核心日志级别由原始配置中的 `log.level` 控制。

## 核心渠道

正式版与测试版由本仓库的 Core Channels 工作流从 reF1nd/sing-box 的对应标签构建。安装包内置两个渠道，可以离线切换。更新下载指定渠道到本地，校验发布摘要、架构及版本。切换会先用候选核心校验当前配置，正在运行的服务会重启，启动失败时尝试恢复原核心。首次下载需要本仓库已生成对应的渠道 Release。

## CLI

在 root Shell 中运行以下命令。命令路径为 /data/adb/modules/netproxy/netproxyctl。

```sh
netproxyctl service status
netproxyctl config raw import /sdcard/Download/config.json
netproxyctl config raw set --interval 86400 https://example.org/sing-box.json
netproxyctl config raw update
netproxyctl service start
netproxyctl config raw panel
netproxyctl logs show core
netproxyctl core update stable
netproxyctl core switch stable
```

安装包通过本仓库的 build-module 工作流构建。Go 源码位于 src/native/netproxy，移动端 WebUI 位于 src/webui，模块暂存清单见 .github/scripts/stage-module.sh。升级使用管理器暂存目录，重启生效。

[工程准则](docs/maintenance/engineering.md) · [许可证](LICENSE) · [第三方声明](NOTICE)
