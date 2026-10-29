// Package diffpatch 解析 unified diff 行数组（仅 hunk 级，不引入第三方 diff 库）。
//
// 一个 change 只允许一个 hunk：同一文件的多处修改应拆成多个 change，
// 这样每个 change 都携带独立的业务解释（设计文档 §4.3）。
package diffpatch

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"intent-review/internal/report"
	"intent-review/internal/schema"
)

// Hunk 是一个解析后的 diff 变更块。
type Hunk struct {
	OldStart int
	OldCount int
	NewStart int
	NewCount int
	Lines    []schema.Line
}

var hunkHeader = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@`)

// Parse 解析 patch 行数组。忽略 `diff --git`/`index`/`---`/`+++` 前缀行，
// 但拒绝出现第二个 hunk。
func Parse(patch []string) (*Hunk, *report.Error) {
	if len(patch) == 0 {
		return nil, &report.Error{Code: report.CodePatchParse, Message: "patch 为空"}
	}
	h := &Hunk{}
	body := 0
	seenHeader := false
	for _, raw := range patch {
		line := strings.TrimSuffix(raw, "\r")
		switch {
		case strings.HasPrefix(line, "@@"):
			if seenHeader {
				return nil, &report.Error{
					Code: report.CodePatchParse, Message: "patch 含多个 hunk",
					Hint: "一个 change 只允许一个 hunk；同一文件多处修改请拆成多个 change（同一意图内按数组顺序应用）。",
				}
			}
			seenHeader = true
			if err := parseHeader(h, line); err != nil {
				return nil, err
			}
			body++
		case body == 0:
			// hunk 之前的头部行（diff --git / index / --- / +++）忽略。
			if strings.HasPrefix(line, "--- ") || strings.HasPrefix(line, "+++ ") ||
				strings.HasPrefix(line, "diff ") || strings.HasPrefix(line, "index ") ||
				strings.HasPrefix(line, "new file mode") || strings.HasPrefix(line, "deleted file mode") ||
				strings.HasPrefix(line, "old mode") || strings.HasPrefix(line, "new mode") ||
				strings.HasPrefix(line, "similarity index") || strings.HasPrefix(line, "rename from") ||
				strings.HasPrefix(line, "rename to") {
				continue
			}
			return nil, &report.Error{
				Code: report.CodePatchParse, Message: "hunk 之前出现无法识别的行: " + line,
				Hint: "patch 应以 @@ -old,count +new,count @@ 开头（可带 ---/+++ 头）。",
			}
		case strings.HasPrefix(line, `\`):
			// "\ No newline at end of file" 与行内容无关，忽略。
			continue
		case line == "":
			h.Lines = append(h.Lines, schema.Line{Type: schema.LineContext, Content: ""})
		default:
			l, err := parseBodyLine(line)
			if err != nil {
				return nil, err
			}
			h.Lines = append(h.Lines, l)
		}
	}
	if !seenHeader {
		return nil, &report.Error{Code: report.CodePatchParse, Message: "patch 中没有 hunk"}
	}
	if err := checkCounts(h); err != nil {
		return nil, err
	}
	return h, nil
}

func parseHeader(h *Hunk, line string) *report.Error {
	m := hunkHeader.FindStringSubmatch(line)
	if m == nil {
		return &report.Error{
			Code: report.CodePatchParse, Message: "hunk 头格式错误: " + line,
			Hint: "正确形式：@@ -12,6 +12,10 @@（行数省略时按 1 计算，如 @@ -12 +12 @@）。",
		}
	}
	atoi := func(s string, def int) (int, error) {
		if s == "" {
			return def, nil
		}
		return strconv.Atoi(s)
	}
	var err error
	if h.OldStart, err = atoi(m[1], 0); err != nil {
		return &report.Error{Code: report.CodePatchParse, Message: "hunk 头旧起始行不是数字: " + line}
	}
	if h.OldCount, err = atoi(m[2], 1); err != nil {
		return &report.Error{Code: report.CodePatchParse, Message: "hunk 头旧行数不是数字: " + line}
	}
	if h.NewStart, err = atoi(m[3], 0); err != nil {
		return &report.Error{Code: report.CodePatchParse, Message: "hunk 头新起始行不是数字: " + line}
	}
	if h.NewCount, err = atoi(m[4], 1); err != nil {
		return &report.Error{Code: report.CodePatchParse, Message: "hunk 头新行数不是数字: " + line}
	}
	return nil
}

func parseBodyLine(line string) (schema.Line, *report.Error) {
	kind := line[0]
	content := line[1:]
	switch kind {
	case ' ':
		return schema.Line{Type: schema.LineContext, Content: content}, nil
	case '-':
		return schema.Line{Type: schema.LineDel, Content: content}, nil
	case '+':
		return schema.Line{Type: schema.LineAdd, Content: content}, nil
	default:
		return schema.Line{}, &report.Error{
			Code: report.CodePatchParse, Message: fmt.Sprintf("无法识别的 patch 行前缀 %q", string(kind)),
			Hint: "行首必须是空格（context）、-（删除）或 +（新增）。",
		}
	}
}

func checkCounts(h *Hunk) *report.Error {
	oldN, newN := 0, 0
	for _, l := range h.Lines {
		if l.IsContext() || l.IsDel() {
			oldN++
		}
		if l.IsContext() || l.IsAdd() {
			newN++
		}
	}
	if oldN != h.OldCount || newN != h.NewCount {
		return &report.Error{
			Code: report.CodePatchParse,
			Message: fmt.Sprintf("hunk 头声明 -%d,+%d 行，实际 -%d,+%d 行",
				h.OldCount, h.NewCount, oldN, newN),
			Hint: "检查 hunk 头行数或是否漏写行首空格（空 context 行也要保留一个空格或空字符串）。",
		}
	}
	return nil
}
