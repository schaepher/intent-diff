// Package validate 执行链条重放校验（设计文档 §7）。
//
// 校验 = 结构校验 + 逐链重放 + 最终状态与 head 比对；错误带
// chain/intent/change/仓库/文件/行号，供 agent 精确修复。
package validate

import (
	"fmt"
	"sort"

	"intent-review/internal/filefilter"
	"intent-review/internal/replay"
	"intent-review/internal/report"
	"intent-review/internal/schema"
)

// Options 是校验的可选参数。
type Options struct {
	// Files 是覆盖检查的排除列表（nil 表示取文档 file_filter 合并内置默认）。
	// 它只影响「哪些文件不需要意图覆盖」，不改变重放与最终状态比对。
	Files *filefilter.List
}

// Run 校验整份文档。返回的 error 只表示用法/IO 级失败（退出码 2）。
func Run(doc schema.Document, repoDir string, opts Options) (report.Result, error) {
	errs := schema.CheckStructure(doc)
	if report.HasBlocking(errs) {
		return report.NewResult(errs), nil
	}
	if unfilled := unfilledChanges(doc); len(unfilled) > 0 {
		return report.NewResult(append(errs, unfilled...)), nil
	}
	project, err := replay.OpenProject(repoDir, doc.Repos)
	if err != nil {
		return report.Result{}, err
	}
	heads := map[string]string{}
	covBases := map[string]string{}
	for _, id := range project.IDs() {
		repo, err := project.Repo(id)
		if err != nil {
			return report.Result{}, err
		}
		if heads[id], err = repo.Resolve(doc.RepoHead(id)); err != nil {
			return report.Result{}, fmt.Errorf("解析仓库 %s 的 head_ref 失败: %w", repoLabel(id), err)
		}
		if covBases[id], err = repo.Resolve(doc.RepoBase(id)); err != nil {
			return report.Result{}, fmt.Errorf("解析仓库 %s 的 base_commit 失败: %w", repoLabel(id), err)
		}
	}
	touched := map[string]map[string]bool{}
	for _, id := range project.IDs() {
		touched[id] = map[string]bool{}
	}
	stopped := false
	for ci := range doc.Chains {
		chain := &doc.Chains[ci]
		bases := map[string]string{}
		for _, id := range project.IDs() {
			repo, err := project.Repo(id)
			if err != nil {
				return report.Result{}, err
			}
			base, err := repo.Resolve(chain.EffBaseFor(doc, id))
			if err != nil {
				return report.Result{}, fmt.Errorf("解析链 %s 在仓库 %s 的 base_commit 失败: %w", chain.ID, repoLabel(id), err)
			}
			bases[id] = base
		}
		es := replay.NewEngines(project, bases)
		chainErrs, stop := replayChain(es, chain)
		errs = append(errs, chainErrs...)
		for _, id := range project.IDs() {
			for _, p := range es.Engine(id).Touched() {
				touched[id][p] = true
			}
		}
		if stop {
			stopped = true
			continue
		}
		for _, id := range project.IDs() {
			repo, err := project.Repo(id)
			if err != nil {
				return report.Result{}, err
			}
			errs = append(errs, compareHead(repo, heads[id], id, chain, es.Engine(id))...)
		}
	}
	files := opts.Files
	if files == nil {
		files = filefilter.Default()
	}
	res := report.NewResult(errs)
	res.ExcludeRules = files.Patterns()
	// 重放全部成功时，再检查 base→head 的变更是否都被某条链覆盖：
	// 漏掉整个文件、未声明的重命名都会在这里暴露（设计文档 §7.2「遗漏变更」）。
	// 被排除列表跳过的路径记录到 Excluded，供页面说明“本次评审范围”。
	if !stopped {
		coverageErrs, excluded, err := checkCoverage(project, covBases, heads, touched, files)
		if err != nil {
			return report.Result{}, err
		}
		res.Errors = append(res.Errors, coverageErrs...)
		if len(coverageErrs) > 0 {
			res.OK = false
		}
		res.Excluded = excluded
	}
	return res, nil
}

func repoLabel(id string) string {
	if id == "" {
		return "（单仓）"
	}
	return id
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
				if !ch.NeedsFill() {
					continue
				}
				errs = append(errs, report.Error{
					Code: report.CodeSchemaInvalid, ChainID: chain.ID, IntentID: in.ID,
					ChangeIndex: report.Index(k), Repo: ch.Repo, File: ch.File,
					Message: "change 仍含 patch 且没有 lines",
					Hint:    "先运行 intent-review fill <file.json> --repo <repo> -o <filled.json>，再校验填充结果。",
				})
			}
		}
	}
	return errs
}

// checkCoverage 要求每个仓库 base→head 的变更路径都被至少一条链触及；
// 命中排除列表的路径（如 *_test.go）跳过（README 实现约定 §10）。
func checkCoverage(project *replay.Project, bases, heads map[string]string, touched map[string]map[string]bool, files *filefilter.List) ([]report.Error, []report.Excluded, error) {
	var errs []report.Error
	var excluded []report.Excluded
	for _, id := range project.IDs() {
		repo, err := project.Repo(id)
		if err != nil {
			return nil, nil, err
		}
		paths, err := repo.ChangedPaths(bases[id], heads[id])
		if err != nil {
			return nil, nil, fmt.Errorf("读取仓库 %s 的 base..head 变更列表失败: %w", repoLabel(id), err)
		}
		sort.Strings(paths)
		for _, p := range paths {
			if touched[id][p] {
				continue
			}
			if rule, skip := files.MatchReason(p); skip {
				excluded = append(excluded, report.Excluded{Repo: id, File: p, Rule: rule})
				continue
			}
			errs = append(errs, report.Error{
				Code: report.CodeFinalStateMismatch, Repo: id, File: p,
				Message: "该文件在 base..head 之间有变更，但没有被任何意图覆盖",
				Hint:    "补一个意图/change 覆盖它；重命名请用 renamed_from 声明（否则旧路径会表现为未覆盖的删除）。若属于测试/第三方目录，可用 --exclude 或配置文件排除。",
			})
		}
	}
	return errs, excluded, nil
}

func replayChain(es *replay.Engines, chain *schema.Chain) ([]report.Error, bool) {
	var errs []report.Error
	for ii := range chain.Intents {
		in := &chain.Intents[ii]
		for k := range in.Changes {
			ch := &in.Changes[k]
			ctx := replay.Ctx{ChainID: chain.ID, IntentID: in.ID, ChangeIndex: k}
			if ch.Binary {
				errs = append(errs, report.Error{
					Code: report.CodeBinaryChangeSkipped, Severity: report.SeverityWarning,
					ChainID: chain.ID, IntentID: in.ID, ChangeIndex: report.Index(k), Repo: ch.Repo, File: ch.File,
					Message: "binary change 不参与重放校验（仅渲染说明）",
				}.WithChange(k))
				_ = es.Apply(ctx, ch)
				continue
			}
			if err := es.Apply(ctx, ch); err != nil {
				errs = append(errs, *err)
				return errs, true
			}
		}
	}
	return errs, false
}

func compareHead(repo *replay.Repo, head, repoID string, chain *schema.Chain, eng *replay.Engine) []report.Error {
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
				Code: report.CodeRepoUnavailable, ChainID: chain.ID, Repo: repoID, File: p,
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
				Code: report.CodeFinalStateMismatch, ChainID: chain.ID, Repo: repoID, File: p, Message: msg,
				Hint: "链内意图未完整覆盖该文件的变更（或遗漏了 new_file/deleted 标记）。",
			})
			continue
		}
		if !wantExists {
			continue
		}
		if e := firstLineDiff(chain.ID, repoID, p, wantLines, got.Lines, head); e != nil {
			errs = append(errs, *e)
		}
	}
	return errs
}

func firstLineDiff(chainID, repoID, file string, want, got []string, head string) *report.Error {
	n := len(want)
	if len(got) < n {
		n = len(got)
	}
	for i := 0; i < n; i++ {
		if want[i] != got[i] {
			return &report.Error{
				Code: report.CodeFinalStateMismatch, ChainID: chainID, Repo: repoID, File: file, Line: i + 1,
				Expected: want[i], Actual: got[i],
				Hint: fmt.Sprintf("最终状态必须等于 head（%s）；检查意图是否遗漏/多出该行变更。", report.ShortSHA(head)),
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
			Code: report.CodeFinalStateMismatch, ChainID: chainID, Repo: repoID, File: file, Line: line,
			Expected: expected, Actual: actual,
			Message: fmt.Sprintf("重放结果共 %d 行，head 共 %d 行", len(got), len(want)),
			Hint:    fmt.Sprintf("最终状态必须等于 head（%s）；检查是否遗漏或多出变更。", report.ShortSHA(head)),
		}
	}
	return nil
}

func existWord(exists bool) string {
	if exists {
		return "存在"
	}
	return "不存在"
}
