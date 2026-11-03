package schema

import (
	"strings"
	"testing"

	"intent-review/internal/report"
)

func treeDoc(chains ...Chain) Document {
	return Document{SchemaVersion: SchemaVersion, Meta: Meta{Title: "t", BaseCommit: "a", HeadRef: "b"}, Chains: chains}
}

func intents(pairs ...Intent) []Intent { return pairs }

func intent(id, parent string) Intent {
	return Intent{ID: id, Title: id, Summary: "s", Parent: parent, Changes: []Change{
		{File: "a.go", Location: "x", BusinessExplanation: "e", OldStart: 1, NewStart: 1,
			Lines: []Line{{Type: LineContext, Content: "a"}}},
	}}
}

func TestCheckIntentTreeValid(t *testing.T) {
	doc := treeDoc(
		Chain{ID: "c1", Title: "c1", Intents: intents(intent("i1", ""), intent("i2", "i1"))},
		Chain{ID: "c2", Title: "c2", Intents: intents(intent("j1", "c1/i2"), intent("j2", "c2/j1"))},
	)
	if errs := CheckIntentTree(doc); len(errs) != 0 {
		t.Fatalf("合法树被拒: %+v", errs)
	}
	if errs := CheckStructure(doc); len(errs) != 0 {
		t.Fatalf("CheckStructure 应包含树校验且通过: %+v", errs)
	}
}

func TestCheckIntentTreeMissingAmbiguousSelf(t *testing.T) {
	doc := treeDoc(
		Chain{ID: "c1", Title: "c1", Intents: intents(intent("i1", "nope"))},
	)
	if errs := CheckIntentTree(doc); len(errs) != 1 || !strings.Contains(errs[0].Message, "找不到") {
		t.Fatalf("缺失父意图应报错: %+v", errs)
	}

	doc = treeDoc(
		Chain{ID: "c1", Title: "c1", Intents: intents(intent("dup", ""))},
		Chain{ID: "c2", Title: "c2", Intents: intents(intent("dup", ""), intent("j1", "dup"))},
	)
	errs := CheckIntentTree(doc)
	if len(errs) != 1 || !strings.Contains(errs[0].Message, "重名") {
		t.Fatalf("裸 id 重名应报歧义: %+v", errs)
	}

	doc = treeDoc(Chain{ID: "c1", Title: "c1", Intents: intents(intent("i1", "i1"))})
	errs = CheckIntentTree(doc)
	if len(errs) != 1 || !strings.Contains(errs[0].Message, "不能指向自己") {
		t.Fatalf("自引用应报错: %+v", errs)
	}
}

func TestCheckIntentTreeCycle(t *testing.T) {
	doc := treeDoc(Chain{ID: "c1", Title: "c1", Intents: intents(
		intent("i1", "i2"), intent("i2", "i1"), intent("i3", ""),
	)})
	errs := CheckIntentTree(doc)
	if len(errs) != 1 || !strings.Contains(errs[0].Message, "形成环") {
		t.Fatalf("环应报一次错误: %+v", errs)
	}
}

// CheckStructure 集成：parent 错误要以 SCHEMA_INVALID 报出。
func TestCheckStructureIncludesTreeErrors(t *testing.T) {
	doc := docWith(validChange())
	doc.Chains[0].Intents[0].Parent = "missing"
	if errs := CheckStructure(doc); !hasCode(errs, report.CodeSchemaInvalid) {
		t.Fatalf("CheckStructure 未包含树校验: %v", codes(errs))
	}
}
