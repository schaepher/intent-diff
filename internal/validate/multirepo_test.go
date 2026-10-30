package validate

import (
	"path/filepath"
	"strings"
	"testing"

	"intent-review/internal/filefilter"
	"intent-review/internal/report"
	"intent-review/internal/schema"
	"intent-review/internal/testutil"
)

// multiRepoFixture 构造项目根下的 web / api 两个仓库，各自改一个文件。
func multiRepoFixture(t *testing.T) (project string, doc schema.Document) {
	t.Helper()
	project = t.TempDir()
	web := testutil.NewGitRepoAt(t, filepath.Join(project, "web"), map[string]string{"src/a.ts": "one\n"})
	api := testutil.NewGitRepoAt(t, filepath.Join(project, "api"), map[string]string{"handler.go": "package api\n\nvar A = 1\n"})
	webBase, apiBase := web.Head(), api.Head()
	web.Write("src/a.ts", "two\n")
	api.Write("handler.go", "package api\n\nvar A = 2\n")
	webHead, apiHead := web.Commit("web change"), api.Commit("api change")

	doc = schema.Document{
		SchemaVersion: schema.SchemaVersion,
		Meta:          schema.Meta{Title: "跨仓需求", BaseCommit: webBase, HeadRef: webHead},
		Repos: []schema.Repo{
			{ID: "web", Title: "前端", Path: "web", BaseCommit: webBase, HeadRef: webHead},
			{ID: "api", Title: "后端", Path: "api", BaseCommit: apiBase, HeadRef: apiHead},
		},
		Chains: []schema.Chain{{
			ID: "c1", Title: "同一需求", Intents: []schema.Intent{{
				ID: "i1", Title: "改两端", Summary: "前端入口 + 后端处理",
				Changes: []schema.Change{
					{
						Repo: "api", File: "handler.go", Location: "A", BusinessExplanation: "后端改值",
						OldStart: 3, NewStart: 3,
						Lines: []schema.Line{
							{Type: schema.LineDel, Content: "var A = 1"},
							{Type: schema.LineAdd, Content: "var A = 2"},
						},
					},
					{
						Repo: "web", File: "src/a.ts", Location: "1", BusinessExplanation: "前端改值",
						OldStart: 1, NewStart: 1,
						Lines: []schema.Line{
							{Type: schema.LineDel, Content: "one"},
							{Type: schema.LineAdd, Content: "two"},
						},
					},
				},
			}},
		}},
	}
	return project, doc
}

func TestRunMultiRepoOK(t *testing.T) {
	project, doc := multiRepoFixture(t)
	res, err := Run(doc, project, Options{})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !res.OK {
		t.Fatalf("跨仓链应通过: %+v", res.Errors)
	}
}

func TestRunMultiRepoCoverageIsPerRepo(t *testing.T) {
	project, doc := multiRepoFixture(t)
	// 在 api 仓库加一处未覆盖变更。
	api := testutil.OpenRepo(t, filepath.Join(project, "api"))
	api.Write("extra.go", "package api\n")
	doc.Repos[1].HeadRef = api.Commit("extra")
	res, err := Run(doc, project, Options{})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if res.OK {
		t.Fatalf("api 仓库的未覆盖文件应报错")
	}
	found := false
	for _, e := range res.Errors {
		if e.Code == report.CodeFinalStateMismatch && e.Repo == "api" && e.File == "extra.go" {
			found = true
		}
	}
	if !found {
		t.Fatalf("errors = %+v", res.Errors)
	}
}

// 单仓文档不应要求 change.repo；多仓文档必须声明 repo。
func TestRunMultiRepoRequiresRepoField(t *testing.T) {
	project, doc := multiRepoFixture(t)
	doc.Chains[0].Intents[0].Changes[0].Repo = ""
	res, err := Run(doc, project, Options{})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if res.OK || !strings.Contains(strings.ToLower(res.Errors[0].Message), "repo") {
		t.Fatalf("errors = %+v", res.Errors)
	}
}

func TestRunMultiRepoMissingRepoDir(t *testing.T) {
	project, doc := multiRepoFixture(t)
	doc.Repos[0].Path = "missing"
	if _, err := Run(doc, project, Options{}); err == nil {
		t.Fatalf("缺失仓库目录应返回用法/IO 错误")
	}
}

// 文档内 file_filter 由调用方合并后传入，覆盖检查应跳过其排除项。
func TestRunMultiRepoFilterAppliesToAllRepos(t *testing.T) {
	project, doc := multiRepoFixture(t)
	web := testutil.OpenRepo(t, filepath.Join(project, "web"))
	web.Write("web_test.go", "test\n")
	doc.Repos[0].HeadRef = web.Commit("test file")
	res, err := Run(doc, project, Options{})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !res.OK {
		t.Fatalf("默认排除 *_test.go 应生效: %+v", res.Errors)
	}
}

// 覆盖检查被过滤跳过的路径要回传（页面展示评审范围），并带命中的规则。
func TestRunReportsExcludedPathsAndRules(t *testing.T) {
	project, doc := multiRepoFixture(t)
	web := testutil.OpenRepo(t, filepath.Join(project, "web"))
	web.Write("src/a.test.tsx", "test\n")
	doc.Repos[0].HeadRef = web.Commit("test file")
	opt := Options{Files: filefilter.New([]string{"*.test.tsx", "*_test.go"})}
	res, err := Run(doc, project, opt)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !res.OK {
		t.Fatalf("errors = %+v", res.Errors)
	}
	if strings.Join(res.ExcludeRules, ",") != "*.test.tsx,*_test.go" {
		t.Fatalf("rules = %v", res.ExcludeRules)
	}
	if len(res.Excluded) != 1 || res.Excluded[0].Repo != "web" ||
		res.Excluded[0].File != "src/a.test.tsx" || res.Excluded[0].Rule != "*.test.tsx" {
		t.Fatalf("excluded = %+v", res.Excluded)
	}
}
