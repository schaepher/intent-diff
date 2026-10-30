// Package report 定义 intent-review 的结构化错误模型与输出格式。
//
// 错误码与设计文档 §10.1 保持一致；Severity 区分 error/warning
// （warning 不改变退出码，但会渲染到状态机节点上）。
package report

import (
	"encoding/json"
	"fmt"
	"strings"
)

// 错误码（设计文档 §10.1）。
const (
	CodeSchemaInvalid       = "SCHEMA_INVALID"
	CodeSchemaVersion       = "SCHEMA_VERSION_UNSUPPORTED"
	CodeMissingExplanation  = "MISSING_BUSINESS_EXPLANATION"
	CodeDuplicateIntentID   = "DUPLICATE_INTENT_ID"
	CodeDuplicateChainID    = "DUPLICATE_CHAIN_ID"
	CodeDuplicateRepoID     = "DUPLICATE_REPO_ID"
	CodePatchParse          = "PATCH_PARSE_ERROR"
	CodePatchContext        = "PATCH_CONTEXT_MISMATCH"
	CodeContextMismatch     = "CONTEXT_MISMATCH"
	CodeFinalStateMismatch  = "FINAL_STATE_MISMATCH"
	CodeFileNotFound        = "FILE_NOT_FOUND"
	CodeRenameMismatch      = "RENAME_MISMATCH"
	CodeHighlightFallback   = "HIGHLIGHT_FALLBACK"
	CodeLargeReview         = "LARGE_REVIEW"
	CodeRepoUnavailable     = "REPO_UNAVAILABLE"
	CodeBinaryChangeSkipped = "BINARY_CHANGE_SKIPPED"
)

// 严重级别。
const (
	SeverityError   = "error"
	SeverityWarning = "warning"
)

// Error 是一条可定位的诊断。字段顺序即 --json 输出顺序。
type Error struct {
	Code     string `json:"code"`
	Severity string `json:"severity,omitempty"`
	ChainID  string `json:"chain_id,omitempty"`
	IntentID string `json:"intent_id,omitempty"`
	// Repo 是多仓评审中的仓库 id（单仓评审为空）。
	Repo string `json:"repo,omitempty"`
	// ChangeIndex 是 0 基下标（与 JSON 内的数组一致）；人类可读输出显示 1 基。
	ChangeIndex *int   `json:"change_index,omitempty"`
	File        string `json:"file,omitempty"`
	Line        int    `json:"line,omitempty"`
	Expected    string `json:"expected,omitempty"`
	Actual      string `json:"actual,omitempty"`
	Hint        string `json:"hint,omitempty"`
	// Message 用于没有行级期望/实际的诊断（schema 错误、警告）。
	Message string `json:"message,omitempty"`
}

// Warning 返回该诊断是否为警告。
func (e Error) Warning() bool { return e.Severity == SeverityWarning }

// Index 返回指向 i 的指针，用于 Error.ChangeIndex（0 基下标）。
func Index(i int) *int { return &i }

// ShortSHA 截断提交 ID，便于错误信息阅读。
func ShortSHA(sha string) string {
	sha = strings.TrimSpace(sha)
	if len(sha) > 8 {
		return sha[:8]
	}
	return sha
}

// HasBlocking 判断诊断中是否存在 error 级（warning 不阻塞）。
func HasBlocking(errs []Error) bool {
	for _, e := range errs {
		if !e.Warning() {
			return true
		}
	}
	return false
}

// WithChange 复制一份并补上 change 下标。
func (e Error) WithChange(idx int) Error {
	e.ChangeIndex = Index(idx)
	return e
}

// Result 是一次命令的结果。Errors 为空且（可选）Warnings 不阻塞成功。
type Result struct {
	OK       bool    `json:"ok"`
	Errors   []Error `json:"errors"`
	Warnings []Error `json:"warnings,omitempty"`
}

// NewResult 由错误列表构造结果；warning 级不参与 OK 判定。
func NewResult(errs []Error) Result {
	res := Result{OK: true, Errors: []Error{}}
	for _, e := range errs {
		if e.Warning() {
			res.Warnings = append(res.Warnings, e)
			continue
		}
		res.OK = false
		res.Errors = append(res.Errors, e)
	}
	return res
}

// JSON 返回 --json 契约的字节序列（带缩进，便于 agent 阅读）。
func (r Result) JSON() ([]byte, error) { return json.MarshalIndent(r, "", "  ") }

// FormatHuman 按设计文档 §7.4 生成人类可读诊断（写 stderr）。
func FormatHuman(e Error) string {
	var b strings.Builder
	head := make([]string, 0, 4)
	if e.Severity != "" && e.Severity != SeverityError {
		head = append(head, strings.ToUpper(e.Severity))
	}
	if e.ChainID != "" {
		head = append(head, "["+e.ChainID+"]")
	}
	if e.IntentID != "" {
		head = append(head, e.IntentID)
	}
	if e.ChangeIndex != nil {
		head = append(head, fmt.Sprintf("change-%d", *e.ChangeIndex+1))
	}
	if e.File != "" {
		loc := e.File
		if e.Repo != "" {
			loc = e.Repo + ":" + loc
		}
		if e.Line > 0 {
			loc = fmt.Sprintf("%s:%d", loc, e.Line)
		}
		head = append(head, loc)
	}
	if len(head) == 0 {
		head = append(head, e.Code)
	} else {
		head = append(head, "("+e.Code+")")
	}
	b.WriteString(strings.Join(head, " "))
	b.WriteString("\n")
	if e.Message != "" {
		fmt.Fprintf(&b, "  %s\n", e.Message)
	}
	if e.Expected != "" || e.Actual != "" {
		fmt.Fprintf(&b, "  期望: %q\n", e.Expected)
		fmt.Fprintf(&b, "  实际: %q\n", e.Actual)
	}
	if e.Hint != "" {
		fmt.Fprintf(&b, "  提示: %s\n", e.Hint)
	}
	return strings.TrimRight(b.String(), "\n")
}
