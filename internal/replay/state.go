package replay

import (
	"fmt"

	"intent-review/internal/report"
	"intent-review/internal/schema"
)

// File 是重放状态下某个文件的内容。Exists=false 表示文件不存在。
type File struct {
	Lines  []string
	Exists bool
}

// Ctx 定位一条诊断所属的链/意图/change。
type Ctx struct {
	ChainID     string
	IntentID    string
	ChangeIndex int
}

// Engine 维护一条链的重放状态，文件内容按需从 base 提交懒加载。
type Engine struct {
	repo  *Repo
	base  string
	state map[string]File
	// touched 记录被 change 触及过的路径（含 renamed_from 的来源），
	// 最终状态比对只需要这些文件。
	touched map[string]bool
	// unverified 记录只由 binary change 触及的路径：重放无法校验内容，
	// 最终状态比对必须跳过它们（否则会误报 FINAL_STATE_MISMATCH）。
	unverified map[string]bool
}

// NewEngine 建立从 base 提交开始的空状态。
func NewEngine(repo *Repo, base string) *Engine {
	return &Engine{
		repo:       repo,
		base:       base,
		state:      map[string]File{},
		touched:    map[string]bool{},
		unverified: map[string]bool{},
	}
}

// State 返回当前状态快照（只读；值语义复制）。
func (e *Engine) State() map[string]File {
	out := make(map[string]File, len(e.state))
	for k, v := range e.state {
		out[k] = File{Lines: append([]string(nil), v.Lines...), Exists: v.Exists}
	}
	return out
}

// Touched 返回被触及的仓库相对路径（排序不稳定，调用方自行排序）。
func (e *Engine) Touched() []string {
	out := make([]string, 0, len(e.touched))
	for p := range e.touched {
		out = append(out, p)
	}
	return out
}

// Unverified 返回无法校验内容的路径（binary change）。
func (e *Engine) Unverified() []string {
	out := make([]string, 0, len(e.unverified))
	for p := range e.unverified {
		out = append(out, p)
	}
	return out
}

// File 返回当前状态中的文件（未加载过的从 base 读取）。
func (e *Engine) File(ctx Ctx, p string) (File, *report.Error) {
	if f, ok := e.state[p]; ok {
		return f, nil
	}
	lines, exists, err := e.repo.ReadFile(e.base, p)
	if err != nil {
		return File{}, &report.Error{
			Code: report.CodeRepoUnavailable, ChainID: ctx.ChainID, IntentID: ctx.IntentID,
			File: p, Message: err.Error(), Hint: "确认 --repo 指向保存该提交的仓库",
		}
	}
	f := File{Lines: lines, Exists: exists}
	e.state[p] = f
	return f, nil
}

// Apply 应用一个 change 并推进状态；失败时返回精确诊断且不修改状态。
func (e *Engine) Apply(ctx Ctx, ch *schema.Change) *report.Error {
	if ch.Binary {
		e.touched[ch.File] = true
		e.unverified[ch.File] = true
		return nil
	}
	delete(e.unverified, ch.File)
	if err := e.applyRename(ctx, ch); err != nil {
		return err
	}
	oldSide, newSide := ch.LeftLines(), ch.RightLines()
	file, ferr := e.File(ctx, ch.File)
	if ferr != nil {
		return ferr
	}
	e.touched[ch.File] = true

	idx, oldLen, err := locate(ctx, e.base, ch, file, oldSide)
	if err != nil {
		return err
	}
	if err := verifyContents(ctx, ch, file, idx, oldSide); err != nil {
		return err
	}
	if !ch.Deleted {
		if want := idx + 1; ch.NewStart != want {
			return &report.Error{
				Code: report.CodeContextMismatch, ChainID: ctx.ChainID, IntentID: ctx.IntentID,
				ChangeIndex: report.Index(ctx.ChangeIndex), File: ch.File, Line: ch.NewStart,
				Message: fmt.Sprintf("new_start 与重放位置不一致：期望 %d，实际 %d", want, ch.NewStart),
				Hint:    "new_start 是变更块在 State(i) 中的起始行号；同一意图内同文件多处修改按数组顺序推进行号。",
			}
		}
	}
	lines := append([]string(nil), file.Lines[:idx]...)
	lines = append(lines, newSide...)
	lines = append(lines, file.Lines[idx+oldLen:]...)
	switch {
	case ch.Deleted:
		e.state[ch.File] = File{Exists: false}
	case ch.NewFile:
		e.state[ch.File] = File{Lines: lines, Exists: true}
	default:
		e.state[ch.File] = File{Lines: lines, Exists: file.Exists || len(lines) > 0}
	}
	return nil
}

func (e *Engine) applyRename(ctx Ctx, ch *schema.Change) *report.Error {
	if ch.RenamedFrom == "" {
		return nil
	}
	fail := func(msg, hint string) *report.Error {
		return &report.Error{
			Code: report.CodeRenameMismatch, ChainID: ctx.ChainID, IntentID: ctx.IntentID,
			ChangeIndex: report.Index(ctx.ChangeIndex), File: ch.File, Message: msg, Hint: hint,
		}
	}
	if ch.NewFile || ch.Deleted {
		return fail("renamed_from 不能与 new_file/deleted 同时使用", "重命名用 renamed_from + 目标路径的 change")
	}
	from, err := e.File(ctx, ch.RenamedFrom)
	if err != nil {
		return err
	}
	if !from.Exists {
		return fail(fmt.Sprintf("重命名来源 %q 在 State 中不存在", ch.RenamedFrom), "确认 renamed_from 是上一状态中的路径")
	}
	if to, ok := e.state[ch.File]; ok && to.Exists {
		return fail(fmt.Sprintf("重命名目标 %q 已存在", ch.File), "目标路径不能已存在于上一状态")
	}
	if to, err := e.File(ctx, ch.File); err == nil && to.Exists {
		return fail(fmt.Sprintf("重命名目标 %q 已存在于 base", ch.File), "目标路径不能已存在于上一状态")
	} else if err != nil {
		return err
	}
	delete(e.state, ch.RenamedFrom)
	e.state[ch.File] = from
	e.touched[ch.RenamedFrom] = true
	e.touched[ch.File] = true
	return nil
}

// locate 计算变更块在当前状态中的位置：
//   - new_file：从 0 开始（整文件新增）
//   - 纯插入（旧侧为空）：old_start 表示“插入到第 N 行之后”，N 可为 0
//   - 其它：old_start 为 1 基起始行
func locate(ctx Ctx, base string, ch *schema.Change, file File, oldSide []string) (idx, oldLen int, err *report.Error) {
	oldLen = len(oldSide)
	fail := func(msg, hint string) *report.Error {
		return &report.Error{
			Code: report.CodeContextMismatch, ChainID: ctx.ChainID, IntentID: ctx.IntentID,
			ChangeIndex: report.Index(ctx.ChangeIndex), File: ch.File, Line: ch.OldStart,
			Message: msg, Hint: hint,
		}
	}
	if ch.NewFile {
		if file.Exists {
			return 0, 0, fail(fmt.Sprintf("new_file 目标 %q 在 State 中已存在", ch.File), "新增文件不能已存在；若只是追加内容请去掉 new_file")
		}
		return 0, 0, nil
	}
	if !file.Exists {
		return 0, 0, &report.Error{
			Code: report.CodeFileNotFound, ChainID: ctx.ChainID, IntentID: ctx.IntentID,
			ChangeIndex: report.Index(ctx.ChangeIndex), File: ch.File,
			Message: fmt.Sprintf("文件 %q 在 base（%s）中不存在", ch.File, report.ShortSHA(base)),
			Hint:    "若是新增文件请设置 new_file: true；若由前序意图创建，请检查前序 change 是否已成功应用。",
		}
	}
	if ch.Deleted {
		if len(file.Lines) == 0 {
			return 0, 0, fail(fmt.Sprintf("文件 %q 在 State 中为空，无需删除", ch.File), "确认前序意图是否已删除该文件")
		}
		if ch.OldStart != 1 || oldLen != len(file.Lines) {
			return 0, 0, fail("删除文件必须覆盖全部内容", fmt.Sprintf("deleted 的 old_start 必须为 1 且左列行数等于文件行数 %d", len(file.Lines)))
		}
		return 0, oldLen, nil
	}
	if oldLen == 0 {
		if ch.OldStart < 0 || ch.OldStart > len(file.Lines) {
			return 0, 0, fail(fmt.Sprintf("old_start=%d 越界（文件共 %d 行）", ch.OldStart, len(file.Lines)), "纯插入的 old_start 表示插入位置之前已有的行数")
		}
		return ch.OldStart, 0, nil
	}
	if ch.OldStart < 1 || ch.OldStart-1+oldLen > len(file.Lines) {
		return 0, 0, fail(fmt.Sprintf("变更块超出文件范围（old_start=%d，左列 %d 行，文件共 %d 行）", ch.OldStart, oldLen, len(file.Lines)), "检查行号是否按 State(i-1) 计算")
	}
	return ch.OldStart - 1, oldLen, nil
}

func verifyContents(ctx Ctx, ch *schema.Change, file File, idx int, oldSide []string) *report.Error {
	for i, want := range oldSide {
		pos := idx + i
		got := ""
		if pos < len(file.Lines) {
			got = file.Lines[pos]
		}
		if got == want {
			continue
		}
		line := ch.OldStart + i
		return &report.Error{
			Code: report.CodeContextMismatch, ChainID: ctx.ChainID, IntentID: ctx.IntentID,
			ChangeIndex: report.Index(ctx.ChangeIndex), File: ch.File, Line: line,
			Expected: want, Actual: got,
			Hint: "意图的 before 与上一状态的 after 不一致：检查意图顺序、行号或 change 内容（已入库的代码必须在当前意图中作为 context 出现）。",
		}
	}
	return nil
}
