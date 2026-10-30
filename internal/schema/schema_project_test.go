package schema

import (
	"testing"

	"intent-review/internal/report"
)

func TestCheckStructureMultiRepo(t *testing.T) {
	doc := docWith(validChange())
	doc.Meta.BaseCommit, doc.Meta.HeadRef = "", ""
	doc.Repos = []Repo{
		{ID: "web", Path: "web", BaseCommit: "b1", HeadRef: "h1"},
		{ID: "api", Path: "api", BaseCommit: "b2", HeadRef: "h2"},
	}
	doc.Chains[0].Intents[0].Changes[0].Repo = "web"
	if errs := CheckStructure(doc); len(errs) != 0 {
		t.Fatalf("合法跨仓文档被拒: %v", codes(errs))
	}

	// change 缺 repo / repo 未声明 / 单仓文档写了 repo。
	ch := &doc.Chains[0].Intents[0].Changes[0]
	ch.Repo = ""
	if errs := CheckStructure(doc); !hasCode(errs, report.CodeSchemaInvalid) {
		t.Fatalf("多仓文档缺 repo 应报错: %v", codes(errs))
	}
	ch.Repo = "nope"
	if errs := CheckStructure(doc); !hasCode(errs, report.CodeSchemaInvalid) {
		t.Fatalf("未声明 repo 应报错: %v", codes(errs))
	}
	single := docWith(validChange())
	single.Chains[0].Intents[0].Changes[0].Repo = "web"
	if errs := CheckStructure(single); !hasCode(errs, report.CodeSchemaInvalid) {
		t.Fatalf("单仓文档不应带 repo: %v", codes(errs))
	}
}

func TestCheckStructureRepoIDsAndBases(t *testing.T) {
	doc := docWith(validChange())
	doc.Meta.BaseCommit, doc.Meta.HeadRef = "", ""
	doc.Repos = []Repo{
		{ID: "web", Path: "web", BaseCommit: "b", HeadRef: "h"},
		{ID: "web", Path: "web2", BaseCommit: "b", HeadRef: "h"},
	}
	doc.Chains[0].Intents[0].Changes[0].Repo = "web"
	if errs := CheckStructure(doc); !hasCode(errs, report.CodeDuplicateRepoID) {
		t.Fatalf("重复仓库 id 应报错: %v", codes(errs))
	}

	doc = docWith(validChange())
	doc.Meta.BaseCommit, doc.Meta.HeadRef = "", ""
	doc.Repos = []Repo{{ID: "web", Path: "/abs/web", BaseCommit: "b", HeadRef: "h"}}
	doc.Chains[0].Intents[0].Changes[0].Repo = "web"
	if errs := CheckStructure(doc); !hasCode(errs, report.CodeSchemaInvalid) {
		t.Fatalf("绝对路径应被拒绝: %v", codes(errs))
	}

	doc = docWith(validChange())
	doc.Meta.BaseCommit, doc.Meta.HeadRef = "", ""
	doc.Repos = []Repo{{ID: "web", Path: "web", HeadRef: "h"}}
	doc.Chains[0].Intents[0].Changes[0].Repo = "web"
	if errs := CheckStructure(doc); !hasCode(errs, report.CodeSchemaInvalid) {
		t.Fatalf("缺 base 应报错: %v", codes(errs))
	}
}

func TestCheckStructureDisplayOrder(t *testing.T) {
	ch := validChange()
	ch.DisplayOrder = -1
	if errs := CheckStructure(docWith(ch)); !hasCode(errs, report.CodeSchemaInvalid) {
		t.Fatalf("负数 display_order 应报错: %v", codes(errs))
	}
	ch.DisplayOrder = 5
	ch.DisplayGroup = "主流程"
	if errs := CheckStructure(docWith(ch)); len(errs) != 0 {
		t.Fatalf("合法 display 字段被拒: %v", codes(errs))
	}
}

func TestCheckStructureChainBaseCommits(t *testing.T) {
	doc := docWith(validChange())
	doc.Meta.BaseCommit, doc.Meta.HeadRef = "", ""
	doc.Repos = []Repo{{ID: "web", Path: "web", BaseCommit: "b", HeadRef: "h"}}
	doc.Chains[0].Intents[0].Changes[0].Repo = "web"
	doc.Chains[0].BaseCommits = map[string]string{"nope": "x"}
	if errs := CheckStructure(doc); !hasCode(errs, report.CodeSchemaInvalid) {
		t.Fatalf("未知仓库的 base_commits 应报错: %v", codes(errs))
	}
}

func TestCheckStructureChangeIDAndReferences(t *testing.T) {
	doc := docWith(validChange())
	doc.Chains[0].Intents[0].Changes[0].ID = "ch-001"
	if errs := CheckStructure(doc); len(errs) != 0 {
		t.Fatalf("合法 change id 被拒: %v", codes(errs))
	}
	dup := doc.Chains[0].Intents[0].Changes[0]
	doc.Chains[0].Intents[0].Changes = append(doc.Chains[0].Intents[0].Changes, dup)
	if errs := CheckStructure(doc); !hasCode(errs, report.CodeDuplicateChangeID) {
		t.Fatalf("重复 change id 应报错: %v", codes(errs))
	}

	doc = docWith(validChange())
	doc.Chains[0].Intents[0].Changes[0].References = []string{"i2/ch-002", "i2#辅助函数"}
	if errs := CheckStructure(doc); len(errs) != 0 {
		t.Fatalf("合法 references 被拒: %v", codes(errs))
	}
	doc.Chains[0].Intents[0].Changes[0].References = []string{"  "}
	if errs := CheckStructure(doc); !hasCode(errs, report.CodeSchemaInvalid) {
		t.Fatalf("空 references 应报错: %v", codes(errs))
	}
}
