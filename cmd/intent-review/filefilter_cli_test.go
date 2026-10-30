package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"intent-review/internal/schema"
	"intent-review/internal/testutil"
)

// filterFixture 构造“改 a.txt + 额外文件 extra”的仓库与只覆盖 a.txt 的文档。
func filterFixture(t *testing.T, extraPath, extraContent string) (*testutil.Repo, schema.Document) {
	t.Helper()
	repo := testutil.NewGitRepo(t, map[string]string{"a.txt": "one\n"})
	base := repo.Head()
	repo.Write("a.txt", "two\n")
	repo.Write(extraPath, extraContent)
	head := repo.Commit("change a.txt + extra")
	doc := schema.Document{
		SchemaVersion: schema.SchemaVersion,
		Meta:          schema.Meta{Title: "filter", BaseCommit: base, HeadRef: head},
		Chains: []schema.Chain{{ID: "c1", Title: "链", Intents: []schema.Intent{{
			ID: "i1", Title: "改 a.txt", Summary: "整行替换",
			Changes: []schema.Change{{
				File: "a.txt", Location: "第 1 行", BusinessExplanation: "替换",
				OldStart: 1, NewStart: 1,
				Lines: []schema.Line{
					{Type: schema.LineDel, Content: "one"},
					{Type: schema.LineAdd, Content: "two"},
				},
			}},
		}}}},
	}
	return repo, doc
}

func writeRepoConfig(t *testing.T, repoDir, body string) string {
	t.Helper()
	path := filepath.Join(repoDir, ".intent-review.json")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("写配置失败: %v", err)
	}
	return path
}

// 配置文件里新增排除项后，被排除文件不再要求意图覆盖。
func TestCLIValidateConfigExclude(t *testing.T) {
	repo, doc := filterFixture(t, "docs/guide.md", "# guide\n")
	docPath := writeDoc(t, doc)

	code, _, errOut := runCLI(t, "validate", docPath, "--repo", repo.Dir)
	if code != 1 || !strings.Contains(errOut, "没有被任何意图覆盖") {
		t.Fatalf("默认应报未覆盖: code=%d err=%s", code, errOut)
	}

	writeRepoConfig(t, repo.Dir, `{"file_filter": {"exclude": ["docs/**"]}}`)
	code, _, errOut = runCLI(t, "validate", docPath, "--repo", repo.Dir)
	if code != 0 {
		t.Fatalf("配置排除后应通过: code=%d err=%s", code, errOut)
	}

	// --config 显式指定仓库外的配置文件（存在时优先于仓库内配置）。
	if err := os.Remove(filepath.Join(repo.Dir, ".intent-review.json")); err != nil {
		t.Fatalf("删除仓库内配置失败: %v", err)
	}
	outside := filepath.Join(t.TempDir(), "cfg.json")
	if err := os.WriteFile(outside, []byte(`{"file_filter": {"exclude": ["docs/**"]}}`), 0o644); err != nil {
		t.Fatalf("写配置失败: %v", err)
	}
	if code, _, errOut := runCLI(t, "validate", docPath, "--repo", repo.Dir, "--config", outside); code != 0 {
		t.Fatalf("显式配置应可用: code=%d err=%s", code, errOut)
	}
}

// --exclude 可重复，且可与位置参数交错。
func TestCLIExcludeFlagRepeatable(t *testing.T) {
	repo, doc := filterFixture(t, "notes/todo.md", "- todo\n")
	docPath := writeDoc(t, doc)

	code, _, errOut := runCLI(t, "validate", "--exclude", "docs/**", "--exclude", "notes/**", docPath, "--repo", repo.Dir)
	if code != 0 {
		t.Fatalf("重复 --exclude 应互相叠加: code=%d err=%s", code, errOut)
	}
}

// --unexclude 删除默认排除项后，测试文件重新需要覆盖。
func TestCLIUnexcludeFlagRestoresCoverage(t *testing.T) {
	repo, doc := filterFixture(t, "a_test.go", "package main\n")
	docPath := writeDoc(t, doc)

	code, _, errOut := runCLI(t, "validate", docPath, "--repo", repo.Dir)
	if code != 0 {
		t.Fatalf("测试文件默认应被排除: code=%d err=%s", code, errOut)
	}
	code, _, errOut = runCLI(t, "validate", docPath, "--repo", repo.Dir, "--unexclude", "*_test.go")
	if code != 1 || !strings.Contains(errOut, "a_test.go") {
		t.Fatalf("unexclude 后应报未覆盖: code=%d err=%s", code, errOut)
	}
}

// 配置写错（未知字段/非法 JSON）必须报用法错误，不能静默忽略。
func TestCLIInvalidConfigFails(t *testing.T) {
	repo, doc := filterFixture(t, "docs/guide.md", "# guide\n")
	docPath := writeDoc(t, doc)

	writeRepoConfig(t, repo.Dir, `{"file_filter": {"exclud": ["docs/**"]}}`)
	code, _, errOut := runCLI(t, "validate", docPath, "--repo", repo.Dir)
	if code != 2 || !strings.Contains(errOut, "unknown field") {
		t.Fatalf("未知配置字段应退出 2: code=%d err=%s", code, errOut)
	}

	writeRepoConfig(t, repo.Dir, `{"file_filter": `)
	if code, _, errOut := runCLI(t, "validate", docPath, "--repo", repo.Dir); code != 2 {
		t.Fatalf("非法 JSON 应退出 2: code=%d err=%s", code, errOut)
	}

	// 显式 --config 指向不存在的文件同样退出 2。
	if code, _, _ := runCLI(t, "validate", docPath, "--repo", repo.Dir, "--config", filepath.Join(repo.Dir, "nope.json")); code != 2 {
		t.Fatalf("缺失的 --config 应退出 2: code=%d", code)
	}
}

// render 的附带校验同样使用排除列表；没有 --repo 时不需要（也不读）配置。
func TestCLIRenderUsesFilter(t *testing.T) {
	repo, doc := filterFixture(t, "a_test.go", "package main\n")
	docPath := writeDoc(t, doc)
	htmlPath := filepath.Join(t.TempDir(), "review.html")

	code, _, errOut := runCLI(t, "render", docPath, "-o", htmlPath, "--repo", repo.Dir)
	if code != 0 || strings.Contains(errOut, "没有被任何意图覆盖") {
		t.Fatalf("render 应跳过被排除文件: code=%d err=%s", code, errOut)
	}

	code, _, errOut = runCLI(t, "render", docPath, "-o", htmlPath, "--repo", repo.Dir, "--unexclude", "*_test.go")
	if code != 0 || !strings.Contains(errOut, "没有被任何意图覆盖") {
		t.Fatalf("unexclude 后 render 应上浮未覆盖诊断: code=%d err=%s", code, errOut)
	}

	// 无 --repo 时不加载配置：坏配置也不能挡住渲染。
	writeRepoConfig(t, repo.Dir, `{"file_filter": `)
	if code, _, errOut := runCLI(t, "render", docPath, "-o", htmlPath); code != 0 {
		t.Fatalf("无 --repo 时不应读配置: code=%d err=%s", code, errOut)
	}
}
