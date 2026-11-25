# NetProxy

Standalone Android arm64 module for running a complete sing-box configuration. The module WebUI provides sing-box configuration and service controls, a dashboard link derived from the user's configuration, MITM HTTPS responses, and component logs. The CLI remains available at /data/adb/modules/netproxy/netproxyctl.

The module includes stable and testing cores built from reF1nd tags. Downloading an update stages it for the selected channel. Switching validates the active configuration and restarts a running service. The original sing-box configuration controls the core log level.

MITM imports NetProxy JSON and ESP32 Repeater v1 YAML backups. The exported NetProxy backup contains the configured private key. Device matching, domains and DNS policies belong in the original sing-box configuration.

[Usage and CLI](README.md) · [License](LICENSE) · [Notices](NOTICE)
