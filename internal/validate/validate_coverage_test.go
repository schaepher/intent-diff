package validate

import (
	"intent-review/internal/filefilter"
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
	res, err := Run(doc, repo.Dir, Options{})
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
	res, err := Run(docFor(base, head), repo.Dir, Options{})
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
	res, err := Run(doc, repo.Dir, Options{})
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
	res, err := Run(doc, repo.Dir, Options{})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !res.OK {
		t.Fatalf("errors = %+v", res.Errors)
	}
}

// 默认排除列表（*_test.go）只影响覆盖检查：测试文件变更不需要意图覆盖。
func TestRunCoverageSkipsDefaultExcludedTestFile(t *testing.T) {
	repo := testutil.NewGitRepo(t, map[string]string{"a.go": baseGo})
	base := repo.Head()
	repo.Write("a.go", stage2Go)
	repo.Write("pkg/a_test.go", "package pkg\n")
	head := repo.Commit("code + test")

	res, err := Run(docFor(base, head), repo.Dir, Options{})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !res.OK {
		t.Fatalf("测试文件应被默认排除，errors = %+v", res.Errors)
	}
}

func TestRunCoverageUnexcludeRestoresTestFileCheck(t *testing.T) {
	repo := testutil.NewGitRepo(t, map[string]string{"a.go": baseGo})
	base := repo.Head()
	repo.Write("a.go", stage2Go)
	repo.Write("a_test.go", "package main\n")
	head := repo.Commit("code + test")

	opt := Options{Files: filefilter.Default().Remove("*_test.go")}
	res, err := Run(docFor(base, head), repo.Dir, opt)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if res.OK {
		t.Fatalf("移除默认排除项后应报未覆盖")
	}
	if !hasCoverageError(res, "a_test.go") {
		t.Fatalf("errors = %+v", res.Errors)
	}
}

func TestRunCoverageHonorsCustomExclude(t *testing.T) {
	repo := testutil.NewGitRepo(t, map[string]string{"a.go": baseGo})
	base := repo.Head()
	repo.Write("a.go", stage2Go)
	repo.Write("docs/guide.md", "# guide\n")
	head := repo.Commit("code + docs")

	res, err := Run(docFor(base, head), repo.Dir, Options{})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !hasCoverageError(res, "docs/guide.md") {
		t.Fatalf("默认列表不排除 docs，errors = %+v", res.Errors)
	}

	opt := Options{Files: filefilter.New([]string{"docs/**"})}
	res, err = Run(docFor(base, head), repo.Dir, opt)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !res.OK {
		t.Fatalf("自定义排除应生效，errors = %+v", res.Errors)
	}
}

// 排除列表不得让已声明的 change 逃过重放校验（只影响覆盖检查）。
func TestRunFilterDoesNotSkipReplayOfExcludedFile(t *testing.T) {
	repo := testutil.NewGitRepo(t, map[string]string{
		"a.go":      baseGo,
		"b_test.go": "package main\n\nvar T = 1\n",
	})
	base := repo.Head()
	repo.Write("a.go", stage2Go)
	repo.Write("b_test.go", "package main\n\nvar T = 2\n")
	head := repo.Commit("code + test edit")

	doc := docFor(base, head)
	doc.Chains[0].Intents = append(doc.Chains[0].Intents, schema.Intent{
		ID: "intent-003", Title: "改测试", Summary: "测试常量",
		Changes: []schema.Change{{
			File: "b_test.go", Location: "T", BusinessExplanation: "测试常量",
			OldStart: 3, NewStart: 3,
			Lines: []schema.Line{
				{Type: schema.LineDel, Content: "var T = 0"},
				{Type: schema.LineAdd, Content: "var T = 2"},
			},
		}},
	})
	res, err := Run(doc, repo.Dir, Options{})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if res.OK || len(res.Errors) == 0 || res.Errors[0].File != "b_test.go" {
		t.Fatalf("排除列表不应跳过重放，errors = %+v", res.Errors)
	}
}

func hasCoverageError(res report.Result, file string) bool {
	for _, e := range res.Errors {
		if e.Code == report.CodeFinalStateMismatch && e.File == file && strings.Contains(e.Message, "没有被任何意图覆盖") {
			return true
		}
	}
	return false
}
