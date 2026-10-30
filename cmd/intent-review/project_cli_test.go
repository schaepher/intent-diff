package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"intent-review/internal/schema"
	"intent-review/internal/testutil"
)

// multiRepoProject 构造项目根下 web / api 两个仓库 + 跨仓 patch-only 文档。
func multiRepoProject(t *testing.T) (project, docPath string, doc schema.Document) {
	t.Helper()
	project = t.TempDir()
	web := testutil.NewGitRepoAt(t, filepath.Join(project, "web"), map[string]string{"src/a.ts": "one\n"})
	api := testutil.NewGitRepoAt(t, filepath.Join(project, "api"), map[string]string{"b.go": "package api\n\nvar A = 1\n"})
	webBase, apiBase := web.Head(), api.Head()
	web.Write("src/a.ts", "two\n")
	api.Write("b.go", "package api\n\nvar A = 2\n")
	webHead, apiHead := web.Commit("web"), api.Commit("api")

	doc = schema.Document{
		SchemaVersion: schema.SchemaVersion,
		Meta:          schema.Meta{Title: "跨仓需求"},
		Repos: []schema.Repo{
			{ID: "web", Title: "前端", Path: "web", BaseCommit: webBase, HeadRef: webHead},
			{ID: "api", Title: "后端", Path: "api", BaseCommit: apiBase, HeadRef: apiHead},
		},
		Chains: []schema.Chain{{ID: "c1", Title: "同一需求", Intents: []schema.Intent{{
			ID: "i1", Title: "两端", Summary: "同一需求跨仓",
			Changes: []schema.Change{
				{
					Repo: "api", File: "b.go", Location: "A", BusinessExplanation: "后端改值", DisplayOrder: 20,
					Patch: []string{"@@ -3,1 +3,1 @@", "-var A = 1", "+var A = 2"},
				},
				{
					Repo: "web", File: "src/a.ts", Location: "第 1 行", BusinessExplanation: "前端改值", DisplayOrder: 10,
					Patch: []string{"@@ -1,1 +1,1 @@", "-one", "+two"},
				},
			},
		}}}},
	}
	return project, writeDoc(t, doc), doc
}

func TestCLIMultiRepoFillValidateRender(t *testing.T) {
	project, docPath, _ := multiRepoProject(t)
	dir := t.TempDir()
	filled := filepath.Join(dir, "filled.json")
	if code, _, errOut := runCLI(t, "fill", docPath, "--repo", project, "-o", filled); code != 0 {
		t.Fatalf("fill: code=%d err=%s", code, errOut)
	}
	code, out, errOut := runCLI(t, "validate", filled, "--repo", project, "--json")
	if code != 0 || !strings.Contains(out, `"ok": true`) {
		t.Fatalf("validate: code=%d out=%s err=%s", code, out, errOut)
	}

	realWeb := t.TempDir()
	htmlPath := filepath.Join(dir, "review.html")
	code, _, errOut = runCLI(t, "render", filled, "-o", htmlPath, "--repo", project, "--source-root", "web="+realWeb)
	if code != 0 {
		t.Fatalf("render: code=%d err=%s", code, errOut)
	}
	html := readFile(t, htmlPath)
	if !strings.Contains(html, `<span class="repo-tag">web</span>`) || !strings.Contains(html, `<span class="repo-tag">api</span>`) {
		t.Fatalf("缺少仓库标签")
	}
	// --source-root 覆盖 web 的源码根；api 默认指向项目下的 checkout（而不是重放快照）。
	if !strings.Contains(html, "vscode://file"+realWeb+"/src/a.ts:1") {
		t.Fatalf("web 链接未指向 --source-root: %s", htmlPath)
	}
	if !strings.Contains(html, "vscode://file"+filepath.Join(project, "api")+"/b.go:3") {
		t.Fatalf("api 链接未指向仓库 checkout")
	}
}

// 文档内 file_filter 参与覆盖检查（统一配置，跨仓共用）。
func TestCLIDocFileFilter(t *testing.T) {
	project := t.TempDir()
	repo := testutil.NewGitRepoAt(t, filepath.Join(project, "web"), map[string]string{"a.txt": "one\n"})
	base := repo.Head()
	repo.Write("a.txt", "two\n")
	repo.Write("docs/guide.md", "# g\n")
	head := repo.Commit("code + docs")
	mk := func(filter *schema.FileFilterConfig) string {
		doc := schema.Document{
			SchemaVersion: schema.SchemaVersion,
			Meta:          schema.Meta{Title: "t"},
			Repos:         []schema.Repo{{ID: "web", Path: "web", BaseCommit: base, HeadRef: head}},
			FileFilter:    filter,
			Chains: []schema.Chain{{ID: "c", Title: "c", Intents: []schema.Intent{{
				ID: "i", Title: "i", Summary: "s",
				Changes: []schema.Change{{
					Repo: "web", File: "a.txt", Location: "1", BusinessExplanation: "e",
					OldStart: 1, NewStart: 1,
					Lines: []schema.Line{
						{Type: schema.LineDel, Content: "one"},
						{Type: schema.LineAdd, Content: "two"},
					},
				}},
			}}}},
		}
		return writeDoc(t, doc)
	}
	if code, _, errOut := runCLI(t, "validate", mk(nil), "--repo", project); code != 1 {
		t.Fatalf("未配置过滤时应报 docs 未覆盖: code=%d err=%s", code, errOut)
	}
	cfg := &schema.FileFilterConfig{Exclude: []string{"docs/**"}}
	code, _, errOut := runCLI(t, "validate", mk(cfg), "--repo", project)
	if code != 0 {
		t.Fatalf("文档 file_filter 应生效: code=%d err=%s", code, errOut)
	}
}

func TestCLIDiffReportsIntentChanges(t *testing.T) {
	before := canonicalDoc()
	after := canonicalDoc()
	after.Chains[0].Intents[0].Changes[0].BusinessExplanation = "新的解释"
	beforePath, afterPath := writeDoc(t, before), writeDoc(t, after)

	code, out, errOut := runCLI(t, "diff", beforePath, afterPath)
	if code != 0 || !strings.Contains(out, "解释内容") {
		t.Fatalf("diff: code=%d out=%s err=%s", code, out, errOut)
	}
	code, out, _ = runCLI(t, "diff", beforePath, afterPath, "--json")
	if code != 0 {
		t.Fatalf("diff --json: code=%d", code)
	}
	var rep struct {
		Intents []struct {
			Status     string   `json:"status"`
			Dimensions []string `json:"dimensions"`
		} `json:"intents"`
	}
	if err := json.Unmarshal([]byte(out), &rep); err != nil {
		t.Fatalf("diff JSON: %v (%s)", err, out)
	}
	if len(rep.Intents) != 1 || rep.Intents[0].Status != "modified" ||
		strings.Join(rep.Intents[0].Dimensions, ",") != "explanation" {
		t.Fatalf("rep = %+v", rep)
	}
	if code, _, _ := runCLI(t, "diff", beforePath); code != 2 {
		t.Fatalf("diff 参数不足应退出 2: code=%d", code)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(raw)
}

// --source-root 必须覆盖 --repo（避免点开临时重放快照），且 flag 在文件名前后都可解析。
func TestCLIRenderSourceRootOverridesRepoAnyOrder(t *testing.T) {
	_, repoDir := fixtureRepo(t)
	docPath := writeDoc(t, canonicalDoc())
	real := t.TempDir()
	outA := filepath.Join(t.TempDir(), "a.html")
	outB := filepath.Join(t.TempDir(), "b.html")
	order := []struct {
		args []string
		out  string
	}{
		{[]string{"render", docPath, "-o", outA, "--repo", repoDir, "--source-root", real}, outA},
		{[]string{"render", "--source-root", real, "--repo", repoDir, docPath, "-o", outB}, outB},
	}
	for i, c := range order {
		code, _, errOut := runCLI(t, c.args...)
		if code != 0 {
			t.Fatalf("case %d: code=%d err=%s", i, code, errOut)
		}
		html := readFile(t, c.out)
		want := "vscode://file" + filepath.ToSlash(real) + "/a.txt:1"
		if !strings.Contains(html, want) {
			t.Fatalf("case %d: 缺少源码根链接 %q", i, want)
		}
		if strings.Contains(html, "vscode://file"+filepath.ToSlash(repoDir)+"/a.txt") {
			t.Fatalf("case %d: 链接仍指向 --repo（重放快照）", i)
		}
	}
}

// 多仓 source-root 映射同样支持 flag 在位置参数之前。
func TestCLIMultiRepoSourceRootBeforeInput(t *testing.T) {
	project, docPath, _ := multiRepoProject(t)
	filled := filepath.Join(t.TempDir(), "filled.json")
	if code, _, errOut := runCLI(t, "fill", docPath, "--repo", project, "-o", filled); code != 0 {
		t.Fatalf("fill: %d %s", code, errOut)
	}
	realWeb, realAPI := t.TempDir(), t.TempDir()
	htmlPath := filepath.Join(t.TempDir(), "review.html")
	code, _, errOut := runCLI(t, "render",
		"--source-root", "web="+realWeb, "--source-root", "api="+realAPI,
		"--repo", project, filled, "-o", htmlPath)
	if code != 0 {
		t.Fatalf("render: code=%d err=%s", code, errOut)
	}
	html := readFile(t, htmlPath)
	if !strings.Contains(html, "vscode://file"+filepath.ToSlash(realWeb)+"/src/a.ts:1") {
		t.Fatalf("web 源码根未生效")
	}
	if !strings.Contains(html, "vscode://file"+filepath.ToSlash(realAPI)+"/b.go:3") {
		t.Fatalf("api 源码根未生效")
	}
}
