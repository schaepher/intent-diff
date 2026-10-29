package validate

import (
	"intent-review/internal/report"
	"intent-review/internal/schema"
	"intent-review/internal/testutil"
	"strings"
	"testing"
)

func TestRunChainBaseCommitOverride(t *testing.T) {
	repo := testutil.NewGitRepo(t, map[string]string{"a.txt": "v1\n"})
	oldBase := repo.Head()
	repo.Write("a.txt", "v2\n")
	repo.Commit("v2")
	repo.Write("a.txt", "v3\n")
	head := repo.Commit("v3")

	// meta.base 指向老提交，chain 覆盖为 v2 的提交：链内只需重放 v2→v3。
	doc := schema.Document{
		SchemaVersion: schema.SchemaVersion,
		Meta:          schema.Meta{Title: "t", BaseCommit: oldBase, HeadRef: head},
		Chains: []schema.Chain{{
			ID: "c", Title: "c", BaseCommit: repo.Head(), Intents: []schema.Intent{{
				ID: "i", Title: "i", Summary: "s", Changes: []schema.Change{{
					File: "a.txt", Location: "全文件", BusinessExplanation: "覆盖 base",
					OldStart: 1, NewStart: 1,
					Lines: []schema.Line{
						{Type: schema.LineDel, Content: "v2"},
						{Type: schema.LineAdd, Content: "v3"},
					},
				}},
			}},
		}},
	}
	// 重新取 v2 提交：上面 repo.Head() 已是 v3，需要显式解析。
	v2, err := (&replayProbe{Dir: repo.Dir}).resolve("HEAD~1")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	doc.Chains[0].BaseCommit = v2
	res, err := Run(doc, repo.Dir)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !res.OK {
		t.Fatalf("errors = %+v", res.Errors)
	}
}

func TestRunUncoveredFile(t *testing.T) {
	repo := testutil.NewGitRepo(t, map[string]string{
		"a.go": baseGo,
		"b.go": "package main\n\nvar X = 1\n",
	})
	base := repo.Head()
	repo.Write("a.go", stage2Go)
	repo.Write("b.go", "package main\n\nvar X = 2\n")
	head := repo.Commit("both changed")

	// 只覆盖 a.go，b.go 的变更被遗漏。
	res, err := Run(docFor(base, head), repo.Dir)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if res.OK {
		t.Fatalf("expected coverage failure")
	}
	found := false
	for _, e := range res.Errors {
		if e.Code == report.CodeFinalStateMismatch && e.File == "b.go" && strings.Contains(e.Message, "没有被任何意图覆盖") {
			found = true
		}
	}
	if !found {
		t.Fatalf("errors = %+v", res.Errors)
	}
}

func TestRunUndeclaredRenameCaught(t *testing.T) {
	repo := testutil.NewGitRepo(t, map[string]string{"old.go": "package main\n\nvar A = 1\n"})
	base := repo.Head()
	repo.Remove("old.go")
	repo.Write("new.go", "package main\n\nvar A = 1\n")
	head := repo.Commit("rename without declaration")

	doc := schema.Document{
		SchemaVersion: schema.SchemaVersion,
		Meta:          schema.Meta{Title: "t", BaseCommit: base, HeadRef: head},
		Chains: []schema.Chain{{ID: "c", Title: "c", Intents: []schema.Intent{{
			ID: "i", Title: "新增文件", Summary: "把内容复制成新文件",
			Changes: []schema.Change{{
				File: "new.go", Location: "新文件", BusinessExplanation: "新增", NewFile: true,
				OldStart: 0, NewStart: 1,
				Lines: []schema.Line{
					{Type: schema.LineAdd, Content: "package main"},
					{Type: schema.LineAdd, Content: ""},
					{Type: schema.LineAdd, Content: "var A = 1"},
				},
			}},
		}}}},
	}
	res, err := Run(doc, repo.Dir)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	found := false
	for _, e := range res.Errors {
		if e.File == "old.go" && strings.Contains(e.Message, "没有被任何意图覆盖") {
			found = true
		}
	}
	if !found {
		t.Fatalf("undeclared rename should leave old.go uncovered, got %+v", res.Errors)
	}
}

func TestRunDeclaredRenamePasses(t *testing.T) {
	repo := testutil.NewGitRepo(t, map[string]string{"old.go": "package main\n\nvar A = 1\n"})
	base := repo.Head()
	repo.Remove("old.go")
	repo.Write("new.go", "package main\n\nvar A = 2\n")
	head := repo.Commit("rename + edit")

	doc := schema.Document{
		SchemaVersion: schema.SchemaVersion,
		Meta:          schema.Meta{Title: "t", BaseCommit: base, HeadRef: head},
		Chains: []schema.Chain{{ID: "c", Title: "c", Intents: []schema.Intent{{
			ID: "i", Title: "重命名", Summary: "改名并调整常量",
			Changes: []schema.Change{{
				File: "new.go", RenamedFrom: "old.go", Location: "重命名", BusinessExplanation: "改名并调值",
				OldStart: 1, NewStart: 1,
				Lines: []schema.Line{
					{Type: schema.LineContext, Content: "package main"},
					{Type: schema.LineContext, Content: ""},
					{Type: schema.LineDel, Content: "var A = 1"},
					{Type: schema.LineAdd, Content: "var A = 2"},
				},
			}},
		}}}},
	}
	res, err := Run(doc, repo.Dir)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !res.OK {
		t.Fatalf("errors = %+v", res.Errors)
	}
}
