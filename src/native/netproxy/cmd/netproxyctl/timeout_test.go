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
