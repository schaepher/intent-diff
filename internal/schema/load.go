package schema

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"intent-review/internal/report"
)

// Load 读取并解码评审文件。未知字段被忽略（minor 兼容），
// 保留原始字节由 fill 负责（见 internal/fill）。
func Load(path string) (Document, []byte, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Document{}, nil, err
	}
	doc, err := Decode(raw)
	if err != nil {
		return Document{}, raw, err
	}
	return doc, raw, nil
}

// Decode 解码 JSON 字节为 Document。
func Decode(raw []byte) (Document, error) {
	var doc Document
	if err := json.Unmarshal(raw, &doc); err != nil {
		return Document{}, fmt.Errorf("解析 JSON 失败: %w", err)
	}
	return doc, nil
}

// ParseVersion 拆分 major.minor。
func ParseVersion(v string) (major int, minor int, err error) {
	parts := strings.SplitN(strings.TrimSpace(v), ".", 2)
	if len(parts) == 0 || parts[0] == "" {
		return 0, 0, fmt.Errorf("schema_version 为空")
	}
	major, err = strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, fmt.Errorf("schema_version %q 不是 major.minor 形式", v)
	}
	if len(parts) == 2 && parts[1] != "" {
		minor, err = strconv.Atoi(parts[1])
		if err != nil {
			return 0, 0, fmt.Errorf("schema_version %q 不是 major.minor 形式", v)
		}
	}
	return major, minor, nil
}

// CheckVersion 校验 schema 版本：major 必须匹配，minor 任意（忽略未知字段）。
func CheckVersion(v string) *report.Error {
	if strings.TrimSpace(v) == "" {
		e := report.Error{
			Code:    report.CodeSchemaInvalid,
			Message: "缺少 schema_version",
			Hint:    fmt.Sprintf("当前支持 %s", SchemaVersion),
		}
		return &e
	}
	major, _, err := ParseVersion(v)
	if err != nil {
		e := report.Error{Code: report.CodeSchemaInvalid, Message: err.Error()}
		return &e
	}
	if major != SupportedMajor {
		e := report.Error{
			Code:    report.CodeSchemaVersion,
			Message: fmt.Sprintf("schema_version %q 的 major 版本不受支持", v),
			Hint:    fmt.Sprintf("本工具支持 major=%d（当前 canonical 版本 %s）", SupportedMajor, SchemaVersion),
		}
		return &e
	}
	return nil
}
