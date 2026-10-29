// Package schema 定义意图评审文件的数据模型、JSON Schema 与结构校验。
//
// 契约见设计文档 §4。结构体上的 json tag 即 canonical 落盘字段；
// 未知字段在读取时被忽略（minor 版本升级兼容），fill 的输出会保留它们。
package schema

// 行类型。
const (
	LineContext = "context"
	LineAdd     = "add"
	LineDel     = "del"
)

// SchemaVersion 是当前支持的 schema 版本。
const SchemaVersion = "1.0"

// SupportedMajor 是唯一接受的 major 版本。
const SupportedMajor = 1

// Document 是评审文件根结构。
type Document struct {
	SchemaVersion string  `json:"schema_version"`
	Meta          Meta    `json:"meta"`
	Chains        []Chain `json:"chains"`
}

// Meta 是整份评审的元信息。
type Meta struct {
	Title        string `json:"title"`
	BaseCommit   string `json:"base_commit"`
	HeadRef      string `json:"head_ref"`
	GeneratedBy  string `json:"generated_by,omitempty"`
	SkillVersion string `json:"skill_version,omitempty"`
}

// Chain 是一条意图链。链内意图严格线性，链间状态隔离。
type Chain struct {
	ID         string   `json:"id"`
	Title      string   `json:"title"`
	BaseCommit string   `json:"base_commit,omitempty"`
	Intents    []Intent `json:"intents"`
}

// EffBaseCommit 返回该链实际使用的 base（缺省继承 meta）。
func (c Chain) EffBaseCommit(meta Meta) string {
	if c.BaseCommit != "" {
		return c.BaseCommit
	}
	return meta.BaseCommit
}

// Intent 是链内的一个阶段。
type Intent struct {
	ID                string   `json:"id"`
	Title             string   `json:"title"`
	Summary           string   `json:"summary"`
	BusinessRationale string   `json:"business_rationale,omitempty"`
	RiskNotes         string   `json:"risk_notes,omitempty"`
	Changes           []Change `json:"changes"`
}

// Change 是一个意图内某个文件的一处连续修改。
type Change struct {
	File                string   `json:"file"`
	Location            string   `json:"location"`
	BusinessExplanation string   `json:"business_explanation"`
	OldStart            int      `json:"old_start,omitempty"`
	NewStart            int      `json:"new_start,omitempty"`
	Lines               []Line   `json:"lines,omitempty"`
	Patch               []string `json:"patch,omitempty"`
	RenamedFrom         string   `json:"renamed_from,omitempty"`
	Binary              bool     `json:"binary,omitempty"`
	Deleted             bool     `json:"deleted,omitempty"`
	NewFile             bool     `json:"new_file,omitempty"`
}

// Line 是一条结构化对齐行。
type Line struct {
	Type    string `json:"type"`
	Content string `json:"content"`
}

// IsContext / IsAdd / IsDel 供对齐算法与重放使用。
func (l Line) IsContext() bool { return l.Type == LineContext }
func (l Line) IsAdd() bool     { return l.Type == LineAdd }
func (l Line) IsDel() bool     { return l.Type == LineDel }

// LeftLines 返回该 change 在旧状态一侧的内容（context+del），按出现顺序。
func (c Change) LeftLines() []string { return collect(c.Lines, false) }

// RightLines 返回该 change 在新状态一侧的内容（context+add），按出现顺序。
func (c Change) RightLines() []string { return collect(c.Lines, true) }

func collect(lines []Line, newSide bool) []string {
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		if l.IsContext() || (newSide && l.IsAdd()) || (!newSide && l.IsDel()) {
			out = append(out, l.Content)
		}
	}
	return out
}

// NeedsFill 表示 change 仍是 patch-only（尚未 fill 成 canonical lines）。
func (c Change) NeedsFill() bool {
	return !c.Binary && len(c.Patch) > 0 && len(c.Lines) == 0
}
