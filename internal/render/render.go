// Package render 生成单文件、离线可打开的评审 HTML（设计文档 §8）。
//
// 所有链与意图预渲染，切换只改显隐；CSS/JS/diff 数据全部内联。
package render

import (
	"bytes"
	_ "embed"
	"fmt"
	"html/template"
	"strings"

	"intent-review/internal/highlight"
	"intent-review/internal/report"
	"intent-review/internal/schema"
)

//go:embed web/review.html.tmpl
var templateText string

//go:embed web/review.css
var styleText string

//go:embed web/app.js
var scriptText string

// DefaultMaxLines 是“大型评审”降级阈值（设计文档 §8.7）。
const DefaultMaxLines = 5000

// Options 控制渲染。
type Options struct {
	// MaxLines>0 时，总变更行数超过该值触发 <details> 折叠降级。
	MaxLines int
	// Warnings 是外部（validate）提供的诊断，会浮到状态机节点。
	Warnings []report.Error
	// DisableHighlight 关闭语法高亮（调试/性能对照用）。
	DisableHighlight bool
	// SourceRoots 是 VS Code 跳转用的真实源码根，key 为仓库 id（单仓为 ""）。
	// 为空表示不生成链接（避免指向重放快照）。
	SourceRoots map[string]string
}

// Result 是渲染产物与附带诊断。
type Result struct {
	HTML       []byte
	Warnings   []report.Error
	LargeMode  bool
	TotalLines int
}

// Build 渲染整份文档。
func Build(doc schema.Document, opt Options) (Result, error) {
	if opt.MaxLines <= 0 {
		opt.MaxLines = DefaultMaxLines
	}
	var hl *highlight.Highlighter
	if !opt.DisableHighlight {
		hl = highlight.New()
	}
	warns := newWarningIndex(opt.Warnings)

	totalLines := 0
	for _, c := range doc.Chains {
		for _, in := range c.Intents {
			for _, ch := range in.Changes {
				totalLines += len(ch.Lines)
			}
		}
	}
	large := totalLines > opt.MaxLines

	page := pageData{
		Title:      doc.Meta.Title,
		LargeMode:  large,
		TotalLines: totalLines,
		CSS:        template.CSS(styleText),
		JS:         template.JS(scriptText),
	}
	for _, id := range doc.RepoIDs() {
		rv := repoView{ID: id, BaseCommit: doc.RepoBase(id), HeadRef: doc.RepoHead(id)}
		rv.Name = id
		if rv.Name == "" {
			rv.Name = "repo"
		}
		if r, ok := doc.RepoByID(id); ok && strings.TrimSpace(r.Title) != "" {
			rv.Name = r.Title
		}
		page.Repos = append(page.Repos, rv)
	}
	for ci := range doc.Chains {
		c := &doc.Chains[ci]
		cv := chainView{ID: c.ID, Title: c.Title}
		for ii := range c.Intents {
			in := &c.Intents[ii]
			iv := intentView{
				ID: in.ID, Title: in.Title, Summary: in.Summary,
				Rationale: in.BusinessRationale, Risk: in.RiskNotes,
				Number: ii + 1, Total: len(c.Intents),
			}
			for _, k := range displayOrder(in.Changes) {
				ch := &in.Changes[k]
				anchor := fmt.Sprintf("c%d-i%d-ch%d", ci, ii, k+1)
				cv2 := changeView{
					Anchor: anchor, Number: k + 1, File: ch.File, Location: ch.Location,
					Repo: ch.Repo, RepoLabel: repoLabel(doc, ch.Repo), DisplayGroup: ch.DisplayGroup,
					StatePrev: ii, StateCur: ii + 1,
					FileLink:    fileLink(opt.SourceRoots[ch.Repo], ch.File, changeLine(ch.OldStart, ch.NewStart)),
					Explanation: ch.BusinessExplanation, HasExplanation: strings.TrimSpace(ch.BusinessExplanation) != "",
					RenamedFrom: ch.RenamedFrom, IsNewFile: ch.NewFile,
					IsDeleted: ch.Deleted, IsBinary: ch.Binary, IsPatchOnly: len(ch.Patch) > 0 && len(ch.Lines) == 0,
				}
				if !ch.Binary {
					rows, ok := alignRows(*ch, hl)
					iv.Changes = append(iv.Changes, withRows(cv2, rows))
					if !ok && hl != nil {
						warns.add(report.Error{
							Code: report.CodeHighlightFallback, Severity: report.SeverityWarning,
							ChainID: c.ID, IntentID: in.ID, File: ch.File,
							Message: "没有匹配的语法高亮 lexer，已降级为纯文本",
						})
					}
				} else {
					iv.Changes = append(iv.Changes, withRows(cv2, nil))
				}
			}
			warns.decorate(&iv, c.ID, in.ID)
			cv.Intents = append(cv.Intents, iv)
		}
		page.Chains = append(page.Chains, cv)
	}

	var buf bytes.Buffer
	tmpl, err := template.New("review").Parse(templateText)
	if err != nil {
		return Result{}, fmt.Errorf("解析模板失败: %w", err)
	}
	if err := tmpl.Execute(&buf, page); err != nil {
		return Result{}, fmt.Errorf("渲染模板失败: %w", err)
	}
	res := Result{HTML: buf.Bytes(), LargeMode: large, TotalLines: totalLines}
	res.Warnings = warns.flatten()
	if large {
		res.Warnings = append(res.Warnings, report.Error{
			Code: report.CodeLargeReview, Severity: report.SeverityWarning,
			Message: fmt.Sprintf("总变更行数 %d 超过阈值 %d，意图内容默认折叠", totalLines, opt.MaxLines),
		})
	}
	return res, nil
}

func withRows(c changeView, rows []Row) changeView {
	c.Rows = rows
	return c
}

// ---- 模板数据结构 ----

type pageData struct {
	Title      string
	LargeMode  bool
	TotalLines int
	Repos      []repoView
	Chains     []chainView
	CSS        template.CSS
	JS         template.JS
}

// repoView 是侧栏底部展示的仓库信息。
type repoView struct {
	ID         string
	Name       string
	BaseCommit string
	HeadRef    string
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
	Repo           string
	RepoLabel      string
	DisplayGroup   string
	FileLink       template.URL
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

// ---- 诊断索引 ----

type warningIndex struct {
	all   []report.Error
	byKey map[string][]report.Error
	chain map[string][]report.Error
}

func newWarningIndex(in []report.Error) *warningIndex {
	idx := &warningIndex{byKey: map[string][]report.Error{}, chain: map[string][]report.Error{}}
	for _, e := range in {
		idx.add(e)
	}
	return idx
}

func (w *warningIndex) add(e report.Error) {
	w.all = append(w.all, e)
	dst := w.byKey[e.ChainID+"\x00"+e.IntentID]
	w.byKey[e.ChainID+"\x00"+e.IntentID] = append(dst, e)
	if e.IntentID == "" {
		dst := w.chain[e.ChainID]
		w.chain[e.ChainID] = append(dst, e)
	}
}

func (w *warningIndex) decorate(iv *intentView, chainID, intentID string) {
	all := append([]report.Error{}, w.byKey[chainID+"\x00"+intentID]...)
	all = append(all, w.chain[chainID]...)
	for _, e := range all {
		iv.Warnings = append(iv.Warnings, warningView{
			Code: e.Code, Severity: e.Severity, Message: firstNonEmpty(e.Message, e.Hint),
			Repo: e.Repo, File: e.File, Line: e.Line, IsError: !e.Warning(),
		})
	}
	if len(iv.Warnings) > 0 {
		iv.HasWarning = true
		parts := make([]string, 0, len(iv.Warnings))
		for _, wv := range iv.Warnings {
			parts = append(parts, wv.Code+": "+wv.Message)
		}
		iv.WarnTitle = strings.Join(parts, " | ")
	}
}

func (w *warningIndex) flatten() []report.Error { return w.all }

// repoLabel 返回页面上显示的仓库前缀（单仓为空）。
func repoLabel(doc schema.Document, repoID string) string {
	if !doc.MultiRepo() {
		return ""
	}
	return repoID
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
