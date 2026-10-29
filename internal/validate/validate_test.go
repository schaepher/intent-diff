package validate

import (
	"strings"
	"testing"

	"intent-review/internal/replay"
	"intent-review/internal/report"
	"intent-review/internal/schema"
	"intent-review/internal/testutil"
)

const baseGo = `package main

func Handler() {
	resp := call()
	_ = resp
}
`

const stage1Go = `package main

func Handler() {
	limiter.Wait()
	resp := call()
	_ = resp
}
`

const stage2Go = `package main

func Handler() {
	limiter.Wait()
	log.Info("start")
	resp := call()
	_ = resp
}
`

// intentsFor 用结构化 lines 构造两段意图，便于复用。
func intentsFor() []schema.Intent {
	return []schema.Intent{
		{
			ID: "intent-001", Title: "引入限流", Summary: "入口加限流",
			Changes: []schema.Change{{
				File: "a.go", Location: "Handler 入口", BusinessExplanation: "入口拦截过载",
				OldStart: 3, NewStart: 3,
				Lines: []schema.Line{
					{Type: schema.LineContext, Content: "func Handler() {"},
					{Type: schema.LineAdd, Content: "\tlimiter.Wait()"},
					{Type: schema.LineContext, Content: "\tresp := call()"},
					{Type: schema.LineContext, Content: "\t_ = resp"},
					{Type: schema.LineContext, Content: "}"},
				},
			}},
		},
		{
			ID: "intent-002", Title: "补日志", Summary: "记录请求开始",
			Changes: []schema.Change{{
				File: "a.go", Location: "limiter 之后", BusinessExplanation: "区分被限流与下游慢",
				OldStart: 4, NewStart: 4,
				Lines: []schema.Line{
					{Type: schema.LineContext, Content: "\tlimiter.Wait()"},
					{Type: schema.LineAdd, Content: "\tlog.Info(\"start\")"},
					{Type: schema.LineContext, Content: "\tresp := call()"},
				},
			}},
		},
	}
}

func docFor(base, head string) schema.Document {
	return schema.Document{
		SchemaVersion: schema.SchemaVersion,
		Meta:          schema.Meta{Title: "限流与日志", BaseCommit: base, HeadRef: head},
		Chains: []schema.Chain{{
			ID: "chain-limit", Title: "限流", Intents: intentsFor(),
		}},
	}
}

func newHeadFixture(t *testing.T) (*testutil.Repo, string, string) {
	t.Helper()
	repo := testutil.NewGitRepo(t, map[string]string{"a.go": baseGo})
	base := repo.Head()
	repo.Write("a.go", stage1Go)
	repo.Commit("stage1")
	repo.Write("a.go", stage2Go)
	head := repo.Commit("stage2")
	return repo, base, head
}

func TestRunOK(t *testing.T) {
	repo, base, head := newHeadFixture(t)
	res, err := Run(docFor(base, head), repo.Dir)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !res.OK {
		t.Fatalf("want ok, errors=%+v", res.Errors)
	}
}

func TestRunContextMismatch(t *testing.T) {
	repo, base, head := newHeadFixture(t)
	doc := docFor(base, head)
	doc.Chains[0].Intents[1].Changes[0].Lines[0].Content = "\tlegacyLimiter.Wait()"
	res, err := Run(doc, repo.Dir)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if res.OK || len(res.Errors) != 1 {
		t.Fatalf("want one error, got %+v", res)
	}
	e := res.Errors[0]
	if e.Code != report.CodeContextMismatch {
		t.Fatalf("code = %s", e.Code)
	}
	if e.ChainID != "chain-limit" || e.IntentID != "intent-002" || e.ChangeIndex == nil || *e.ChangeIndex != 0 {
		t.Fatalf("locator = %+v", e)
	}
	if e.Line != 4 || e.Expected != "\tlegacyLimiter.Wait()" || e.Actual != "\tlimiter.Wait()" {
		t.Fatalf("detail = %+v", e)
	}
	if !strings.Contains(e.Hint, "意图") {
		t.Fatalf("hint = %q", e.Hint)
	}
}

func TestRunFinalStateMismatch(t *testing.T) {
	repo, base, _ := newHeadFixture(t)
	repo.Write("a.go", stage2Go+"\n// extra\n")
	head := repo.Commit("head with extra")
	res, err := Run(docFor(base, head), repo.Dir)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if res.OK {
		t.Fatalf("want failure")
	}
	if res.Errors[0].Code != report.CodeFinalStateMismatch {
		t.Fatalf("code = %s (%+v)", res.Errors[0].Code, res.Errors[0])
	}
}

func TestRunStructuralErrorSkipsRepo(t *testing.T) {
	doc := docFor("whatever", "whatever")
	doc.Chains[0].Intents[0].Changes[0].BusinessExplanation = ""
	res, err := Run(doc, "/nonexistent-repo-path")
	if err != nil {
		t.Fatalf("structural errors must not require repo: %v", err)
	}
	if res.OK || res.Errors[0].Code != report.CodeMissingExplanation {
		t.Fatalf("res = %+v", res)
	}
}

func TestRunChainIsolation(t *testing.T) {
	repo := testutil.NewGitRepo(t, map[string]string{
		"a.go": baseGo,
		"b.go": "package main\n\nvar X = 1\n",
	})
	base := repo.Head()
	// 两条链各自从 base 重放不同文件，互不影响。
	repo.Write("a.go", stage2Go)
	repo.Write("b.go", "package main\n\nvar X = 2\n")
	head := repo.Commit("all")

	doc := docFor(base, head)
	doc.Chains = append(doc.Chains, schema.Chain{
		ID: "chain-b", Title: "配置", Intents: []schema.Intent{{
			ID: "intent-b1", Title: "改配置", Summary: "调整 X",
			Changes: []schema.Change{{
				File: "b.go", Location: "var X", BusinessExplanation: "配置改值",
				OldStart: 1, NewStart: 1,
				Lines: []schema.Line{
					{Type: schema.LineContext, Content: "package main"},
					{Type: schema.LineContext, Content: ""},
					{Type: schema.LineDel, Content: "var X = 1"},
					{Type: schema.LineAdd, Content: "var X = 2"},
				},
			}},
		}},
	})
	res, err := Run(doc, repo.Dir)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !res.OK {
		t.Fatalf("want ok, errors=%+v", res.Errors)
	}
}

func TestRunBinaryWarning(t *testing.T) {
	repo, base, head := newHeadFixture(t)
	doc := docFor(base, head)
	doc.Chains[0].Intents[0].Changes = append(doc.Chains[0].Intents[0].Changes, schema.Change{
		File: "logo.png", Location: "图标", BusinessExplanation: "替换图标", Binary: true,
	})
	repo.Write("logo.png", "binary")
	_ = base
	head2 := repo.Commit("add binary")
	doc.Meta.HeadRef = head2
	res, err := Run(doc, repo.Dir)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !res.OK {
		t.Fatalf("warning must not fail: %+v", res.Errors)
	}
	if len(res.Warnings) == 0 || res.Warnings[0].Code != report.CodeBinaryChangeSkipped {
		t.Fatalf("warnings = %+v", res.Warnings)
	}
	_ = head
}

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

func TestRunSameFileStaleLineNumber(t *testing.T) {
	repo, base, head := newHeadFixture(t)
	doc := docFor(base, head)
	doc.Chains[0].Intents[1].Changes[0].OldStart = 6
	res, err := Run(doc, repo.Dir)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if res.OK {
		t.Fatalf("expected failure for stale line number")
	}
	if res.Errors[0].Code != report.CodeContextMismatch {
		t.Fatalf("code = %s", res.Errors[0].Code)
	}
}

func TestRunUnfilledPatchHint(t *testing.T) {
	doc := docFor("whatever", "whatever")
	doc.Chains[0].Intents[0].Changes[0].Lines = nil
	doc.Chains[0].Intents[0].Changes[0].Patch = []string{"@@ -1 +1 @@", "-a", "+b"}
	res, err := Run(doc, "/nonexistent-repo")
	if err != nil {
		t.Fatalf("unfilled check must not need repo: %v", err)
	}
	if res.OK || res.Errors[0].Code != report.CodeSchemaInvalid {
		t.Fatalf("res = %+v", res)
	}
	if !strings.Contains(res.Errors[0].Hint, "fill") {
		t.Fatalf("hint = %q", res.Errors[0].Hint)
	}
}

// replayProbe 用 replay 包解析提交，避免测试里直接依赖 git 命令。
type replayProbe struct{ Dir string }

func (p *replayProbe) resolve(ref string) (string, error) {
	r, err := replay.Open(p.Dir)
	if err != nil {
		return "", err
	}
	return r.Resolve(ref)
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
