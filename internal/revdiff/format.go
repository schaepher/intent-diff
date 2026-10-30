package revdiff

import (
	"fmt"
	"strings"
)

// 维度名称（人类可读）。
var dimensionLabels = map[string]string{
	"title":       "标题",
	"summary":     "意图摘要",
	"files":       "文件集合",
	"order":       "文件顺序",
	"group":       "函数分组",
	"explanation": "解释内容",
	"diff":        "代码内容",
}

var dimensionOrder = []string{"title", "summary", "files", "order", "group", "explanation", "diff"}

// FormatHuman 生成可直接贴给评审者的变更说明。
func (r Report) FormatHuman() string {
	if r.Empty() {
		return "没有意图级变化"
	}
	var b strings.Builder
	for _, ic := range r.Intents {
		b.WriteString(intentHead(ic))
		b.WriteString("\n")
		for _, f := range ic.FilesAdded {
			fmt.Fprintf(&b, "  + %s\n", f)
		}
		for _, f := range ic.FilesRemoved {
			fmt.Fprintf(&b, "  - %s\n", f)
		}
		if len(ic.OrderBefore) != 0 || len(ic.OrderAfter) != 0 {
			fmt.Fprintf(&b, "  顺序: %s → %s\n", strings.Join(ic.OrderBefore, ", "), strings.Join(ic.OrderAfter, ", "))
		}
		if len(ic.ChangedChanges) != 0 {
			fmt.Fprintf(&b, "  变更项: %s\n", strings.Join(ic.ChangedChanges, "、"))
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func intentHead(ic IntentChange) string {
	label := map[string]string{"added": "新增", "removed": "删除", "modified": "修改"}[ic.Status]
	head := fmt.Sprintf("意图 [%s] %s %s", ic.ChainID, ic.IntentID, label)
	if ic.Title != "" {
		head += "（" + ic.Title + "）"
	}
	if len(ic.Dimensions) > 0 {
		labels := make([]string, 0, len(ic.Dimensions))
		for _, d := range ic.Dimensions {
			labels = append(labels, dimensionLabels[d])
		}
		head += ": " + strings.Join(labels, "、")
	}
	return head
}
