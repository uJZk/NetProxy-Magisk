package main

import (
	"context"
	"fmt"
	"strings"

	json "encoding/json/v2"

	"os"

	"testing"
)

func TestCommandCancellationContract(t *testing.T) {
	for _, test := range []struct {
		cause   error
		code    string
		message string
		status  int
	}{
		{context.Canceled, "command.canceled", "操作已取消", 1},
		{context.DeadlineExceeded, "command.timeout", "命令执行超时", 124},
	} {
		t.Run(test.code, func(t *testing.T) {
			capture, err := os.CreateTemp(t.TempDir(), "stdout-")
			if err != nil {
				t.Fatal(err)
			}
			defer capture.Close()
			previous := os.Stdout
			os.Stdout = capture
			defer func() { os.Stdout = previous }()
			status := (&cli{}).runCommand(t.Context(), func(context.Context, []string) error {
				return fmt.Errorf("等待订阅更新: %w", test.cause)
			})
			payload, err := os.ReadFile(capture.Name())
			if err != nil {
				t.Fatal(err)
			}
			var response result
			if err := json.Unmarshal(payload, &response); err != nil {
				t.Fatal(err)
			}
			expected, err := json.Marshal(result{Schema: 1, OK: false, Code: test.code, Message: test.message, Data: map[string]any{}}, json.Deterministic(true))
			if err != nil {
				t.Fatal(err)
			}
			if status != test.status || strings.TrimSpace(string(payload)) != string(expected) {
				t.Fatalf("取消契约变化: exit=%d response=%s", status, payload)
			}
		})
	}
}
