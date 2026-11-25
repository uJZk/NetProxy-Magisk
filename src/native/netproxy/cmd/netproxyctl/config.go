package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	moduleapp "github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/module"
)

func (c *cli) config(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("缺少 config 操作")
	}
	if args[0] == "upgrade-snapshot" && len(args) == 1 {
		if err := upgradeSnapshot(c.options); err != nil {
			return err
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "config.upgraded", Message: "设置迁移完成"})
		return nil
	}
	if args[0] == "raw" {
		return c.rawConfig(ctx, args[1:])
	}
	flags := newFlagSet("config")
	revision := flags.String("revision", "", "读取配置时返回的 revision，阻止覆盖并发修改")
	includePrivate := flags.Bool("include-private", false, "导出包含私钥的热点站点配置（仅 Root）")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	options := c.options
	action := args[0]
	positionals := flags.Args()
	if len(positionals) > 0 && (positionals[0] == "hotspot" || positionals[0] == "mitm") {
		switch action {
		case "read":
			if len(positionals) != 1 {
				return usageError("config read hotspot 不接受其他参数")
			}
			var data map[string]string
			var err error
			if *includePrivate {
				data, err = moduleapp.ReadHotspotBackup(options)
			} else {
				data, err = moduleapp.ReadHotspotConfig(options)
			}
			if err != nil {
				return err
			}
			writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "config.read", Message: "热点站点配置", Data: data})
			return nil
		case "apply", "validate":
			if *includePrivate {
				return usageError("--include-private 仅用于读取备份")
			}
			if len(positionals) != 2 {
				return usageError("config apply hotspot 需要内容文件")
			}
			content, err := os.ReadFile(positionals[1])
			if err != nil {
				return err
			}
			saved, err := moduleapp.ApplyHotspotConfig(ctx, options, content, action == "validate", *revision)
			if errors.Is(err, moduleapp.ErrConfigConflict) {
				return &resultError{Code: "config.conflict", Message: err.Error()}
			}
			if err != nil {
				return err
			}
			if action == "apply" {
				if err := moduleapp.EnsureWorker(ctx, options); err != nil {
					return fmt.Errorf("配置已保存，但后台 Worker 启动失败: %w", err)
				}
			}
			writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "config." + action, Message: "热点站点配置已处理", Data: map[string]string{"target": "hotspot", "revision": saved}})
			return nil
		}
	}
	if options.RawOnly && action != "migrate-raw-module" && action != "check" && (len(positionals) == 0 || positionals[0] != "module/auto_start") {
		return usageError("配置目标：hotspot、module/auto_start、raw")
	}
	if *includePrivate {
		return usageError("--include-private 仅用于读取热点站点备份")
	}
	switch action {
	case "migrate-raw-module":
		if len(positionals) != 2 {
			return usageError("config migrate-raw-module 需要模块快照和直通设置文件")
		}
		if err := migrateRawModule(positionals[0], positionals[1]); err != nil {
			return err
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "config.migrate_raw_module", Message: "旧版直通模块设置已转换", Data: map[string]any{}})
	case "list":
		data, err := moduleapp.ListConfigs(options)
		if err != nil {
			return err
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "config.list", Message: "sing-box 配置列表", Data: data})
	case "read":
		if len(positionals) == 0 {
			return errors.New("config read 需要目标")
		}
		data, err := moduleapp.ReadConfig(options, positionals[0])
		if err != nil {
			return err
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "config.read", Message: "配置内容", Data: data})
	case "check":
		err := moduleapp.CheckService(ctx, options)
		if err != nil {
			return err
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "config.checked", Message: "sing-box 配置检查通过", Data: map[string]any{}})
	case "validate", "apply":
		if len(positionals) < 2 {
			return errors.New("config 操作需要目标和内容文件")
		}
		savedRevision, err := moduleapp.ApplyConfig(ctx, options, positionals[0], positionals[1], action == "validate", *revision)
		if err != nil {
			if errors.Is(err, moduleapp.ErrConfigConflict) {
				return &resultError{Code: "config.conflict", Message: err.Error()}
			}
			return err
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "config." + action, Message: "配置检查通过", Data: map[string]string{"target": positionals[0], "revision": savedRevision}})
	default:
		return fmt.Errorf("未知 config 操作 %q", action)
	}
	return nil
}
