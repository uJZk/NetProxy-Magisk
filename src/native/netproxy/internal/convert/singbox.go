package convert

import (
	"context"
	"fmt"
	"strings"

	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/option"
	providerparser "github.com/sagernet/sing-box/provider/parser"
	SJSON "github.com/sagernet/sing/common/json"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/provider"
)

// singBoxGroupTypes 是 sing-box 完整配置里不代表节点的出站类型。
// NetProxy 自行生成 Auto/Select/Proxy 出站图，导入来源配置的分组会与之重名；
// 集合与 sing-box provider 解析器丢弃的类型保持一致，逐节点回退时需要同样静默跳过。
var singBoxGroupTypes = map[string]struct{}{
	C.TypeDirect:   {},
	C.TypeBlock:    {},
	C.TypeDNS:      {},
	C.TypeSelector: {},
	C.TypeURLTest:  {},
	C.TypePass:     {},
}

type singBoxNodeHeader struct {
	Type string `json:"type"`
	Tag  string `json:"tag"`
}

// parseSingBox 解析 sing-box 原始配置或 Provider 文档。
// handled 为 false 表示内容不含 outbounds/endpoints，调用方应继续尝试其他订阅格式。
func parseSingBox(ctx context.Context, content string, allowInsecure bool) (result provider.ParseResult, handled bool, err error) {
	// 整份文档优先交给 sing-box 解析：它承载注释容错、类型注册和分组过滤，
	// 逐节点回退只是为了在个别节点不受支持时仍能导入其余节点。
	outbounds, endpoints, parseErr := providerparser.ParseBoxSubscription(ctx, content)
	if parseErr == nil {
		document := provider.Document{Outbounds: outbounds, Endpoints: endpoints}
		stripSourceEnvironment(&document)
		result, err = finish(document, nil, allowInsecure)
		return result, true, err
	}

	sections, ok := singBoxSections(ctx, content)
	if !ok {
		return provider.ParseResult{}, false, nil
	}

	var document provider.Document
	var diagnostics []provider.Diagnostic
	for _, section := range sections {
		for index, raw := range section.nodes {
			nodeOutbounds, nodeEndpoints, diagnostic := parseSingBoxNode(ctx, section.name, index, raw)
			if diagnostic != nil {
				diagnostics = append(diagnostics, *diagnostic)
				continue
			}
			document.Outbounds = append(document.Outbounds, nodeOutbounds...)
			document.Endpoints = append(document.Endpoints, nodeEndpoints...)
		}
	}
	stripSourceEnvironment(&document)
	result, err = finish(document, diagnostics, allowInsecure)
	return result, true, err
}

type singBoxSection struct {
	name  string
	nodes []SJSON.RawMessage
}

// singBoxSections 拆出 sing-box 文档的节点数组。
// 使用 sing 的 JSON 解码器而非 encoding/json/v2，才能保留 sing-box 配置允许的注释。
func singBoxSections(ctx context.Context, content string) ([]singBoxSection, bool) {
	var shape map[string]SJSON.RawMessage
	if err := SJSON.UnmarshalContext(ctx, []byte(content), &shape); err != nil {
		return nil, false
	}
	var sections []singBoxSection
	for _, name := range []string{"outbounds", "endpoints"} {
		raw, exists := shape[name]
		if !exists {
			continue
		}
		var nodes []SJSON.RawMessage
		if err := SJSON.UnmarshalContext(ctx, raw, &nodes); err != nil {
			return nil, false
		}
		sections = append(sections, singBoxSection{name: name, nodes: nodes})
	}
	return sections, len(sections) > 0
}

func parseSingBoxNode(ctx context.Context, section string, index int, raw SJSON.RawMessage) ([]option.Outbound, []option.Endpoint, *provider.Diagnostic) {
	label := fmt.Sprintf("%s[%d]", section, index)
	var header singBoxNodeHeader
	if err := SJSON.UnmarshalContext(ctx, raw, &header); err != nil {
		return nil, nil, &provider.Diagnostic{Index: index + 1, Source: label, Code: "singbox.node_invalid", Message: err.Error()}
	}
	if tag := strings.TrimSpace(header.Tag); tag != "" {
		label = tag
	}
	if header.Type == "" {
		return nil, nil, &provider.Diagnostic{Index: index + 1, Source: label, Code: "singbox.node_invalid", Message: fmt.Sprintf("missing %s type", strings.TrimSuffix(section, "s"))}
	}
	if _, isGroup := singBoxGroupTypes[header.Type]; isGroup && section == "outbounds" {
		return nil, nil, nil
	}
	outbounds, endpoints, err := providerparser.ParseBoxSubscription(ctx, singleNodeDocument(section, raw))
	if err != nil {
		return nil, nil, &provider.Diagnostic{
			Index:   index + 1,
			Source:  label,
			Code:    "singbox.protocol_unsupported",
			Message: fmt.Sprintf("unsupported sing-box protocol %q: %s", header.Type, err.Error()),
		}
	}
	return outbounds, endpoints, nil
}

func singleNodeDocument(section string, raw SJSON.RawMessage) string {
	var builder strings.Builder
	builder.WriteString(`{"`)
	builder.WriteString(section)
	builder.WriteString(`":[`)
	builder.Write(raw)
	builder.WriteString(`]}`)
	return builder.String()
}

// stripSourceEnvironment 清除只在来源配置或来源设备上成立的拨号设置。
// domain_resolver 指向来源配置 dns 段的服务器标签，NetProxy 没有同名标签，保留会让
// sing-box 启动时报 DNS server not found；bind_interface、inet4/6_bind_address、
// protect_path、netns 和 routing_mark 绑定来源设备的网络环境，保留会让节点在本机
// 静默连不通，其中 routing_mark 还会覆盖 sing-box 自己的防回环标记。
// detour 不在此处理：sing-box 加载 Local Provider 时会重写或清除组内引用。
func stripSourceEnvironment(document *provider.Document) {
	for index := range document.Outbounds {
		stripNodeEnvironment(document.Outbounds[index].Options)
	}
	for index := range document.Endpoints {
		stripNodeEnvironment(document.Endpoints[index].Options)
	}
}

func stripNodeEnvironment(options any) {
	switch typed := options.(type) {
	case *option.WireGuardEndpointOptions:
		typed.InnerDomainResolver = nil
	case *option.TailscaleEndpointOptions:
		typed.InnerDomainResolver = nil
	}
	wrapper, ok := options.(option.DialerOptionsWrapper)
	if !ok {
		return
	}
	dialer := wrapper.TakeDialerOptions()
	dialer.DomainResolver = nil
	dialer.BindInterface = ""
	dialer.Inet4BindAddress = nil
	dialer.Inet6BindAddress = nil
	dialer.ProtectPath = ""
	dialer.RoutingMark = 0
	dialer.NetNs = ""
	wrapper.ReplaceDialerOptions(dialer)
}
