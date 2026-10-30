package main

import (
	"path/filepath"
	"testing"

	"intent-review/internal/schema"
)

func singleDoc() schema.Document {
	return schema.Document{
		SchemaVersion: schema.SchemaVersion,
		Meta:          schema.Meta{Title: "t", BaseCommit: "a", HeadRef: "b"},
	}
}

func multiDoc(project string) schema.Document {
	return schema.Document{
		SchemaVersion: schema.SchemaVersion,
		Meta:          schema.Meta{Title: "t"},
		Repos: []schema.Repo{
			{ID: "web", Path: "web", BaseCommit: "a", HeadRef: "b"},
			{ID: "api", Path: "api", BaseCommit: "a", HeadRef: "b", SourceRoot: "src-of-api"},
		},
	}
}

func TestResolveSourceRootsDefaults(t *testing.T) {
	project := t.TempDir()
	roots, err := resolveSourceRoots(multiDoc(project), project, nil)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if roots["web"] != filepath.Join(project, "web") {
		t.Fatalf("web root = %q", roots["web"])
	}
	// repos[].source_root 优先于 checkout 路径。
	if roots["api"] != filepath.Join(project, "src-of-api") {
		t.Fatalf("api root = %q", roots["api"])
	}
}

func TestResolveSourceRootsOverrides(t *testing.T) {
	project := t.TempDir()
	real := filepath.Join(t.TempDir(), "real-web")
	roots, err := resolveSourceRoots(multiDoc(project), project, []string{"web=" + real})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if roots["web"] != real {
		t.Fatalf("web root = %q, want %q", roots["web"], real)
	}
	// 单仓：--source-root <path>；仓库根为空时也能解析。
	single, err := resolveSourceRoots(singleDoc(), "", []string{real})
	if err != nil {
		t.Fatalf("single resolve: %v", err)
	}
	if single[""] != real {
		t.Fatalf("single root = %q", single[""])
	}
	// 未指定 --repo 且没有 flag：不生成链接。
	empty, err := resolveSourceRoots(singleDoc(), "", nil)
	if err != nil || len(empty) != 0 {
		t.Fatalf("empty = %v err=%v", empty, err)
	}
}

func TestResolveSourceRootsErrors(t *testing.T) {
	project := t.TempDir()
	if _, err := resolveSourceRoots(multiDoc(project), project, []string{"no-equals"}); err == nil {
		t.Fatalf("多仓 --source-root 缺少 id=path 应报错")
	}
	if _, err := resolveSourceRoots(multiDoc(project), project, []string{"nope=/tmp/x"}); err == nil {
		t.Fatalf("未声明仓库应报错")
	}
	if _, err := resolveSourceRoots(multiDoc(project), project, []string{"=x"}); err == nil {
		t.Fatalf("空 id 应报错")
	}
}
