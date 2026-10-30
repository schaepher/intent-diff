package revdiff

import (
	"strings"
	"testing"

	"intent-review/internal/schema"
)

func doc(changes []schema.Change) schema.Document {
	return schema.Document{
		SchemaVersion: schema.SchemaVersion,
		Meta:          schema.Meta{Title: "t", BaseCommit: "a", HeadRef: "b"},
		Chains: []schema.Chain{{ID: "c1", Title: "链", Intents: []schema.Intent{{
			ID: "i1", Title: "意图", Summary: "摘要", Changes: changes,
		}}}},
	}
}

func ch(file, explanation string, order int, group string) schema.Change {
	return schema.Change{
		File: file, Location: "入口", BusinessExplanation: explanation,
		DisplayOrder: order, DisplayGroup: group,
		OldStart: 1, NewStart: 1,
		Lines: []schema.Line{{Type: schema.LineContext, Content: "x"}},
	}
}

func TestCompareIdentical(t *testing.T) {
	a := doc([]schema.Change{ch("a.go", "e", 1, "g")})
	if rep := Compare(a, a); !rep.Empty() {
		t.Fatalf("相同文档不应有变化: %+v", rep)
	}
}

func TestCompareExplanationOnly(t *testing.T) {
	before := doc([]schema.Change{ch("a.go", "旧解释", 1, "g")})
	after := doc([]schema.Change{ch("a.go", "新解释", 1, "g")})
	rep := Compare(before, after)
	if len(rep.Intents) != 1 {
		t.Fatalf("rep = %+v", rep)
	}
	ic := rep.Intents[0]
	if ic.Status != "modified" || strings.Join(ic.Dimensions, ",") != "explanation" {
		t.Fatalf("ic = %+v", ic)
	}
	human := rep.FormatHuman()
	if !strings.Contains(human, "解释内容") || !strings.Contains(human, "意图 [c1] i1 修改") {
		t.Fatalf("human = %s", human)
	}
}

func TestCompareOrderAndGroup(t *testing.T) {
	before := doc([]schema.Change{ch("a.go", "e", 10, "入口"), ch("b.go", "e", 20, "辅助")})
	after := doc([]schema.Change{ch("a.go", "e", 20, "入口"), ch("b.go", "e", 10, "入口")})
	rep := Compare(before, after)
	if len(rep.Intents) != 1 {
		t.Fatalf("rep = %+v", rep)
	}
	dims := strings.Join(rep.Intents[0].Dimensions, ",")
	if !strings.Contains(dims, "order") || !strings.Contains(dims, "group") {
		t.Fatalf("dimensions = %s", dims)
	}
	if strings.Join(rep.Intents[0].OrderAfter, ",") != "b.go @ 入口,a.go @ 入口" {
		t.Fatalf("order after = %v", rep.Intents[0].OrderAfter)
	}
}

func TestCompareFilesAndIntents(t *testing.T) {
	before := doc([]schema.Change{ch("a.go", "e", 0, "")})
	after := doc([]schema.Change{ch("a.go", "e", 0, ""), ch("b.go", "e", 0, "")})
	rep := Compare(before, after)
	if len(rep.Intents) != 1 || rep.Intents[0].FilesAdded[0] != "b.go @ 入口" {
		t.Fatalf("rep = %+v", rep)
	}
	if !strings.Contains(strings.Join(rep.Intents[0].Dimensions, ","), "files") {
		t.Fatalf("dimensions = %v", rep.Intents[0].Dimensions)
	}

	empty := schema.Document{SchemaVersion: schema.SchemaVersion, Meta: schema.Meta{Title: "t", BaseCommit: "a", HeadRef: "b"}}
	rep = Compare(empty, after)
	if len(rep.Intents) != 1 || rep.Intents[0].Status != "added" {
		t.Fatalf("added = %+v", rep.Intents)
	}
	rep = Compare(after, empty)
	if len(rep.Intents) != 1 || rep.Intents[0].Status != "removed" {
		t.Fatalf("removed = %+v", rep.Intents)
	}
}

func TestCompareCrossRepoLabels(t *testing.T) {
	before := doc([]schema.Change{})
	after := doc([]schema.Change{{Repo: "web", File: "a.ts", Location: "x", BusinessExplanation: "e"}})
	rep := Compare(before, after)
	if !strings.Contains(rep.FormatHuman(), "web:a.ts") {
		t.Fatalf("human = %s", rep.FormatHuman())
	}
}
