package replay

import (
	"fmt"
	"path/filepath"
	"strings"

	"intent-review/internal/report"
	"intent-review/internal/schema"
)

// Project 是评审涉及的仓库集合：文档未声明 repos 时退化为一个隐含仓库（id 为空串）。
// root 是项目根（CLI --repo），repos[].path 相对它解析。
type Project struct {
	repos map[string]*Repo
	order []string
}

// OpenProject 打开项目声明中的所有仓库。
func OpenProject(root string, decl []schema.Repo) (*Project, error) {
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	if len(decl) == 0 {
		r, err := Open(root)
		if err != nil {
			return nil, err
		}
		return &Project{repos: map[string]*Repo{"": r}, order: []string{""}}, nil
	}
	p := &Project{repos: make(map[string]*Repo, len(decl))}
	for _, d := range decl {
		dir := root
		if strings.TrimSpace(d.Path) != "" {
			if filepath.IsAbs(d.Path) {
				dir = d.Path
			} else {
				dir = filepath.Join(root, d.Path)
			}
		}
		r, err := Open(dir)
		if err != nil {
			return nil, fmt.Errorf("仓库 %s（%s）: %w", d.ID, dir, err)
		}
		p.repos[d.ID] = r
		p.order = append(p.order, d.ID)
	}
	return p, nil
}

// IDs 返回仓库 id（声明顺序；单仓为 [""]）。
func (p *Project) IDs() []string {
	out := make([]string, len(p.order))
	copy(out, p.order)
	return out
}

// Repo 返回指定仓库；未声明时报错。
func (p *Project) Repo(id string) (*Repo, error) {
	r, ok := p.repos[id]
	if !ok {
		return nil, fmt.Errorf("仓库 %q 未在文档 repos 中声明", id)
	}
	return r, nil
}

// Engines 管理一条链在多个仓库上的重放状态（每仓一个 Engine）。
type Engines struct {
	project *Project
	byID    map[string]*Engine
}

// NewEngines 为项目中的每个仓库建立从各自 base 开始的重放引擎。
func NewEngines(p *Project, bases map[string]string) *Engines {
	es := &Engines{project: p, byID: make(map[string]*Engine, len(p.order))}
	for _, id := range p.order {
		es.byID[id] = NewEngine(p.repos[id], bases[id])
	}
	return es
}

// Engine 返回某仓库的重放引擎（未声明的 id 返回 nil）。
func (es *Engines) Engine(id string) *Engine { return es.byID[id] }

// Apply 按 change.repo 路由到对应仓库的引擎，并把仓库标注到诊断上。
func (es *Engines) Apply(ctx Ctx, ch *schema.Change) *report.Error {
	eng, ok := es.byID[ch.Repo]
	if !ok {
		return &report.Error{
			Code: report.CodeRepoUnavailable, ChainID: ctx.ChainID, IntentID: ctx.IntentID,
			ChangeIndex: report.Index(ctx.ChangeIndex), Repo: ch.Repo, File: ch.File,
			Message: fmt.Sprintf("change 引用了未声明的仓库 %q", ch.Repo),
			Hint:    "在文档 repos[] 中声明该仓库（id/path/base_commit），或去掉 change.repo",
		}
	}
	e := eng.Apply(ctx, ch)
	if e != nil && ch.Repo != "" {
		e.Repo = ch.Repo
	}
	return e
}
