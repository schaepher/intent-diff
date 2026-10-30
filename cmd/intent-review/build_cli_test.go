package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"intent-review/internal/schema"
	"intent-review/internal/testutil"
)

// build 是统一生成入口：fill → validate → render，产物固定，摘要含评审范围与更新影响。
func TestCLIBuildArtifactsScopeAndSummary(t *testing.T) {
	project, docPath, doc := multiRepoProject(t)
	// 文档级过滤规则：新增一个会被跳过的测试文件，验证 build 每次都读取配置并展示。
	web := testutil.OpenRepo(t, filepath.Join(project, "web"))
	web.Write("src/page.test.tsx", "test\n")
	doc.Repos[0].HeadRef = web.Commit("test file")
	doc.FileFilter = &schema.FileFilterConfig{Exclude: []string{"*.test.tsx", "*.md"}}
	docPath = writeDoc(t, doc)

	outDir := filepath.Join(t.TempDir(), "artifacts")
	code, _, errOut := runCLI(t, "build", docPath, "--repo", project, "--out-dir", outDir)
	if code != 0 {
		t.Fatalf("build: code=%d err=%s", code, errOut)
	}
	for _, name := range []string{"review.filled.json", "review.html"} {
		if _, err := os.Stat(filepath.Join(outDir, name)); err != nil {
			t.Fatalf("产物 %s 缺失: %v", name, err)
		}
	}
	for _, want := range []string{"首次生成", "评审范围：排除", "跳过 1 个文件", "产物已写入"} {
		if !strings.Contains(errOut, want) {
			t.Fatalf("摘要缺少 %q:\n%s", want, errOut)
		}
	}
	html := readFile(t, filepath.Join(outDir, "review.html"))
	for _, want := range []string{"评审范围：排除", "<code>*.test.tsx</code>", "web:src/page.test.tsx"} {
		if !strings.Contains(html, want) {
			t.Fatalf("页面评审范围缺少 %q", want)
		}
	}

	// 第二次更新：只改解释，摘要应指出受影响意图与维度。
	doc.Chains[0].Intents[0].Changes[0].BusinessExplanation = "更新后的后端解释"
	code, _, errOut = runCLI(t, "build", writeDoc(t, doc), "--repo", project, "--out-dir", outDir)
	if code != 0 {
		t.Fatalf("rebuild: code=%d err=%s", code, errOut)
	}
	for _, want := range []string{"本次更新影响 1 个意图", "解释内容"} {
		if !strings.Contains(errOut, want) {
			t.Fatalf("更新摘要缺少 %q:\n%s", want, errOut)
		}
	}

	// clean 只删产物目录；业务目录必须拒绝。
	if code, _, errOut := runCLI(t, "clean", "--out-dir", outDir); code != 0 {
		t.Fatalf("clean: code=%d err=%s", code, errOut)
	}
	if _, err := os.Stat(outDir); !os.IsNotExist(err) {
		t.Fatalf("产物目录应被删除")
	}
	if code, _, _ := runCLI(t, "clean", "--out-dir", project); code != 2 {
		t.Fatalf("拒绝清理业务目录: code=%d", code)
	}
	if _, err := os.Stat(project); err != nil {
		t.Fatalf("业务目录不应被删除: %v", err)
	}
}

// 校验失败仍更新页面（诊断上浮），退出码为 1。
func TestCLIBuildWritesPageOnValidationFailure(t *testing.T) {
	repo, repoDir := fixtureRepo(t)
	repo.Write("b.txt", "extra\n")
	head := repo.Commit("extra file")
	doc := canonicalDoc()
	doc.Meta.HeadRef = head
	outDir := filepath.Join(t.TempDir(), "artifacts")
	code, _, errOut := runCLI(t, "build", writeDoc(t, doc), "--repo", repoDir, "--out-dir", outDir)
	if code != 1 {
		t.Fatalf("code=%d err=%s", code, errOut)
	}
	if !strings.Contains(errOut, "没有被任何意图覆盖") || !strings.Contains(errOut, "校验未通过") {
		t.Fatalf("stderr = %s", errOut)
	}
	if _, err := os.Stat(filepath.Join(outDir, "review.html")); err != nil {
		t.Fatalf("校验失败也应写页面: %v", err)
	}
}

func TestCLIBuildRequiresRepo(t *testing.T) {
	docPath := writeDoc(t, canonicalDoc())
	if code, _, errOut := runCLI(t, "build", docPath); code != 2 || !strings.Contains(errOut, "--repo") {
		t.Fatalf("code=%d err=%s", code, errOut)
	}
}
