package validate

import (
	"intent-review/internal/report"
	"intent-review/internal/schema"
	"strings"
	"testing"
)

func TestRunStructuralErrorSkipsRepo(t *testing.T) {
	doc := docFor("whatever", "whatever")
	doc.Chains[0].Intents[0].Changes[0].BusinessExplanation = ""
	res, err := Run(doc, "/nonexistent-repo-path", Options{})
	if err != nil {
		t.Fatalf("structural errors must not require repo: %v", err)
	}
	if res.OK || res.Errors[0].Code != report.CodeMissingExplanation {
		t.Fatalf("res = %+v", res)
	}
}

func TestRunUnfilledPatchHint(t *testing.T) {
	doc := docFor("whatever", "whatever")
	doc.Chains[0].Intents[0].Changes[0].Lines = nil
	doc.Chains[0].Intents[0].Changes[0].Patch = []string{"@@ -1 +1 @@", "-a", "+b"}
	res, err := Run(doc, "/nonexistent-repo", Options{})
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
	res, err := Run(doc, repo.Dir, Options{})
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
