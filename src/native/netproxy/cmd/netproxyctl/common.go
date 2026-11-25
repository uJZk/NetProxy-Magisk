package main

import (
	"context"
	"fmt"
	"io"
	"os"

	json "encoding/json/v2"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/contract"
)

func usageError(message string) error {
	return &resultError{Code: "usage.invalid", Message: message, Status: 2}
}

type commandHandler func(context.Context, []string) error

func (c *cli) runCommand(ctx context.Context, handler commandHandler, args ...string) int {
	if err := handler(ctx, args); err != nil {
		response, status := contract.Failure(ctx, err)
		writeJSON(os.Stdout, response)
		return status
	}
	return 0
}

func (c *cli) fail(code, message string, status int) int {
	return c.failData(code, message, map[string]any{}, status)
}

func (c *cli) failData(code, message string, data any, status int) int {
	if status <= 0 {
		status = 1
	}
	writeJSON(os.Stdout, result{Schema: 1, OK: false, Code: code, Message: message, Data: data})
	return status
}

func (c *cli) help() {

	fmt.Fprintln(os.Stdout, `netproxyctl [--json] [--timeout 时长]
service status|start|stop|toggle|restart|reload|check
config raw show|set [--interval 秒] URL|update [--force]|import 文件|clear-cache|panel
config read [--include-private] mitm
config read module/auto_start
config apply [--revision 版本] mitm|module/auto_start 文件
core status|update stable|testing|switch stable|testing
logs show|clear core|service|mitm|config`)
}

func writeJSON(writer io.Writer, value any) {
	if err := json.MarshalWrite(writer, value, json.Deterministic(true)); err != nil {
		return
	}
	_, _ = io.WriteString(writer, "\n")
}
