package main

import (
	"testing"
	"time"
)

// restart/reload/toggle 内部要等控制接口就绪（最多 30 秒），
// 沿用 30 秒的默认时限会在核心还在启动时就报 command.timeout。
func TestServiceLifecycleActionsGetStartBudget(t *testing.T) {
	for _, action := range []string{"start", "restart", "reload", "toggle"} {
		if got := defaultTimeoutFor([]string{"service", action}); got != serviceStartTimeout {
			t.Fatalf("service %s 超时 = %v，应为 %v", action, got, serviceStartTimeout)
		}
	}
}

func TestReadOnlyServiceActionsKeepDefaultBudget(t *testing.T) {
	for _, action := range []string{"status", "check"} {
		if got := defaultTimeoutFor([]string{"service", action}); got != defaultCommandTimeout {
			t.Fatalf("service %s 超时 = %v，应为 %v", action, got, defaultCommandTimeout)
		}
	}
}

func TestSubscriptionMutationsHaveNoOuterDeadline(t *testing.T) {
	if got := defaultTimeoutFor([]string{"sub", "update"}); got != time.Duration(0) {
		t.Fatalf("sub update 超时 = %v，应为 0", got)
	}
}

// config apply 在核心运行时会触发完整 reload，Android 管理器的设置开关走的正是这条路径。
func TestConfigMutationsGetLongBudget(t *testing.T) {
	cases := [][]string{
		{"config", "apply", "module", "/tmp/x"},
		{"config", "validate", "module", "/tmp/x"},
		{"config", "check"},
		{"config", "raw", "update"},
	}
	for _, args := range cases {
		if got := defaultTimeoutFor(args); got != serviceStartTimeout {
			t.Fatalf("%v 超时 = %v，应为 %v", args, got, serviceStartTimeout)
		}
	}
}

func TestConfigReadsKeepDefaultBudget(t *testing.T) {
	cases := [][]string{
		{"config", "list"},
		{"config", "read", "module"},
		{"config", "raw", "show"},
	}
	for _, args := range cases {
		if got := defaultTimeoutFor(args); got != defaultCommandTimeout {
			t.Fatalf("%v 超时 = %v，应为 %v", args, got, defaultCommandTimeout)
		}
	}
}
