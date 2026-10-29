package schema

import (
	"fmt"
	"strings"

	"intent-review/internal/report"
)

// CheckStructure 做与仓库无关的结构校验（设计文档 §7.2 的 schema 部分）。
// 返回的诊断里 warning 不阻塞；error 级表示必须修复。
func CheckStructure(doc Document) []report.Error {
	if e := CheckVersion(doc.SchemaVersion); e != nil {
		return []report.Error{*e}
	}
	var errs []report.Error
	add := func(e report.Error) { errs = append(errs, e) }

	if strings.TrimSpace(doc.Meta.Title) == "" {
		add(report.Error{Code: report.CodeSchemaInvalid, Message: "meta.title 为空"})
	}
	if strings.TrimSpace(doc.Meta.BaseCommit) == "" {
		add(report.Error{Code: report.CodeSchemaInvalid, Message: "meta.base_commit 为空", Hint: "fill/validate 需要 base 提交才能重放"})
	}
	if strings.TrimSpace(doc.Meta.HeadRef) == "" {
		add(report.Error{Code: report.CodeSchemaInvalid, Message: "meta.head_ref 为空", Hint: "重放校验的最终状态需要与 head 比对"})
	}
	if len(doc.Chains) == 0 {
		add(report.Error{Code: report.CodeSchemaInvalid, Message: "chains 至少需要一个"})
		return errs
	}

	seenChains := map[string]bool{}
	for ci := range doc.Chains {
		c := &doc.Chains[ci]
		where := func(e report.Error) report.Error {
			if e.ChainID == "" {
				e.ChainID = c.ID
			}
			return e
		}
		if strings.TrimSpace(c.ID) == "" {
			add(where(report.Error{Code: report.CodeSchemaInvalid, Message: fmt.Sprintf("chains[%d].id 为空", ci)}))
		} else if seenChains[c.ID] {
			add(report.Error{Code: report.CodeDuplicateChainID, ChainID: c.ID, Message: fmt.Sprintf("链 id %q 重复", c.ID)})
		}
		seenChains[c.ID] = true
		if strings.TrimSpace(c.Title) == "" {
			add(where(report.Error{Code: report.CodeSchemaInvalid, Message: "链标题为空"}))
		}
		if len(c.Intents) == 0 {
			add(where(report.Error{Code: report.CodeSchemaInvalid, Message: "链内 intents 至少需要一个"}))
		}
		seenIntents := map[string]bool{}
		for ii := range c.Intents {
			checkIntent(c, &c.Intents[ii], ii, seenIntents, add)
		}
	}
	return errs
}

func checkIntent(c *Chain, in *Intent, ii int, seen map[string]bool, add func(report.Error)) {
	wrap := func(e report.Error) report.Error {
		e.ChainID = c.ID
		if e.IntentID == "" {
			e.IntentID = in.ID
		}
		return e
	}
	switch {
	case strings.TrimSpace(in.ID) == "":
		add(wrap(report.Error{Code: report.CodeSchemaInvalid, Message: fmt.Sprintf("intents[%d].id 为空", ii)}))
	case seen[in.ID]:
		add(report.Error{Code: report.CodeDuplicateIntentID, Message: fmt.Sprintf("意图 id %q 在链内重复", in.ID)})
	}
	seen[in.ID] = true
	if strings.TrimSpace(in.Title) == "" {
		add(wrap(report.Error{Code: report.CodeSchemaInvalid, Message: "意图 title 为空"}))
	}
	if strings.TrimSpace(in.Summary) == "" {
		add(wrap(report.Error{Code: report.CodeSchemaInvalid, Message: "意图 summary 为空"}))
	}
	if len(in.Changes) == 0 {
		add(wrap(report.Error{Code: report.CodeSchemaInvalid, Message: "意图 changes 至少需要一个"}))
	}
	for k := range in.Changes {
		checkChangeShape(in, &in.Changes[k], k, wrap, add)
	}
}

func checkChangeShape(in *Intent, ch *Change, k int, wrap func(report.Error) report.Error, add func(report.Error)) {
	w := func(e report.Error) report.Error {
		e = wrap(e)
		i := k
		e.ChangeIndex = &i
		if e.File == "" {
			e.File = ch.File
		}
		return e
	}
	if strings.TrimSpace(ch.File) == "" {
		add(w(report.Error{Code: report.CodeSchemaInvalid, Message: "change.file 为空"}))
	}
	if strings.TrimSpace(ch.Location) == "" {
		add(w(report.Error{Code: report.CodeSchemaInvalid, Message: "change.location 为空"}))
	}
	if strings.TrimSpace(ch.BusinessExplanation) == "" {
		add(w(report.Error{Code: report.CodeMissingExplanation, Message: "business_explanation 为空", Hint: "必须回答“为什么这里要改”，不能留空或写 null"}))
	}
	if ch.Binary {
		return
	}
	if len(ch.Lines) == 0 {
		if len(ch.Patch) == 0 {
			add(w(report.Error{Code: report.CodeSchemaInvalid, Message: "change 既无 lines 也无 patch", Hint: "先运行 intent-review fill 生成 lines"}))
		}
		return
	}
	for li, l := range ch.Lines {
		switch l.Type {
		case LineContext, LineAdd, LineDel:
		default:
			add(w(report.Error{Code: report.CodeSchemaInvalid, Line: li + 1, Message: fmt.Sprintf("非法的行类型 %q（只允许 context/add/del）", l.Type)}))
		}
	}
	checkLineNumbers(ch, w, add)
}

func checkLineNumbers(ch *Change, w func(report.Error) report.Error, add func(report.Error)) {
	hasOldSide := len(ch.LeftLines()) > 0
	hasNewSide := len(ch.RightLines()) > 0
	if ch.NewFile {
		if ch.OldStart != 0 {
			add(w(report.Error{Code: report.CodeContextMismatch, Line: 1, Message: "new_file 的 old_start 必须为 0", Hint: "新增文件没有旧状态一侧"}))
		}
		if hasOldSide {
			add(w(report.Error{Code: report.CodeContextMismatch, Line: 1, Message: "new_file 不允许 context/del 行", Hint: "新增文件的所有行都是 add"}))
		}
	}
	if ch.Deleted {
		if ch.NewStart != 0 {
			add(w(report.Error{Code: report.CodeContextMismatch, Line: 1, Message: "deleted 的 new_start 必须为 0", Hint: "删除文件没有新状态一侧"}))
		}
		if hasNewSide {
			add(w(report.Error{Code: report.CodeContextMismatch, Line: 1, Message: "deleted 不允许 context/add 行", Hint: "删除文件的所有行都是 del"}))
		}
	}
	if !ch.NewFile && hasOldSide && ch.OldStart <= 0 {
		add(w(report.Error{Code: report.CodeSchemaInvalid, Message: "缺少 old_start", Hint: "lines 中的旧状态一侧需要起始行号"}))
	}
	if !ch.Deleted && hasNewSide && ch.NewStart <= 0 {
		add(w(report.Error{Code: report.CodeSchemaInvalid, Message: "缺少 new_start", Hint: "lines 中的新状态一侧需要起始行号"}))
	}
}
