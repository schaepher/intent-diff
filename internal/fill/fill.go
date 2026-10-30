// Package fill 把骨架 JSON 中的 patch 填充为结构化 lines 与行号，
// 同时按意图顺序重放状态（设计文档 §5.1）。
package fill

import (
	"fmt"
	"sort"

	"intent-review/internal/diffpatch"
	"intent-review/internal/replay"
	"intent-review/internal/report"
	"intent-review/internal/schema"
)

// Run 填充并返回新的 JSON 字节与结果。返回的 error 只表示用法/IO 级失败。
func Run(doc schema.Document, raw []byte, repoDir string) ([]byte, report.Result, error) {
	errs := schema.CheckStructure(doc)
	if report.HasBlocking(errs) {
		return nil, report.NewResult(errs), nil
	}
	project, err := replay.OpenProject(repoDir, doc.Repos)
	if err != nil {
		return nil, report.Result{}, err
	}
	updates := map[key]changeUpdate{}
	for ci := range doc.Chains {
		chain := &doc.Chains[ci]
		bases := map[string]string{}
		for _, id := range project.IDs() {
			repo, err := project.Repo(id)
			if err != nil {
				return nil, report.Result{}, err
			}
			base, err := repo.Resolve(chain.EffBaseFor(doc, id))
			if err != nil {
				return nil, report.Result{}, fmt.Errorf("解析链 %s 在仓库 %s 的 base_commit 失败: %w", chain.ID, id, err)
			}
			bases[id] = base
		}
		es := replay.NewEngines(project, bases)
		chainErrs, ok := fillChain(ci, es, chain, updates)
		errs = append(errs, chainErrs...)
		if !ok {
			continue
		}
	}
	res := report.NewResult(errs)
	if !res.OK {
		return nil, res, nil
	}
	out, err := emit(raw, updates)
	if err != nil {
		return nil, report.Result{}, err
	}
	return out, res, nil
}

func fillChain(chainIdx int, es *replay.Engines, chain *schema.Chain, updates map[key]changeUpdate) ([]report.Error, bool) {
	var errs []report.Error
	for ii := range chain.Intents {
		in := &chain.Intents[ii]
		for k := range in.Changes {
			ch := &in.Changes[k]
			ctx := replay.Ctx{ChainID: chain.ID, IntentID: in.ID, ChangeIndex: k}
			eng := es.Engine(ch.Repo)
			if eng == nil {
				errs = append(errs, report.Error{
					Code: report.CodeSchemaInvalid, ChainID: chain.ID, IntentID: in.ID,
					ChangeIndex: report.Index(k), Repo: ch.Repo, File: ch.File,
					Message: fmt.Sprintf("change 引用了未声明的仓库 %q", ch.Repo),
					Hint:    "在文档 repos[] 中声明该仓库，或去掉 change.repo",
				})
				return errs, false
			}
			if ch.Binary {
				_ = eng.Apply(ctx, ch)
				continue
			}
			hadPatch := len(ch.Patch) > 0
			if hadPatch {
				msg, cerr := applyPatch(eng, ctx, ch)
				if cerr != nil {
					errs = append(errs, *cerr)
					return errs, false
				}
				if msg != nil {
					errs = append(errs, *msg)
				}
			}
			if len(ch.Lines) == 0 {
				errs = append(errs, report.Error{
					Code: report.CodeSchemaInvalid, ChainID: chain.ID, IntentID: in.ID,
					ChangeIndex: report.Index(k), Repo: ch.Repo, File: ch.File,
					Message: "change 既无 lines 也无 patch",
					Hint:    "为每个 change 提供 patch（unified diff 行数组）或完整的 lines/行号。",
				})
				return errs, false
			}
			if aerr := eng.Apply(ctx, ch); aerr != nil {
				if hadPatch {
					aerr.Code = report.CodePatchContext
				}
				errs = append(errs, *aerr)
				return errs, false
			}
			updates[key{chainIdx, ii, k}] = changeUpdate{
				OldStart: ch.OldStart, NewStart: ch.NewStart,
				Lines: append([]schema.Line(nil), ch.Lines...),
			}
		}
	}
	return errs, true
}

// applyPatch 解析 patch 并回填 lines/行号；返回非致命警告。
// 若 patch 的上下文在 old_start 处不匹配但该块在文件中唯一出现，则按
// 实际位置回填并给出警告（上下文不足/行号过期时仍可修复，且不掩盖内容错误）。
func applyPatch(eng *replay.Engine, ctx replay.Ctx, ch *schema.Change) (*report.Error, *report.Error) {
	h, perr := diffpatch.Parse(ch.Patch)
	if perr != nil {
		perr.ChainID, perr.IntentID = ctx.ChainID, ctx.IntentID
		withChange := perr.WithChange(ctx.ChangeIndex)
		withChange.Repo, withChange.File = ch.Repo, ch.File
		return nil, &withChange
	}
	if ch.NewFile && h.OldStart != 0 {
		return nil, &report.Error{
			Code: report.CodePatchParse, ChainID: ctx.ChainID, IntentID: ctx.IntentID,
			ChangeIndex: report.Index(ctx.ChangeIndex), Repo: ch.Repo, File: ch.File,
			Message: "new_file 的 patch hunk 头应为 @@ -0,0 +1,N @@",
		}
	}
	ch.Lines = h.Lines
	ch.OldStart = h.OldStart
	ch.NewStart = replay.NewStartFor(h.OldStart, h.OldCount, ch.Deleted)
	var warn *report.Error
	if !ch.NewFile && !ch.Deleted && ch.RenamedFrom == "" && len(ch.LeftLines()) > 0 {
		if relocated := tryRelocate(eng, ctx, ch); relocated != nil {
			warn = relocated
		}
	}
	return warn, nil
}

// tryRelocate 在 old_start 处上下文不匹配时，寻找左列内容的唯一出现位置。
func tryRelocate(eng *replay.Engine, ctx replay.Ctx, ch *schema.Change) *report.Error {
	file, err := eng.File(ctx, ch.File)
	if err != nil || !file.Exists {
		return nil
	}
	left := ch.LeftLines()
	if matchesAt(file.Lines, ch.OldStart-1, left) {
		return nil
	}
	positions := findAll(file.Lines, left)
	if len(positions) != 1 {
		return nil
	}
	pos := positions[0]
	e := report.Error{
		Code: report.CodePatchContext, Severity: report.SeverityWarning,
		ChainID: ctx.ChainID, IntentID: ctx.IntentID, ChangeIndex: report.Index(ctx.ChangeIndex), Repo: ch.Repo, File: ch.File,
		Message: fmt.Sprintf("patch 上下文在 old_start=%d 处不匹配，但该块在 State 中唯一出现于第 %d 行，已按实际位置回填",
			ch.OldStart, pos+1),
		Hint: "建议让 agent 用上一状态重新生成 patch（行号随前序 change 推进）。",
	}
	ch.OldStart = pos + 1
	ch.NewStart = ch.OldStart
	return &e
}

func matchesAt(lines []string, idx int, want []string) bool {
	if idx < 0 || idx+len(want) > len(lines) {
		return false
	}
	for i, w := range want {
		if lines[idx+i] != w {
			return false
		}
	}
	return true
}

func findAll(lines, want []string) []int {
	var out []int
	for i := 0; i+len(want) <= len(lines); i++ {
		if matchesAt(lines, i, want) {
			out = append(out, i)
		}
	}
	sort.Ints(out)
	return out
}
