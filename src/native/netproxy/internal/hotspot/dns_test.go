package hotspot

import (
	"context"
	"encoding/json"
	"net/netip"
	"testing"

	"github.com/sagernet/sing-box/option"
)

func TestProjectDNSPrependsAndPreservesSource(t *testing.T) {
	source := []byte(`{"dns":{"servers":[{"tag":"upstream"}],"rules":[{"domain":"example.org","server":"upstream"}]},"other":{"large":12345678901234567890}}`)
	output, err := ProjectDNS(source, "WWW.Example.com.", netip.MustParseAddr("192.168.1.1"))
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		DNS struct {
			Rules []map[string]any `json:"rules"`
		} `json:"dns"`
		Other json.RawMessage `json:"other"`
	}
	if err := json.Unmarshal(output, &result); err != nil {
		t.Fatal(err)
	}
	if len(result.DNS.Rules) != 3 || result.DNS.Rules[0]["domain"] != "www.example.com" || result.DNS.Rules[0]["query_type"] != "A" || result.DNS.Rules[2]["domain"] != "example.org" {
		t.Fatalf("unexpected rules: %s", output)
	}
	if string(result.Other) != `{"large":12345678901234567890}` {
		t.Fatal(string(result.Other))
	}
	for _, r := range result.DNS.Rules[:2] {
		encoded, err := json.Marshal(r)
		if err != nil {
			t.Fatal(err)
		}
		var rule option.DNSRule
		if err := rule.UnmarshalJSONContext(context.Background(), encoded); err != nil || !rule.IsValid() {
			t.Fatalf("core rejected %s: %v", encoded, err)
		}
	}
}

func TestProjectDNSRejectsInvalidInput(t *testing.T) {
	for _, tc := range []struct{ document, domain, address string }{
		{`[]`, "a.test", "192.168.1.1"},
		{`{"dns":{"rules":{}}}`, "a.test", "192.168.1.1"},
		{`{}`, "bad..name", "192.168.1.1"},
		{`{}`, "a.test", "::1"},
	} {
		if _, err := ProjectDNS([]byte(tc.document), tc.domain, netip.MustParseAddr(tc.address)); err == nil {
			t.Fatalf("accepted %+v", tc)
		}
	}
}

func TestSiteDNSUsesFixedAddress(t *testing.T) {
	output, err := ProjectSiteDNS([]byte(`{}`), DefaultDomain)
	if err != nil {
		t.Fatal(err)
	}
	var data struct {
		DNS struct {
			Rules []struct {
				Answer []string `json:"answer"`
			} `json:"rules"`
		} `json:"dns"`
	}
	if err := json.Unmarshal(output, &data); err != nil {
		t.Fatal(err)
	}
	if len(data.DNS.Rules) != 2 || len(data.DNS.Rules[0].Answer) != 1 || data.DNS.Rules[0].Answer[0] != "www.aabbcc.com. 60 IN A 192.2.0.1" {
		t.Fatalf("unexpected projection: %s", output)
	}
}

func TestSiteDNSCustomOverridesAndExistingRules(t *testing.T) {
	data, err := ProjectSiteRules([]byte(`{"dns":{"rules":[{"domain":"other.example","server":"upstream"}]}}`), "www.aabbcc.com", []DNSOverride{{Domain: "custom.example", Address: "10.0.0.8"}, {Domain: "local.example"}})
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		DNS struct {
			Rules []struct {
				Domain string   `json:"domain"`
				Answer []string `json:"answer"`
			} `json:"rules"`
		} `json:"dns"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	rules := result.DNS.Rules
	if len(rules) != 7 || rules[0].Domain != "custom.example" || rules[0].Answer[0] != "custom.example. 60 IN A 10.0.0.8" || rules[2].Answer[0] != "local.example. 60 IN A 192.2.0.1" || rules[4].Domain != DefaultDomain || rules[6].Domain != "other.example" {
		t.Fatalf("unexpected projection: %s", data)
	}
	if _, err := ProjectSiteRules([]byte(`{}`), DefaultDomain, []DNSOverride{{Domain: "X.example"}, {Domain: "x.example."}}); err == nil {
		t.Fatal("accepted duplicate domain")
	}
}

func TestBlockRulePrecedenceAndCoreFormat(t *testing.T) {
	s := DefaultSettings()
	s.DNSBlock = []string{"*.Example.com", "ads?.test"}
	s.DNSAllow = []string{"safe.example.com"}
	data, err := ProjectConfiguredDNS([]byte(`{"dns":{"rules":[{"domain":"old.test","server":"upstream"}]}}`), s)
	if err != nil {
		t.Fatal(err)
	}
	var parsed struct {
		DNS struct {
			Rules []json.RawMessage `json:"rules"`
		} `json:"dns"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatal(err)
	}
	if len(parsed.DNS.Rules) != 4 {
		t.Fatalf("unexpected rules: %s", data)
	}
	var rule option.DNSRule
	if err := rule.UnmarshalJSONContext(context.Background(), parsed.DNS.Rules[2]); err != nil || !rule.IsValid() {
		t.Fatalf("core rejected %s: %v", parsed.DNS.Rules[2], err)
	}
	if _, err := blockRule([]string{"*"}, nil); err == nil {
		t.Fatal("accepted universal wildcard")
	}
}
