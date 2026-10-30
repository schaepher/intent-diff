package render

import (
	"html/template"

	"intent-review/internal/report"
	"intent-review/internal/schema"
)

// ---- 模板数据结构 ----

type pageData struct {
	Title        string
	LargeMode    bool
	TotalLines   int
	Repos        []repoView
	Chains       []chainView
	ExcludeRules []string
	Excluded     []scopeView
	CSS          template.CSS
	JS           template.JS
}

// repoView 是侧栏底部展示的仓库信息。
type repoView struct {
	ID         string
	Name       string
	BaseCommit string
	HeadRef    string
}

// scopeView 是一条被排除规则跳过、不在本次评审范围内的路径。
type scopeView struct {
	Repo string
	File string
	Rule string
}

type chainView struct {
	ID      string
	Title   string
	Intents []intentView
}

type intentView struct {
	ID         string
	Title      string
	Summary    string
	Rationale  string
	Risk       string
	Number     int
	Total      int
	HasWarning bool
	WarnTitle  string
	Warnings   []warningView
	Changes    []changeView
}

type warningView struct {
	Code     string
	Severity string
	Message  string
	Repo     string
	File     string
	Line     int
	IsError  bool
}

type changeView struct {
	Anchor         string
	Number         int
	ChangeID       string
	Repo           string
	RepoLabel      string
	DisplayGroup   string
	GroupStart     bool
	GroupName      string
	GroupNote      string
	GroupAnchor    string
	FileLink       template.URL
	FileMissing    bool
	Refs           []refView
	StatePrev      int
	StateCur       int
	File           string
	Location       string
	Explanation    string
	HasExplanation bool
	RenamedFrom    string
	IsNewFile      bool
	IsDeleted      bool
	IsBinary       bool
	IsPatchOnly    bool
	Rows           []Row
}

// refView 是 change 上的“继续阅读”链接（已解析）。
type refView struct {
	Href  string
	Label string
}

func scopeViews(excluded []report.Excluded) []scopeView {
	out := make([]scopeView, 0, len(excluded))
	for _, e := range excluded {
		out = append(out, scopeView{Repo: e.Repo, File: e.File, Rule: e.Rule})
	}
	return out
}

// repoLabel 返回页面上显示的仓库前缀（单仓为空）。
func repoLabel(doc schema.Document, repoID string) string {
	if !doc.MultiRepo() {
		return ""
	}
	return repoID
}
