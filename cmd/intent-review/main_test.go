package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"intent-review/internal/schema"
	"intent-review/internal/testutil"
)

func fixtureRepo(t *testing.T) (*testutil.Repo, string) {
	t.Helper()
	repo := testutil.NewGitRepo(t, map[string]string{"a.txt": "one\ntwo\n"})
	repo.Write("a.txt", "one\nONE\ntwo\n")
	repo.Commit("add ONE")
	return repo, repo.Dir
}

func skeletonDoc() schema.Document {
	return schema.Document{
		SchemaVersion: schema.SchemaVersion,
		Meta:          schema.Meta{Title: "CLI 测试", BaseCommit: "HEAD~1", HeadRef: "HEAD"},
		Chains: []schema.Chain{{
			ID: "c1", Title: "链一", Intents: []schema.Intent{{
				ID: "i1", Title: "加一行", Summary: "在 one 后插入 ONE",
				Changes: []schema.Change{{
					File: "a.txt", Location: "第 1 行后", BusinessExplanation: "演示插入",
					Patch: []string{"@@ -1,1 +1,2 @@", " one", "+ONE"},
				}},
			}},
		}},
	}
}

func writeDoc(t *testing.T, doc schema.Document) string {
	t.Helper()
	raw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	path := filepath.Join(t.TempDir(), "review.json")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	return path
}

func runCLI(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	code := run(args, &out, &errb)
	return code, out.String(), errb.String()
}

func TestCLIHelpAndVersion(t *testing.T) {
	if code, out, _ := runCLI(t, "help"); code != 0 || !strings.Contains(out, "validate") {
		t.Fatalf("help: code=%d out=%q", code, out)
	}
	if code, out, _ := runCLI(t, "version"); code != 0 || !strings.Contains(out, "intent-review") {
		t.Fatalf("version: code=%d out=%q", code, out)
	}
	if code, _, errOut := runCLI(t, "nope"); code != 2 || !strings.Contains(errOut, "未知命令") {
		t.Fatalf("unknown command: code=%d err=%q", code, errOut)
	}
	if code, _, _ := runCLI(t); code != 2 {
		t.Fatalf("no args should exit 2")
	}
}

func TestCLISchema(t *testing.T) {
	code, out, errOut := runCLI(t, "schema")
	if code != 0 {
		t.Fatalf("schema: code=%d err=%s", code, errOut)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("schema output is not JSON: %v", err)
	}
	path := filepath.Join(t.TempDir(), "schema.json")
	if code, _, errOut := runCLI(t, "schema", "--output", path); code != 0 {
		t.Fatalf("schema --output: %d %s", code, errOut)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("schema file missing: %v", err)
	}
}

func TestCLIValidateRequiresLines(t *testing.T) {
	repo, repoDir := fixtureRepo(t)
	_ = repo
	docPath := writeDoc(t, skeletonDoc())
	code, out, errOut := runCLI(t, "validate", docPath, "--repo", repoDir, "--json")
	if code != 1 {
		t.Fatalf("code=%d out=%s err=%s", code, out, errOut)
	}
	var res struct {
		OK     bool `json:"ok"`
		Errors []struct {
			Code string `json:"code"`
		} `json:"errors"`
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("validate JSON: %v (%s)", err, out)
	}
	if res.OK || len(res.Errors) == 0 || res.Errors[0].Code != "SCHEMA_INVALID" {
		t.Fatalf("res = %+v", res)
	}
}

func TestCLIFillValidateRenderFlow(t *testing.T) {
	_, repoDir := fixtureRepo(t)
	dir := t.TempDir()
	skeletonPath := filepath.Join(dir, "review.json")
	raw, _ := json.MarshalIndent(skeletonDoc(), "", "  ")
	if err := os.WriteFile(skeletonPath, raw, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	filledPath := filepath.Join(dir, "filled.json")
	// flag 在位置参数之后也必须被解析。
	code, _, errOut := runCLI(t, "fill", skeletonPath, "--repo", repoDir, "-o", filledPath)
	if code != 0 {
		t.Fatalf("fill: code=%d err=%s", code, errOut)
	}
	filled, err := os.ReadFile(filledPath)
	if err != nil {
		t.Fatalf("read filled: %v", err)
	}
	if !strings.Contains(string(filled), `"lines"`) || strings.Contains(string(filled), `"patch"`) {
		t.Fatalf("fill output wrong: %s", filled)
	}

	code, out, errOut := runCLI(t, "validate", filledPath, "--repo", repoDir, "--json")
	if code != 0 {
		t.Fatalf("validate: code=%d out=%s err=%s", code, out, errOut)
	}
	if !strings.Contains(out, `"ok": true`) {
		t.Fatalf("validate out = %s", out)
	}

	htmlPath := filepath.Join(dir, "review.html")
	code, _, errOut = runCLI(t, "render", filledPath, "-o", htmlPath, "--repo", repoDir)
	if code != 0 {
		t.Fatalf("render: code=%d err=%s", code, errOut)
	}
	html, err := os.ReadFile(htmlPath)
	if err != nil {
		t.Fatalf("read html: %v", err)
	}
	if !bytes.Contains(html, []byte("<!DOCTYPE html>")) || bytes.Contains(html, []byte("http://")) {
		t.Fatalf("html not self-contained")
	}

	// render 可直接吃 patch-only 输入（内部自动 fill）。
	code, _, errOut = runCLI(t, "render", skeletonPath, "-o", htmlPath, "--repo", repoDir)
	if code != 0 {
		t.Fatalf("render patch-only: code=%d err=%s", code, errOut)
	}
	// 没有 --repo 时无法自动填充。
	code, _, errOut = runCLI(t, "render", skeletonPath, "-o", htmlPath)
	if code != 2 || !strings.Contains(errOut, "--repo") {
		t.Fatalf("render patch-only without repo: code=%d err=%s", code, errOut)
	}
}

func TestCLIRenderRequiresOutput(t *testing.T) {
	docPath := writeDoc(t, skeletonDoc())
	code, _, errOut := runCLI(t, "render", docPath)
	if code != 2 || !strings.Contains(errOut, "-o") {
		t.Fatalf("code=%d err=%s", code, errOut)
	}
}

func TestCLIUnsupportedSchemaVersion(t *testing.T) {
	doc := skeletonDoc()
	doc.SchemaVersion = "2.0"
	docPath := writeDoc(t, doc)
	code, out, errOut := runCLI(t, "validate", docPath, "--repo", ".", "--json")
	if code != 3 {
		t.Fatalf("code=%d out=%s err=%s", code, out, errOut)
	}
	if !strings.Contains(out, "SCHEMA_VERSION_UNSUPPORTED") {
		t.Fatalf("out = %s", out)
	}
}

func TestCLIRenderWarnsOnValidationFailure(t *testing.T) {
	_, repoDir := fixtureRepo(t)
	doc := skeletonDoc()
	ch := &doc.Chains[0].Intents[0].Changes[0]
	ch.Patch = nil
	ch.OldStart, ch.NewStart = 1, 1
	ch.Lines = []schema.Line{
		{Type: schema.LineContext, Content: "one"},
		{Type: schema.LineAdd, Content: "WRONG"},
	}
	docPath := writeDoc(t, doc)
	htmlPath := filepath.Join(t.TempDir(), "review.html")
	code, _, errOut := runCLI(t, "render", docPath, "-o", htmlPath, "--repo", repoDir)
	if code != 0 {
		t.Fatalf("render should still succeed: code=%d err=%s", code, errOut)
	}
	if !strings.Contains(errOut, "最终状态必须等于 head") || !strings.Contains(errOut, "校验未通过") {
		t.Fatalf("expected diagnostic on stderr, got %s", errOut)
	}
	html, _ := os.ReadFile(htmlPath)
	if !bytes.Contains(html, []byte("has-warning")) {
		t.Fatalf("warning not floated to timeline")
	}
}

func TestCLIHumanDiagnostics(t *testing.T) {
	doc := skeletonDoc()
	doc.Chains[0].Intents[0].Changes[0].BusinessExplanation = ""
	docPath := writeDoc(t, doc)
	code, out, errOut := runCLI(t, "validate", docPath)
	if code != 1 {
		t.Fatalf("code=%d", code)
	}
	if out != "" {
		t.Fatalf("human diagnostics must go to stderr, stdout=%q", out)
	}
	if !strings.Contains(errOut, "[c1] i1 change-1 a.txt") || !strings.Contains(errOut, "business_explanation 为空") {
		t.Fatalf("stderr = %s", errOut)
	}
}

func TestCLIRenderPrintsRenderWarnings(t *testing.T) {
	_, repoDir := fixtureRepo(t)
	dir := t.TempDir()
	skeletonPath := filepath.Join(dir, "review.json")
	raw, _ := json.MarshalIndent(skeletonDoc(), "", "  ")
	if err := os.WriteFile(skeletonPath, raw, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	filledPath := filepath.Join(dir, "filled.json")
	if code, _, errOut := runCLI(t, "fill", skeletonPath, "--repo", repoDir, "-o", filledPath); code != 0 {
		t.Fatalf("fill: %d %s", code, errOut)
	}
	htmlPath := filepath.Join(dir, "review.html")
	code, _, errOut := runCLI(t, "render", filledPath, "-o", htmlPath, "--repo", repoDir, "--max-lines", "1")
	if code != 0 {
		t.Fatalf("render: %d %s", code, errOut)
	}
	if !strings.Contains(errOut, "超过阈值") {
		t.Fatalf("expected large-review warning, stderr=%s", errOut)
	}
}
