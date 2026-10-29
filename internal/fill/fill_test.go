package fill

import (
	"encoding/json"
	"strings"
	"testing"

	"intent-review/internal/report"
	"intent-review/internal/schema"
	"intent-review/internal/testutil"
	"intent-review/internal/validate"
)

func skeleton(base, head string) schema.Document {
	return schema.Document{
		SchemaVersion: schema.SchemaVersion,
		Meta:          schema.Meta{Title: "限流与日志", BaseCommit: base, HeadRef: head},
		Chains: []schema.Chain{{
			ID: "chain-limit", Title: "限流", Intents: []schema.Intent{
				{
					ID: "intent-001", Title: "引入限流", Summary: "入口加限流",
					Changes: []schema.Change{{
						File: "a.go", Location: "Handler 入口", BusinessExplanation: "入口拦截过载",
						Patch: []string{
							"@@ -3,4 +3,5 @@",
							" func Handler() {",
							"+	limiter.Wait()",
							" 	resp := call()",
							" 	_ = resp",
							" }",
						},
					}},
				},
				{
					ID: "intent-002", Title: "补日志", Summary: "记录请求开始",
					Changes: []schema.Change{{
						File: "a.go", Location: "limiter 之后", BusinessExplanation: "区分被限流与下游慢",
						Patch: []string{
							"@@ -4,2 +4,3 @@",
							" 	limiter.Wait()",
							"+	log.Info(\"start\")",
							" 	resp := call()",
						},
					}},
				},
			},
		}},
	}
}

func fixture(t *testing.T) (*testutil.Repo, string, string) {
	t.Helper()
	repo := testutil.NewGitRepo(t, map[string]string{"a.go": "package main\n\nfunc Handler() {\n\tresp := call()\n\t_ = resp\n}\n"})
	base := repo.Head()
	repo.Write("a.go", "package main\n\nfunc Handler() {\n\tlimiter.Wait()\n\tresp := call()\n\t_ = resp\n}\n")
	repo.Commit("stage1")
	repo.Write("a.go", "package main\n\nfunc Handler() {\n\tlimiter.Wait()\n\tlog.Info(\"start\")\n\tresp := call()\n\t_ = resp\n}\n")
	head := repo.Commit("stage2")
	return repo, base, head
}

func mustMarshal(t *testing.T, doc schema.Document) []byte {
	t.Helper()
	raw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return raw
}

func TestRunFillsLines(t *testing.T) {
	repo, base, head := fixture(t)
	out, res, err := Run(skeleton(base, head), mustMarshal(t, skeleton(base, head)), repo.Dir)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !res.OK {
		t.Fatalf("errors = %+v", res.Errors)
	}
	var filled schema.Document
	if err := json.Unmarshal(out, &filled); err != nil {
		t.Fatalf("decode: %v", err)
	}
	ch := filled.Chains[0].Intents[0].Changes[0]
	if ch.OldStart != 3 || ch.NewStart != 3 || len(ch.Lines) != 5 {
		t.Fatalf("change1 = %+v", ch)
	}
	if ch.Lines[1].Type != schema.LineAdd || ch.Lines[1].Content != "\tlimiter.Wait()" {
		t.Fatalf("lines = %+v", ch.Lines)
	}
	if len(ch.Patch) != 0 {
		t.Fatalf("patch must be deleted, got %v", ch.Patch)
	}
	ch2 := filled.Chains[0].Intents[1].Changes[0]
	if ch2.OldStart != 4 || ch2.NewStart != 4 {
		t.Fatalf("change2 starts = %+v", ch2)
	}
	// 填充结果必须能被 validate 重放通过。
	vres, err := validate.Run(filled, repo.Dir)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if !vres.OK {
		t.Fatalf("filled doc fails validate: %+v", vres.Errors)
	}
}

func TestRunIdempotent(t *testing.T) {
	repo, base, head := fixture(t)
	raw := mustMarshal(t, skeleton(base, head))
	once, res, err := Run(skeleton(base, head), raw, repo.Dir)
	if err != nil || !res.OK {
		t.Fatalf("first fill: %v %+v", err, res.Errors)
	}
	var doc schema.Document
	if err := json.Unmarshal(once, &doc); err != nil {
		t.Fatalf("decode: %v", err)
	}
	twice, res2, err := Run(doc, once, repo.Dir)
	if err != nil || !res2.OK {
		t.Fatalf("second fill: %v %+v", err, res2.Errors)
	}
	if string(once) != string(twice) {
		t.Fatalf("fill not idempotent:\nfirst:\n%s\nsecond:\n%s", once, twice)
	}
}

func TestRunRelocatesUniqueContext(t *testing.T) {
	repo, base, head := fixture(t)
	doc := skeleton(base, head)
	doc.Chains[0].Intents[1].Changes[0].Patch[0] = "@@ -9,2 +9,3 @@"
	out, res, err := Run(doc, mustMarshal(t, doc), repo.Dir)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !res.OK {
		t.Fatalf("relocation should succeed: %+v", res.Errors)
	}
	if len(res.Warnings) != 1 || res.Warnings[0].Code != report.CodePatchContext {
		t.Fatalf("warnings = %+v", res.Warnings)
	}
	var filled schema.Document
	if err := json.Unmarshal(out, &filled); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got := filled.Chains[0].Intents[1].Changes[0].OldStart; got != 4 {
		t.Fatalf("relocated old_start = %d want 4", got)
	}
}

func TestRunPatchContextMismatch(t *testing.T) {
	repo, base, head := fixture(t)
	doc := skeleton(base, head)
	doc.Chains[0].Intents[1].Changes[0].Patch[1] = " \tlegacyLimiter.Wait()"
	out, res, err := Run(doc, mustMarshal(t, doc), repo.Dir)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if res.OK || out != nil {
		t.Fatalf("want failure without output, got ok=%v out=%q", res.OK, out)
	}
	if res.Errors[0].Code != report.CodePatchContext {
		t.Fatalf("code = %s (%+v)", res.Errors[0].Code, res.Errors[0])
	}
	if res.Errors[0].IntentID != "intent-002" {
		t.Fatalf("locator = %+v", res.Errors[0])
	}
}

func TestRunMultipleHunksRejected(t *testing.T) {
	repo, base, head := fixture(t)
	doc := skeleton(base, head)
	doc.Chains[0].Intents[0].Changes[0].Patch = []string{
		"@@ -3,1 +3,1 @@", " func Handler() {", "@@ -6,1 +6,1 @@", " }",
	}
	_, res, err := Run(doc, mustMarshal(t, doc), repo.Dir)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if res.OK || res.Errors[0].Code != report.CodePatchParse {
		t.Fatalf("res = %+v", res)
	}
	if !strings.Contains(res.Errors[0].Hint, "一个 change 只允许一个 hunk") {
		t.Fatalf("hint = %q", res.Errors[0].Hint)
	}
}

func TestRunPreservesUnknownFields(t *testing.T) {
	repo, base, head := fixture(t)
	doc := skeleton(base, head)
	raw := mustMarshal(t, doc)
	var tree map[string]any
	if err := json.Unmarshal(raw, &tree); err != nil {
		t.Fatalf("decode: %v", err)
	}
	tree["x-note"] = "keep-me"
	chains := tree["chains"].([]any)
	chain := chains[0].(map[string]any)
	intents := chain["intents"].([]any)
	change := intents[0].(map[string]any)["changes"].([]any)[0].(map[string]any)
	change["x-custom"] = "change-level"
	raw, _ = json.Marshal(tree)

	out, res, err := Run(doc, raw, repo.Dir)
	if err != nil || !res.OK {
		t.Fatalf("run: %v %+v", err, res.Errors)
	}
	var back map[string]any
	if err := json.Unmarshal(out, &back); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if back["x-note"] != "keep-me" {
		t.Fatalf("top-level unknown field lost")
	}
	ch := back["chains"].([]any)[0].(map[string]any)["intents"].([]any)[0].(map[string]any)["changes"].([]any)[0].(map[string]any)
	if ch["x-custom"] != "change-level" {
		t.Fatalf("change-level unknown field lost")
	}
}

func TestRunNewFileAndDeleted(t *testing.T) {
	repo := testutil.NewGitRepo(t, map[string]string{
		"del.go": "package main\n\nvar A = 1\n",
	})
	base := repo.Head()
	repo.Remove("del.go")
	repo.Write("new.go", "package main\n\nvar B = 2\n")
	head := repo.Commit("replace")

	doc := schema.Document{
		SchemaVersion: schema.SchemaVersion,
		Meta:          schema.Meta{Title: "t", BaseCommit: base, HeadRef: head},
		Chains: []schema.Chain{{ID: "c", Title: "c", Intents: []schema.Intent{{
			ID: "i1", Title: "增删", Summary: "新增与删除",
			Changes: []schema.Change{
				{
					File: "new.go", Location: "新文件", BusinessExplanation: "新增常量", NewFile: true,
					Patch: []string{"@@ -0,0 +1,3 @@", "+package main", "+", "+var B = 2"},
				},
				{
					File: "del.go", Location: "旧文件", BusinessExplanation: "删除旧常量", Deleted: true,
					Patch: []string{"@@ -1,3 +0,0 @@", "-package main", "-", "-var A = 1"},
				},
			},
		}}}},
	}
	out, res, err := Run(doc, mustMarshal(t, doc), repo.Dir)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !res.OK {
		t.Fatalf("errors = %+v", res.Errors)
	}
	var filled schema.Document
	if err := json.Unmarshal(out, &filled); err != nil {
		t.Fatalf("decode: %v", err)
	}
	nf := filled.Chains[0].Intents[0].Changes[0]
	if nf.OldStart != 0 || nf.NewStart != 1 {
		t.Fatalf("new_file starts = %d/%d", nf.OldStart, nf.NewStart)
	}
	df := filled.Chains[0].Intents[0].Changes[1]
	if df.OldStart != 1 || df.NewStart != 0 {
		t.Fatalf("deleted starts = %d/%d", df.OldStart, df.NewStart)
	}
	vres, err := validate.Run(filled, repo.Dir)
	if err != nil || !vres.OK {
		t.Fatalf("validate: %v %+v", err, vres.Errors)
	}
}

func TestRunCRLFFile(t *testing.T) {
	repo := testutil.NewGitRepo(t, map[string]string{"a.txt": "one\r\ntwo\r\n"})
	base := repo.Head()
	repo.Write("a.txt", "one\r\nONE\r\ntwo\r\n")
	head := repo.Commit("crlf")

	doc := schema.Document{
		SchemaVersion: schema.SchemaVersion,
		Meta:          schema.Meta{Title: "crlf", BaseCommit: base, HeadRef: head},
		Chains: []schema.Chain{{ID: "c", Title: "c", Intents: []schema.Intent{{
			ID: "i", Title: "插入", Summary: "CRLF 文件插入一行",
			Changes: []schema.Change{{
				File: "a.txt", Location: "第 1 行后", BusinessExplanation: "验证 CRLF 内容不被改写",
				Patch: []string{"@@ -1,1 +1,2 @@", " one\r", "+ONE\r"},
			}},
		}}}},
	}
	out, res, err := Run(doc, mustMarshal(t, doc), repo.Dir)
	if err != nil || !res.OK {
		t.Fatalf("fill: %v %+v", err, res.Errors)
	}
	var filled schema.Document
	if err := json.Unmarshal(out, &filled); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got := filled.Chains[0].Intents[0].Changes[0].Lines[0].Content; got != "one\r" {
		t.Fatalf("CR 被剥掉: %q", got)
	}
	vres, err := validate.Run(filled, repo.Dir)
	if err != nil || !vres.OK {
		t.Fatalf("validate: %v %+v", err, vres.Errors)
	}
}
