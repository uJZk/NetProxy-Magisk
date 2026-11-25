package main

import (
	"bytes"

	"reflect"
	"strings"
	"testing"
	"time"
)

func TestWriteJSONPreservesEmptyDataObject(t *testing.T) {
	var output bytes.Buffer
	writeJSON(&output, result{Schema: 1, OK: false, Code: "test.failed", Message: "测试失败", Data: map[string]any{}})
	if !strings.Contains(output.String(), `"data":{}`) {
		t.Fatalf("schema=1 空 data 对象丢失: %s", output.String())
	}
}

func TestParseCommandArgsSupportsMixedOptions(t *testing.T) {
	args, timeout, err := parseCommandArgs([]string{
		"service", "status", "--json", "--timeout=45s",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(args, []string{"service", "status"}) {
		t.Fatalf("clean args = %v", args)
	}
	if timeout != 45*time.Second {
		t.Fatalf("timeout = %s, want 45s", timeout)
	}
}

func TestParseCommandTimeoutAcceptsSecondsAndDurations(t *testing.T) {
	for value, want := range map[string]time.Duration{
		"30": 30 * time.Second,
		"2m": 2 * time.Minute,
	} {
		got, err := parseCommandTimeout(value)
		if err != nil {
			t.Fatalf("parse %q: %v", value, err)
		}
		if got != want {
			t.Fatalf("parse %q = %s, want %s", value, got, want)
		}
	}
	if _, err := parseCommandTimeout("0"); err == nil {
		t.Fatal("zero timeout should fail")
	}
}

func TestInternalUsageOnlyListsProcessEntrypoints(t *testing.T) {
	usage := internalUsageText()
	for _, expected := range []string{"__internal boot", "__internal worker <start|stop|run>"} {
		if !strings.Contains(usage, expected) {
			t.Fatalf("usage missing process entry %q: %s", expected, usage)
		}
	}
	for _, removed := range []string{"__internal module", "__internal catalog", "__internal control", "__internal ebpf", "__internal version", "netproxy-native"} {
		if strings.Contains(usage, removed) {
			t.Fatalf("usage still lists removed entry %q: %s", removed, usage)
		}
	}
}
