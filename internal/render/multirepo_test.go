package render

import (
	"strings"
	"testing"

	"intent-review/internal/schema"
)

func changeDoc(chainID, intentID string, changes []schema.Change) schema.Document {
	return schema.Document{
		SchemaVersion: schema.SchemaVersion,
		Meta:          schema.Meta{Title: "t", BaseCommit: "abc1234", HeadRef: "def5678"},
		Chains: []schema.Chain{{ID: chainID, Title: "链", Intents: []schema.Intent{{
			ID: intentID, Title: "意图", Summary: "摘要", Changes: changes,
		}}}},
	}
}

func lineChange(file string, order int, group string) schema.Change {
	return schema.Change{
		File: file, Location: "入口", BusinessExplanation: "解释",
		DisplayOrder: order, DisplayGroup: group,
		OldStart: 1, NewStart: 1,
		Lines: []schema.Line{{Type: schema.LineContext, Content: "a"}},
	}
}

// display_order 只决定页面顺序：辅助函数(10) 先于入口(20)，与声明顺序相反。
func TestBuildOrdersChangesByDisplayOrder(t *testing.T) {
	doc := changeDoc("c", "i", []schema.Change{
		lineChange("entry.go", 20, "页面入口"),
		lineChange("helper.go", 10, "辅助函数"),
		lineChange("legacy.go", 0, ""),
	})
	res, err := Build(doc, Options{DisableHighlight: true})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	html := string(res.HTML)
	helper := strings.Index(html, `helper.go`)
	entry := strings.Index(html, `entry.go`)
	legacy := strings.Index(html, `legacy.go`)
	if helper < 0 || entry < 0 || legacy < 0 {
		t.Fatalf("文件未渲染: helper=%d entry=%d legacy=%d", helper, entry, legacy)
	}
	if !(helper < entry && entry < legacy) {
		t.Fatalf("展示顺序应为 helper < entry < legacy，实际 %d, %d, %d", helper, entry, legacy)
	}
	if !strings.Contains(html, `class="group-tag"`) || !strings.Contains(html, "辅助函数") {
		t.Fatalf("display_group 未渲染")
	}
}

// 跨仓：显示仓库标签，文件路径生成 vscode:// 链接（指向真实源码根）。
func TestBuildMultiRepoLabelsAndVSCodeLinks(t *testing.T) {
	doc := changeDoc("c", "i", []schema.Change{
		{Repo: "web", File: "src/a.ts", Location: "入口", BusinessExplanation: "前端", OldStart: 2, NewStart: 5,
			Lines: []schema.Line{{Type: schema.LineAdd, Content: "x"}}},
	})
	doc.Repos = []schema.Repo{{ID: "web", Path: "web", BaseCommit: "abc", HeadRef: "def"}}
	doc.Meta.BaseCommit, doc.Meta.HeadRef = "", ""
	res, err := Build(doc, Options{DisableHighlight: true, SourceRoots: map[string]string{"web": "/home/me/proj/web"}})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	html := string(res.HTML)
	if !strings.Contains(html, `<span class="repo-tag">web</span>`) {
		t.Fatalf("缺少仓库标签")
	}
	if !strings.Contains(html, `vscode://file/home/me/proj/web/src/a.ts:5`) {
		t.Fatalf("缺少 vscode 链接（含行号）")
	}
}

func TestFileLinkEscapesAndSkipsEmptyRoot(t *testing.T) {
	got := string(fileLink("/root/my proj", "a b.go", 7))
	if !strings.Contains(got, "my%20proj") || !strings.HasSuffix(got, "a%20b.go:7") {
		t.Fatalf("fileLink = %q", got)
	}
	if fileLink("", "a.go", 1) != "" {
		t.Fatalf("未指定源码根时不应生成链接")
	}
	if got := string(fileLink("/root", "a.go", 0)); !strings.HasSuffix(got, "a.go") {
		t.Fatalf("line=0 时不应带行号: %q", got)
	}
}

// 单仓文档仍展示 base/head（sidebar 不回归为空）。
func TestBuildSidebarShowsRepoBaseHead(t *testing.T) {
	doc := changeDoc("c", "i", []schema.Change{lineChange("a.go", 0, "")})
	res, err := Build(doc, Options{DisableHighlight: true})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	html := string(res.HTML)
	if !strings.Contains(html, "repo base") || !strings.Contains(html, "abc1234") || !strings.Contains(html, "def5678") {
		t.Fatalf("sidebar 缺 base/head")
	}
}
