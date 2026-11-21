package hotspot

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"regexp"
	"strings"

	SJSON "github.com/sagernet/sing/common/json"
)

// ProjectSiteDNS maps the site's domain to its fixed additional address.
func ProjectSiteDNS(source []byte, domain string) ([]byte, error) {
	return ProjectSiteRules(source, domain, nil)
}

// ProjectConfiguredDNS preserves the original precedence: explicit local
// answers precede the blocklist, whose exact exceptions bypass that list.
func ProjectConfiguredDNS(source []byte, settings Settings) ([]byte, error) {
	block, err := blockRule(settings.DNSBlock, settings.DNSAllow)
	if err != nil {
		return nil, err
	}
	if block != nil {
		clean, err := io.ReadAll(SJSON.NewCommentFilter(bytes.NewReader(source)))
		if err != nil {
			return nil, err
		}
		var root map[string]json.RawMessage
		if err := json.Unmarshal(clean, &root); err != nil || root == nil {
			return nil, errors.New("完整配置必须是 JSON 对象")
		}
		dns := make(map[string]json.RawMessage)
		if v, ok := root["dns"]; ok && string(v) != "null" {
			if err := json.Unmarshal(v, &dns); err != nil || dns == nil {
				return nil, errors.New("dns 必须是 JSON 对象")
			}
		}
		var rules []json.RawMessage
		if v, ok := dns["rules"]; ok && string(v) != "null" {
			if err := json.Unmarshal(v, &rules); err != nil {
				return nil, errors.New("dns.rules 必须是数组")
			}
		}
		dns["rules"], err = json.Marshal(append([]json.RawMessage{block}, rules...))
		if err != nil {
			return nil, err
		}
		root["dns"], err = json.Marshal(dns)
		if err != nil {
			return nil, err
		}
		source, err = json.Marshal(root)
		if err != nil {
			return nil, err
		}
	}
	return ProjectSiteRules(source, settings.Domain, settings.DNSOverrides)
}

func blockRule(block, allow []string) (json.RawMessage, error) {
	if len(block) > 64 || len(allow) > 32 {
		return nil, errors.New("DNS 黑名单或例外超过数量限制")
	}
	var patterns, exceptions []string
	for _, value := range block {
		value = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(value), "."))
		if len(value) == 0 || len(value) > 63 || !validGlob(value) {
			return nil, errors.New("DNS 黑名单规则无效")
		}
		var b strings.Builder
		b.WriteString("(?i)^")
		for _, c := range value {
			switch c {
			case '*':
				b.WriteString(".*")
			case '?':
				b.WriteByte('.')
			default:
				b.WriteString(regexp.QuoteMeta(string(c)))
			}
		}
		b.WriteString("$")
		patterns = append(patterns, b.String())
	}
	for _, value := range allow {
		value = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(value), "."))
		if !validDNSName(value) {
			return nil, errors.New("DNS 例外域名无效")
		}
		exceptions = append(exceptions, value)
	}
	if len(patterns) == 0 {
		return nil, nil
	}
	var rule map[string]any
	if len(exceptions) == 0 {
		rule = map[string]any{"domain_regex": patterns, "action": "predefined", "rcode": "NXDOMAIN"}
	} else {
		rule = map[string]any{"type": "logical", "mode": "and", "rules": []any{map[string]any{"domain_regex": patterns}, map[string]any{"domain": exceptions, "invert": true}}, "action": "predefined", "rcode": "NXDOMAIN"}
	}
	return json.Marshal(rule)
}

func validGlob(value string) bool {
	hasAlnum := false
	for i, c := range value {
		if c >= 'a' && c <= 'z' || c >= '0' && c <= '9' {
			hasAlnum = true
			continue
		}
		if c == '.' && (i == 0 || value[i-1] == '.') {
			return false
		}
		if c != '.' && c != '-' && c != '_' && c != '*' && c != '?' {
			return false
		}
	}
	return hasAlnum
}

func normalizedOverrides(domain string, overrides []DNSOverride) ([]DNSOverride, error) {
	domain = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(domain), "."))
	if !validDomain(domain) || len(domain) > 63 || len(overrides) > 16 {
		return nil, errors.New("站点域名无效或自定义解析超过 16 条")
	}
	result := make([]DNSOverride, 0, len(overrides)+1)
	seen := make(map[string]bool)
	for _, item := range overrides {
		item.Domain = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(item.Domain), "."))
		if !validDNSName(item.Domain) || seen[item.Domain] {
			return nil, errors.New("自定义解析域名无效或重复")
		}
		seen[item.Domain] = true
		if item.Address == "" {
			item.Address = DefaultSiteAddress
		}
		addr, err := netip.ParseAddr(item.Address)
		if err != nil || !addr.Is4() || addr.IsUnspecified() {
			return nil, errors.New("自定义解析需要有效 IPv4 地址")
		}
		item.Address = addr.String()
		result = append(result, item)
	}
	if !seen[domain] {
		if len(result) >= 16 {
			return nil, errors.New("自定义解析包含站点域名时最多为 16 条")
		}
		result = append(result, DNSOverride{Domain: domain, Address: DefaultSiteAddress})
	}
	return result, nil
}

// ProjectSiteRules projects the site's domain and custom exact-name mappings.
// Project in reverse so their order remains the one given by the user.
func ProjectSiteRules(source []byte, domain string, overrides []DNSOverride) ([]byte, error) {
	rules, err := normalizedOverrides(domain, overrides)
	if err != nil {
		return nil, err
	}
	for i := len(rules) - 1; i >= 0; i-- {
		address, err := netip.ParseAddr(rules[i].Address)
		if err != nil {
			return nil, err
		}
		source, err = ProjectDNS(source, rules[i].Domain, address)
		if err != nil {
			return nil, err
		}
	}
	return source, nil
}

// ProjectDNS inserts the site's local answer ahead of user rules in a copy of
// the complete configuration. It does not modify the downloaded source.
func ProjectDNS(source []byte, domain string, address netip.Addr) ([]byte, error) {
	domain = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(domain), "."))
	if !validDNSName(domain) || !address.Is4() || address.IsUnspecified() {
		return nil, errors.New("热点站点需要有效域名和 IPv4 地址")
	}
	// Complete configurations may contain sing-box JSON comments.
	clean, err := io.ReadAll(SJSON.NewCommentFilter(bytes.NewReader(source)))
	if err != nil {
		return nil, err
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(clean, &root); err != nil || root == nil {
		return nil, errors.New("完整配置必须是 JSON 对象")
	}
	dns := make(map[string]json.RawMessage)
	if v, ok := root["dns"]; ok && string(v) != "null" {
		if err := json.Unmarshal(v, &dns); err != nil || dns == nil {
			return nil, errors.New("dns 必须是 JSON 对象")
		}
	}
	var rules []json.RawMessage
	if v, ok := dns["rules"]; ok && string(v) != "null" {
		if err := json.Unmarshal(v, &rules); err != nil {
			return nil, errors.New("dns.rules 必须是数组")
		}
	}
	answer := fmt.Sprintf("%s. 60 IN A %s", domain, address)
	first, err := json.Marshal(map[string]any{"domain": domain, "query_type": "A", "action": "predefined", "answer": []string{answer}})
	if err != nil {
		return nil, err
	}
	other, err := json.Marshal(map[string]any{"domain": domain, "action": "predefined"})
	if err != nil {
		return nil, err
	}
	dns["rules"], err = json.Marshal(append([]json.RawMessage{first, other}, rules...))
	if err != nil {
		return nil, err
	}
	root["dns"], err = json.Marshal(dns)
	if err != nil {
		return nil, err
	}
	return json.Marshal(root)
}

func validDomain(domain string) bool {
	if len(domain) == 0 || len(domain) > 253 {
		return false
	}
	for _, label := range strings.Split(domain, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, c := range label {
			if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
				return false
			}
		}
	}
	return true
}

// DNS entries allow underscores, unlike HTTPS certificate hostnames.
func validDNSName(name string) bool {
	if len(name) == 0 || len(name) > 63 {
		return false
	}
	for _, label := range strings.Split(name, ".") {
		if len(label) == 0 || len(label) > 63 {
			return false
		}
		for _, c := range label {
			if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
				return false
			}
		}
	}
	return true
}
