package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	moduleapp "github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/module"
)

func (c *cli) rawConfig(ctx context.Context, args []string) error {
	if len(args) == 0 {
		args = []string{"show"}
	}
	action := args[0]
	flags := newFlagSet("config raw " + action)
	interval := flags.Int64("interval", 86400, "更新周期，单位秒；0 表示手动更新")
	agent := flags.String("user-agent", "", "下载 User-Agent")
	force := flags.Bool("force", false, "强制重新下载")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	pos := flags.Args()
	for _, p := range pos {
		if strings.HasPrefix(p, "-") {
			return errors.New("选项必须位于地址之前")
		}
	}
	var data any = map[string]any{}
	switch action {
	case "show":
		if len(pos) != 0 {
			return errors.New("show 不接受参数")
		}
		state, err := moduleapp.ReadRawConfig(c.options)
		if err != nil {
			return err
		}
		data = state
	case "set":
		if len(pos) != 1 {
			return errors.New("set 需要一个配置地址")
		}
		if err := moduleapp.SetRawConfigSource(ctx, c.options, pos[0], *interval, *agent); err != nil {
			return err
		}
		if *interval > 0 {
			if err := moduleapp.EnsureWorker(ctx, c.options); err != nil {
				return fmt.Errorf("设置已保存，Worker 启动失败: %w", err)
			}
		}
	case "update":
		if len(pos) != 0 {
			return errors.New("update 不接受位置参数")
		}
		v, err := moduleapp.UpdateRawConfig(ctx, c.options, *force)
		if err != nil {
			return err
		}
		data = v
	case "enable", "disable":
		if len(pos) != 0 {
			return errors.New("操作不接受位置参数")
		}
		if err := moduleapp.SetRawEnabled(ctx, c.options, action == "enable"); err != nil {
			return err
		}
	case "clear":
		if len(pos) != 0 {
			return errors.New("clear 不接受位置参数")
		}
		if err := moduleapp.ClearRawConfig(ctx, c.options); err != nil {
			return err
		}
	default:
		return fmt.Errorf("未知 config raw 操作 %q", action)
	}
	writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "config.raw_" + action, Message: "直通配置操作完成", Data: data})
	return nil
}
