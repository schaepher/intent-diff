// Package validate 执行链条重放校验（设计文档 §7）。
//
// 校验 = 结构校验 + 逐链重放 + 最终状态与 head 比对；错误带
// chain/intent/change/文件/行号，供 agent 精确修复。
package validate

import (
	"fmt"
	"sort"
	"strings"

	"intent-review/internal/replay"
	"intent-review/internal/report"
	"intent-review/internal/schema"
)

// Run 校验整份文档。返回的 error 只表示用法/IO 级失败（退出码 2）。
func Run(doc schema.Document, repoDir string) (report.Result, error) {
	errs := schema.CheckStructure(doc)
	if hasBlocking(errs) {
		return report.NewResult(errs), nil
	}
	if unfilled := unfilledChanges(doc); len(unfilled) > 0 {
		return report.NewResult(append(errs, unfilled...)), nil
	}
	repo, err := replay.Open(repoDir)
	if err != nil {
		return report.Result{}, err
	}
	head, err := repo.Resolve(doc.Meta.HeadRef)
	if err != nil {
		return report.Result{}, fmt.Errorf("解析 meta.head_ref 失败: %w", err)
	}
	touched := map[string]bool{}
	stopped := false
	for ci := range doc.Chains {
		chain := &doc.Chains[ci]
		base, err := repo.Resolve(chain.EffBaseCommit(doc.Meta))
		if err != nil {
			return report.Result{}, fmt.Errorf("解析链 %s 的 base_commit 失败: %w", chain.ID, err)
		}
		eng := replay.NewEngine(repo, base)
		chainErrs, stop := replayChain(eng, chain)
		errs = append(errs, chainErrs...)
		for _, p := range eng.Touched() {
			touched[p] = true
		}
		if stop {
			stopped = true
			continue
		}
		errs = append(errs, compareHead(repo, head, chain, eng)...)
	}
	// 重放全部成功时，再检查 base→head 的变更是否都被某条链覆盖：
	// 漏掉整个文件、未声明的重命名都会在这里暴露（设计文档 §7.2「遗漏变更」）。
	if !stopped {
		coverageErrs, err := checkCoverage(repo, doc.Meta.BaseCommit, head, touched)
		if err != nil {
			return report.Result{}, err
		}
		errs = append(errs, coverageErrs...)
	}
	return report.NewResult(errs), nil
}

// unfilledChanges 找出仍是 patch-only 的 change：validate 只接受 canonical lines，
// 保持职责单一（设计文档 §5.2），填充交给 fill。
func unfilledChanges(doc schema.Document) []report.Error {
	var errs []report.Error
	for ci := range doc.Chains {
		chain := &doc.Chains[ci]
		for ii := range chain.Intents {
			in := &chain.Intents[ii]
			for k := range in.Changes {
				ch := &in.Changes[k]
				if ch.Binary || len(ch.Patch) == 0 || len(ch.Lines) > 0 {
					continue
				}
				errs = append(errs, report.Error{
					Code: report.CodeSchemaInvalid, ChainID: chain.ID, IntentID: in.ID,
					ChangeIndex: intPtr(k), File: ch.File,
					Message: "change 仍含 patch 且没有 lines",
					Hint:    "先运行 intent-review fill <file.json> --repo <repo> -o <filled.json>，再校验填充结果。",
				})
			}
		}
	}
	return errs
}

// checkCoverage 要求 base→head 的每个变更路径都被至少一条链触及。
func checkCoverage(repo *replay.Repo, baseRef, head string, touched map[string]bool) ([]report.Error, error) {
	base, err := repo.Resolve(baseRef)
	if err != nil {
		return nil, fmt.Errorf("解析 meta.base_commit 失败: %w", err)
	}
	paths, err := repo.ChangedPaths(base, head)
	if err != nil {
		return nil, fmt.Errorf("读取 base..head 变更列表失败: %w", err)
	}
	sort.Strings(paths)
	var errs []report.Error
	for _, p := range paths {
		if touched[p] {
			continue
		}
		errs = append(errs, report.Error{
			Code: report.CodeFinalStateMismatch, File: p,
			Message: "该文件在 base..head 之间有变更，但没有被任何意图覆盖",
			Hint:    "补一个意图/change 覆盖它；重命名请用 renamed_from 声明（否则旧路径会表现为未覆盖的删除）。",
		})
	}
	return errs, nil
}

func replayChain(eng *replay.Engine, chain *schema.Chain) ([]report.Error, bool) {
	var errs []report.Error
	for ii := range chain.Intents {
		in := &chain.Intents[ii]
		for k := range in.Changes {
			ch := &in.Changes[k]
			ctx := replay.Ctx{ChainID: chain.ID, IntentID: in.ID, ChangeIndex: k}
			if ch.Binary {
				errs = append(errs, report.Error{
					Code: report.CodeBinaryChangeSkipped, Severity: report.SeverityWarning,
					ChainID: chain.ID, IntentID: in.ID, ChangeIndex: intPtr(k), File: ch.File,
					Message: "binary change 不参与重放校验（仅渲染说明）",
				}.WithChange(k))
				_ = eng.Apply(ctx, ch)
				continue
			}
			if err := eng.Apply(ctx, ch); err != nil {
				errs = append(errs, *err)
				return errs, true
			}
		}
	}
	return errs, false
}

func compareHead(repo *replay.Repo, head string, chain *schema.Chain, eng *replay.Engine) []report.Error {
	paths := eng.Touched()
	sort.Strings(paths)
	state := eng.State()
	skip := map[string]bool{}
	for _, p := range eng.Unverified() {
		skip[p] = true
	}
	var errs []report.Error
	for _, p := range paths {
		if skip[p] {
			continue
		}
		wantLines, wantExists, err := repo.ReadFile(head, p)
		if err != nil {
			errs = append(errs, report.Error{
				Code: report.CodeRepoUnavailable, ChainID: chain.ID, File: p,
				Message: err.Error(), Hint: "确认 head_ref 指向包含该文件的提交",
			})
			continue
		}
		got, ok := state[p]
		if !ok {
			got = replay.File{}
		}
		if got.Exists != wantExists {
			msg := fmt.Sprintf("重放结果%s，但 head 中%s", existWord(got.Exists), existWord(wantExists))
			errs = append(errs, report.Error{
				Code: report.CodeFinalStateMismatch, ChainID: chain.ID, File: p, Message: msg,
				Hint: "链内意图未完整覆盖该文件的变更（或遗漏了 new_file/deleted 标记）。",
			})
			continue
		}
		if !wantExists {
			continue
		}
		if e := firstLineDiff(chain.ID, p, wantLines, got.Lines, head); e != nil {
			errs = append(errs, *e)
		}
	}
	return errs
}

func firstLineDiff(chainID, file string, want, got []string, head string) *report.Error {
	n := len(want)
	if len(got) < n {
		n = len(got)
	}
	for i := 0; i < n; i++ {
		if want[i] != got[i] {
			return &report.Error{
				Code: report.CodeFinalStateMismatch, ChainID: chainID, File: file, Line: i + 1,
				Expected: want[i], Actual: got[i],
				Hint: fmt.Sprintf("最终状态必须等于 head（%s）；检查意图是否遗漏/多出该行变更。", shortSHA(head)),
			}
		}
	}
	if len(got) != len(want) {
		line := n + 1
		expected, actual := "", ""
		if len(want) > n {
			expected = want[n]
		}
		if len(got) > n {
			actual = got[n]
		}
		return &report.Error{
			Code: report.CodeFinalStateMismatch, ChainID: chainID, File: file, Line: line,
			Expected: expected, Actual: actual,
			Message: fmt.Sprintf("重放结果共 %d 行，head 共 %d 行", len(got), len(want)),
			Hint:    fmt.Sprintf("最终状态必须等于 head（%s）；检查是否遗漏或多出变更。", shortSHA(head)),
		}
	}
	return nil
}

func hasBlocking(errs []report.Error) bool {
	for _, e := range errs {
		if !e.Warning() {
			return true
		}
	}
	return false
}

func intPtr(i int) *int { return &i }

func existWord(exists bool) string {
	if exists {
		return "存在"
	}
	return "不存在"
}

func shortSHA(sha string) string {
	if len(sha) > 8 {
		return sha[:8]
	}
	return strings.TrimSpace(sha)
}
