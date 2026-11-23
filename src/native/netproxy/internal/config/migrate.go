package config

import (
	"bytes"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"strings"
)

// MigrateRawModule 只转换旧版直通模式中已不再使用的活动节点选择。
func MigrateRawModule(content, rawSettings []byte) ([]byte, error) {
	var raw struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.Unmarshal(rawSettings, &raw); err != nil || !raw.Enabled {
		return nil, errors.New("仅支持迁移已启用完整配置直通的旧版配置")
	}
	var fields map[string]jsontext.Value
	if err := json.Unmarshal(content, &fields); err != nil || fields == nil {
		return nil, errors.New("旧模块配置不是有效的 JSON 对象")
	}
	selection, ok := fields["selection"]
	selection = bytes.TrimSpace(selection)
	if !ok || len(selection) == 0 || selection[0] != '{' {
		return nil, errors.New("旧模块配置缺少有效的 selection")
	}
	var old struct {
		GroupID string `json:"group_id"`
		NodeTag string `json:"node_tag"`
	}
	decoder := jsontext.NewDecoder(bytes.NewReader(selection))
	for {
		token, err := decoder.ReadToken()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if token.Kind() == 'n' {
			return nil, errors.New("旧节点选择不能包含 null")
		}
	}
	if err := json.Unmarshal(selection, &old, json.RejectUnknownMembers(true)); err != nil {
		return nil, fmt.Errorf("旧节点选择无效: %w", err)
	}
	if old.NodeTag != "" && (old.GroupID == "" || strings.TrimSpace(old.NodeTag) == "") {
		return nil, errors.New("旧节点选择无效")
	}
	delete(fields, "selection")
	migrated, err := json.Marshal(fields, json.Deterministic(true))
	if err != nil {
		return nil, err
	}
	if _, err := ParseModule(migrated); err != nil {
		return nil, err
	}
	return append(migrated, '\n'), nil
}
