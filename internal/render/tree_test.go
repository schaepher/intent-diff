package render

import (
	"strings"
	"testing"

	"intent-review/internal/schema"
)

// treeFixture：c1 下 i1 → {i2, i3}（分支），c2 的 j1 跨链挂在 i2 下。
func treeFixture() schema.Document {
	change := func(file string) []schema.Change {
		return []schema.Change{{
			File: file, Location: "x", BusinessExplanation: "e", OldStart: 1, NewStart: 1,
			Lines: []schema.Line{{Type: schema.LineContext, Content: "a"}},
		}}
	}
	return schema.Document{
		SchemaVersion: schema.SchemaVersion,
		Meta:          schema.Meta{Title: "树", BaseCommit: "a", HeadRef: "b"},
		Chains: []schema.Chain{
			{ID: "c1", Title: "限流", Intents: []schema.Intent{
				{ID: "i1", Title: "入口", Summary: "先看入口", Changes: change("a.go")},
				{ID: "i2", Title: "实现", Summary: "主流程", Parent: "c1/i1", Changes: change("b.go")},
				{ID: "i3", Title: "分支 B", Summary: "另一条路径", Parent: "c1/i1", Changes: change("c.go")},
			}},
			{ID: "c2", Title: "日志", Intents: []schema.Intent{
				{ID: "j1", Title: "审计", Summary: "跨链子意图", Parent: "c1/i2", Changes: change("d.go")},
			}},
		},
	}
}

func TestBuildIntentTreeStructure(t *testing.T) {
	res, err := Build(treeFixture(), Options{DisableHighlight: true})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	html := string(res.HTML)
	for _, want := range []string{
		`data-tree="1"`, "全局意图树", "4 个意图 · 1 个根 · 1 处分支",
		`<section class="tree-section" id="intent-tree" data-tree-section hidden>`,
		`class="tree-canvas"`, "自上而下",
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("缺少 %q", want)
		}
	}
	if n := strings.Count(html, `class="tree-node"`); n != 4 {
		t.Fatalf("tree-node 数量 = %d, want 4", n)
	}
	// i1 有 2 个孩子；i2 有 1 个跨链孩子 → 两个 tree-children 列表。
	if n := strings.Count(html, `<ul class="tree-children">`); n != 2 {
		t.Fatalf("tree-children 列表 = %d, want 2", n)
	}
	for _, want := range []string{`href="#c1/i1/1"`, `href="#c1/i2/1"`, `href="#c2/j1/1"`} {
		if !strings.Contains(html, want) {
			t.Fatalf("缺少深链 %q", want)
		}
	}
	// 跨链子节点带链标题；分支节点顺序按声明顺序（i2 在 i3 前）。
	i2, i3 := strings.Index(html, ">实现<"), strings.Index(html, ">分支 B<")
	if i2 < 0 || i3 < 0 || i2 > i3 {
		t.Fatalf("子节点顺序错误: i2=%d i3=%d", i2, i3)
	}
	j1 := strings.Index(html, ">审计<")
	if j1 < 0 || j1 < i2 {
		t.Fatalf("跨链子节点未挂在父节点之后: %d", j1)
	}
	if !strings.Contains(html, "日志 · 意图 1") {
		t.Fatalf("跨链节点缺少链标题")
	}
}

// 父意图缺失/环内节点仍要展示（标 Orphan），不能让页面无声丢意图。
func TestBuildIntentTreeOrphanNodes(t *testing.T) {
	doc := treeFixture()
	doc.Chains[0].Intents = append(doc.Chains[0].Intents, schema.Intent{
		ID: "i9", Title: "缺父", Summary: "s", Parent: "nope", Changes: doc.Chains[0].Intents[0].Changes,
	})
	res, err := Build(doc, Options{DisableHighlight: true})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	html := string(res.HTML)
	if !strings.Contains(html, "父意图未找到") {
		t.Fatalf("未标记缺失父意图")
	}
	if !strings.Contains(html, "5 个意图 · 2 个根") {
		t.Fatalf("根数量应包含孤儿节点")
	}
}

func TestTreeHrefEscapesIDs(t *testing.T) {
	if got := treeHref("c 1", "i#2"); got != "#c%201/i%232/1" {
		t.Fatalf("treeHref = %q", got)
	}
}
