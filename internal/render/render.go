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
	// ExcludeRules/Excluded 是本次覆盖检查生效的过滤信息，页面展示评审范围。
	ExcludeRules []string
	Excluded     []report.Excluded
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
		Title:        doc.Meta.Title,
		LargeMode:    large,
		TotalLines:   totalLines,
		ExcludeRules: opt.ExcludeRules,
		Excluded:     scopeViews(opt.Excluded),
		CSS:          template.CSS(styleText),
		JS:           template.JS(scriptText),
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
		refs := newChainRefs(c, ci)
		cv := chainView{ID: c.ID, Title: c.Title}
		for ii := range c.Intents {
			in := &c.Intents[ii]
			plan := planGroups(in.Changes)
			iv := intentView{
				ID: in.ID, Title: in.Title, Summary: in.Summary,
				Rationale: in.BusinessRationale, Risk: in.RiskNotes,
				Number: ii + 1, Total: len(c.Intents),
			}
			prevGroup := "\x00initial"
			for _, k := range displayOrder(in.Changes) {
				ch := &in.Changes[k]
				cv2 := buildChange(doc, *ch, k, ci, ii, plan, prevGroup, opt.SourceRoots, refs)
				prevGroup = strings.TrimSpace(ch.DisplayGroup)
				if !ch.Binary {
					rows, ok := alignRows(*ch, hl)
					withLineLinks(rows, opt.SourceRoots[ch.Repo], ch.File)
					iv.Changes = append(iv.Changes, withRows(cv2, rows))
					if !ok && hl != nil {
						warns.add(report.Error{
							Code: report.CodeHighlightFallback, Severity: report.SeverityWarning,
							ChainID: c.ID, IntentID: in.ID, Repo: ch.Repo, File: ch.File,
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

// buildChange 组装单个 change 的视图（展示顺序、分组、链接、引用）。
func buildChange(doc schema.Document, ch schema.Change, k, ci, ii int, plan groupPlan, prevGroup string, roots map[string]string, refs *chainRefs) changeView {
	group := strings.TrimSpace(ch.DisplayGroup)
	root := roots[ch.Repo]
	var link template.URL
	missing := false
	if !ch.Deleted {
		// 删除文件在本地已不存在，不生成可能误导的源码链接。
		link = fileLink(root, ch.File, changeLine(ch.OldStart, ch.NewStart))
		missing = link != "" && sourceFileMissing(root, ch.File)
	}
	return changeView{
		Anchor:         fmt.Sprintf("c%d-i%d-ch%d", ci, ii, k+1),
		Number:         k + 1,
		ChangeID:       ch.ID,
		Repo:           ch.Repo,
		RepoLabel:      repoLabel(doc, ch.Repo),
		DisplayGroup:   group,
		GroupStart:     group != "" && group != prevGroup,
		GroupName:      group,
		GroupNote:      plan.notes[group],
		GroupAnchor:    groupAnchor(ci, ii, plan, group),
		FileLink:       link,
		FileMissing:    missing,
		Refs:           refs.resolve(ch.References),
		StatePrev:      ii,
		StateCur:       ii + 1,
		File:           ch.File,
		Location:       ch.Location,
		Explanation:    ch.BusinessExplanation,
		HasExplanation: strings.TrimSpace(ch.BusinessExplanation) != "",
		RenamedFrom:    ch.RenamedFrom,
		IsNewFile:      ch.NewFile,
		IsDeleted:      ch.Deleted,
		IsBinary:       ch.Binary,
		IsPatchOnly:    len(ch.Patch) > 0 && len(ch.Lines) == 0,
	}
}

func withRows(c changeView, rows []Row) changeView {
	c.Rows = rows
	return c
}
