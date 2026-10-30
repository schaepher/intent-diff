// Package schema 定义意图评审文件的数据模型、JSON Schema 与结构校验。
//
// 契约见设计文档 §4。结构体上的 json tag 即 canonical 落盘字段；
// 未知字段在读取时被忽略（minor 版本升级兼容），fill 的输出会保留它们。
package schema

import "strings"

// 行类型。
const (
	LineContext = "context"
	LineAdd     = "add"
	LineDel     = "del"
)

// SchemaVersion 是当前支持的 schema 版本。
const SchemaVersion = "1.1"

// SupportedMajor 是唯一接受的 major 版本。
const SupportedMajor = 1

// Document 是评审文件根结构。
type Document struct {
	SchemaVersion string `json:"schema_version"`
	Meta          Meta   `json:"meta"`
	// Repos 声明跨仓评审涉及的仓库；为空时退化为单个隐含仓库（--repo 指向它）。
	Repos      []Repo            `json:"repos,omitempty"`
	FileFilter *FileFilterConfig `json:"file_filter,omitempty"`
	Chains     []Chain           `json:"chains"`
}

// Repo 是多仓评审中的一个仓库。path 相对项目根（--repo），base/head 缺省继承 meta。
type Repo struct {
	ID         string `json:"id"`
	Title      string `json:"title,omitempty"`
	Path       string `json:"path,omitempty"`
	BaseCommit string `json:"base_commit,omitempty"`
	HeadRef    string `json:"head_ref,omitempty"`
	// SourceRoot 是 VS Code 跳转使用的真实源码根（可为绝对路径或相对项目根）；
	// 渲染时指向真实 checkout，而不是重放用的临时快照。
	SourceRoot string `json:"source_root,omitempty"`
}

// FileFilterConfig 是文档内的统一过滤配置（与 .intent-review.json 的 file_filter 同构）。
type FileFilterConfig struct {
	Exclude   []string `json:"exclude,omitempty"`
	Unexclude []string `json:"unexclude,omitempty"`
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
	ID          string            `json:"id"`
	Title       string            `json:"title"`
	BaseCommit  string            `json:"base_commit,omitempty"`
	BaseCommits map[string]string `json:"base_commits,omitempty"`
	Intents     []Intent          `json:"intents"`
}

// EffBaseFor 返回该链在某仓库上实际使用的 base：
// chain.base_commits[repo] > chain.base_commit > repos[].base_commit > meta.base_commit。
func (c Chain) EffBaseFor(d Document, repoID string) string {
	if v, ok := c.BaseCommits[repoID]; ok && strings.TrimSpace(v) != "" {
		return v
	}
	if strings.TrimSpace(c.BaseCommit) != "" {
		return c.BaseCommit
	}
	return d.RepoBase(repoID)
}

// RepoIDs 返回需要重放的仓库 id（单仓文档返回 [""]）。
func (d Document) RepoIDs() []string {
	if len(d.Repos) == 0 {
		return []string{""}
	}
	ids := make([]string, 0, len(d.Repos))
	for _, r := range d.Repos {
		ids = append(ids, r.ID)
	}
	return ids
}

// MultiRepo 表示文档是否声明了 repos（跨仓评审）。
func (d Document) MultiRepo() bool { return len(d.Repos) > 0 }

// RepoByID 按 id 查找仓库声明。
func (d Document) RepoByID(id string) (Repo, bool) {
	for _, r := range d.Repos {
		if r.ID == id {
			return r, true
		}
	}
	return Repo{}, false
}

// RepoBase 返回某仓库的缺省 base 引用（repos[].base_commit 优先，其次 meta.base_commit）。
func (d Document) RepoBase(repoID string) string {
	if r, ok := d.RepoByID(repoID); ok && strings.TrimSpace(r.BaseCommit) != "" {
		return r.BaseCommit
	}
	return d.Meta.BaseCommit
}

// RepoHead 返回某仓库的缺省 head 引用（repos[].head_ref 优先，其次 meta.head_ref）。
func (d Document) RepoHead(repoID string) string {
	if r, ok := d.RepoByID(repoID); ok && strings.TrimSpace(r.HeadRef) != "" {
		return r.HeadRef
	}
	return d.Meta.HeadRef
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
	// Repo 是多仓文档中 change 所属仓库 id；单仓文档必须为空。
	Repo                string   `json:"repo,omitempty"`
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
	// DisplayOrder/DisplayGroup 只影响页面展示顺序与分组，不参与重放。
	// 数值小的先展示（未设置=0 排在已设置之后）；同值按声明顺序。
	DisplayOrder int    `json:"display_order,omitempty"`
	DisplayGroup string `json:"display_group,omitempty"`
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
