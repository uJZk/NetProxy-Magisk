package main

import (
	"context"
	moduleapp "github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/module"
	"os"
)

func (c *cli) core(ctx context.Context, args []string) error {
	var data any
	var err error
	if len(args) == 0 {
		args = []string{"status"}
	}
	switch {
	case len(args) == 1 && args[0] == "status":
		data, err = moduleapp.CoreStatus(ctx, c.options)
	case len(args) == 2 && args[0] == "update":
		err = moduleapp.UpdateCore(ctx, c.options, args[1])
	case len(args) == 2 && args[0] == "switch":
		err = moduleapp.SwitchCore(ctx, c.options, args[1])
	default:
		return usageError("core status | update stable|testing | switch stable|testing")
	}
	if err != nil {
		return err
	}
	writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "core." + args[0], Message: "核心操作成功", Data: data})
	return nil
}
