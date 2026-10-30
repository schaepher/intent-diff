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
	vres, err := validate.Run(filled, repo.Dir, validate.Options{})
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
