package render

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"intent-review/internal/report"
	"intent-review/internal/schema"
)

// 页面需要展示本次生效的排除规则与被跳过的文件。
func TestBuildShowsReviewScope(t *testing.T) {
	doc := changeDoc("c", "i", []schema.Change{lineChange("a.go", 0, "")})
	doc.Repos = []schema.Repo{{ID: "web", Path: "web", BaseCommit: "a", HeadRef: "b"}}
	doc.Meta.BaseCommit, doc.Meta.HeadRef = "", ""
	res, err := Build(doc, Options{
		DisableHighlight: true,
		ExcludeRules:     []string{"*.md", "*.test.tsx"},
		Excluded: []report.Excluded{
			{Repo: "web", File: "docs/g.md", Rule: "*.md"},
			{Repo: "web", File: "src/page.test.tsx", Rule: "*.test.tsx"},
		},
	})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	html := string(res.HTML)
	for _, want := range []string{
		"评审范围：排除 2 条规则，跳过 2 个文件",
		"<code>*.md</code>", "<code>*.test.tsx</code>",
		"web:docs/g.md", "web:src/page.test.tsx",
		`<span class="scope-rule">*.md</span>`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("缺少 %q", want)
		}
	}
}

// 无过滤信息时不渲染评审范围区块（避免空面板）。
func TestBuildHidesScopeWhenNoRules(t *testing.T) {
	doc := changeDoc("c", "i", []schema.Change{lineChange("a.go", 0, "")})
	res, err := Build(doc, Options{DisableHighlight: true})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if strings.Contains(string(res.HTML), `<details class="scope">`) {
		t.Fatalf("没有排除规则时不应渲染 scope 面板")
	}
}

// 分组标题 + 分组说明 + 锚点；独立分组各自成段。
func TestBuildGroupHeadersAndNotes(t *testing.T) {
	entry := lineChange("entry.go", 10, "页面入口")
	entry.DisplayGroupNote = "从入口开始读，先看调用方。"
	caller := lineChange("caller.go", 20, "页面入口")
	helper := lineChange("helper.go", 30, "辅助函数")
	doc := changeDoc("c", "i", []schema.Change{entry, caller, helper})
	res, err := Build(doc, Options{DisableHighlight: true})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	html := string(res.HTML)
	if strings.Count(html, `class="group-head"`) != 2 {
		t.Fatalf("应有两个分组标题，html 片段: %s", html)
	}
	if !strings.Contains(html, `id="group-c0-i0-g0"`) || !strings.Contains(html, `id="group-c0-i0-g1"`) {
		t.Fatalf("分组锚点缺失")
	}
	if !strings.Contains(html, "从入口开始读") {
		t.Fatalf("分组说明未渲染")
	}
	// 入口组在前、辅助函数组在后（display_order）。
	if strings.Index(html, "entry.go") > strings.Index(html, "helper.go") {
		t.Fatalf("分组/展示顺序错误")
	}
}

// references：按稳定 id / 下标指向 change，按 #group 指向分组；缺失引用退化为纯文本。
func TestBuildResolvesReferences(t *testing.T) {
	doc := changeDoc("c", "i1", []schema.Change{
		{
			ID: "ch-001", File: "entry.go", Location: "调用方", BusinessExplanation: "调用 Helper，实现在后面展开",
			References: []string{"i1/ch-002", "i1/1", "i2#辅助函数", "i9/ch-9"},
			OldStart:   1, NewStart: 1, Lines: []schema.Line{{Type: schema.LineContext, Content: "a"}},
		},
		{ID: "ch-002", File: "helper.go", Location: "实现", BusinessExplanation: "e", OldStart: 1, NewStart: 1,
			Lines: []schema.Line{{Type: schema.LineContext, Content: "b"}}},
	})
	doc.Chains[0].Intents = append(doc.Chains[0].Intents, schema.Intent{
		ID: "i2", Title: "实现", Summary: "s",
		Changes: []schema.Change{{ID: "ch-003", File: "impl.go", Location: "实现", BusinessExplanation: "e",
			DisplayGroup: "辅助函数", OldStart: 1, NewStart: 1,
			Lines: []schema.Line{{Type: schema.LineContext, Content: "c"}}}},
	})
	res, err := Build(doc, Options{DisableHighlight: true})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	html := string(res.HTML)
	for _, want := range []string{
		`href="#change-c0-i0-ch2"`, // 稳定 id → 同意图第二个 change
		`href="#change-c0-i0-ch1"`, // 1 基下标
		`href="#group-c0-i1-g0"`,   // i2 的分组锚点
		`<span class="ref-missing" title="未找到引用目标">i9/ch-9</span>`,
		`data-change-id="ch-001"`, `data-change-id="ch-002"`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("缺少 %q", want)
		}
	}
}

// 源码跳转：删除文件不生成链接；本地缺文件给出提示；新状态行号可精确跳转。
func TestBuildSourceLinksEdgeCases(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.go"), []byte("package main\nvar X = 1\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	doc := changeDoc("c", "i", []schema.Change{
		{
			File: "a.go", Location: "入口", BusinessExplanation: "e",
			OldStart: 1, NewStart: 1,
			Lines: []schema.Line{
				{Type: schema.LineContext, Content: "package main"},
				{Type: schema.LineAdd, Content: "var X = 1"},
			},
		},
		{
			File: "gone.go", Location: "删除", BusinessExplanation: "删除旧文件", Deleted: true,
			OldStart: 1, Lines: []schema.Line{{Type: schema.LineDel, Content: "x"}},
		},
		{
			File: "missing.go", Location: "缺失", BusinessExplanation: "e",
			OldStart: 1, NewStart: 1, Lines: []schema.Line{{Type: schema.LineAdd, Content: "y"}},
		},
	})
	res, err := Build(doc, Options{DisableHighlight: true, SourceRoots: map[string]string{"": root}})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	html := string(res.HTML)
	if !strings.Contains(html, `class="ln-link" href="vscode://file`+filepath.ToSlash(root)+`/a.go:2"`) {
		t.Fatalf("缺少右列行号跳转")
	}
	if strings.Contains(html, "gone.go") && strings.Contains(html, "vscode://file"+filepath.ToSlash(root)+"/gone.go") {
		t.Fatalf("删除文件不应生成源码链接")
	}
	if !strings.Contains(html, `class="file-link is-missing"`) {
		t.Fatalf("缺失文件应标记 is-missing")
	}
	if !strings.Contains(html, "未找到该文件") {
		t.Fatalf("缺失文件缺少说明")
	}
}
