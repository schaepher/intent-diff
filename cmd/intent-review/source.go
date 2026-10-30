package main

import (
	"flag"
	"fmt"
	"path/filepath"
	"strings"

	"intent-review/internal/schema"
)

// addSourceRootFlag 注册可重复的 --source-root：单仓写 <path>，多仓写 <repo-id>=<path>。
func addSourceRootFlag(fs *flag.FlagSet, f *stringList) {
	fs.Var(f, "source-root", "VS Code 跳转源码根（可重复；多仓用 <repo-id>=<path>）")
}

// resolveSourceRoots 计算每个仓库的源码根（绝对路径）：
// CLI --source-root > repos[].source_root > 仓库 checkout 路径（项目根 + repos[].path）。
// 项目根为空且未显式指定时不生成链接，避免把重放快照当成源码。
func resolveSourceRoots(doc schema.Document, repoDir string, flags []string) (map[string]string, error) {
	roots := map[string]string{}
	if repoDir != "" {
		for _, id := range doc.RepoIDs() {
			p := "."
			if r, ok := doc.RepoByID(id); ok {
				switch {
				case strings.TrimSpace(r.SourceRoot) != "":
					p = r.SourceRoot
				case strings.TrimSpace(r.Path) != "":
					p = r.Path
				}
			}
			abs, err := absPath(repoDir, p)
			if err != nil {
				return nil, err
			}
			roots[id] = abs
		}
	}
	for _, v := range flags {
		id, p, err := splitSourceRoot(doc, v)
		if err != nil {
			return nil, err
		}
		if doc.MultiRepo() {
			if _, ok := doc.RepoByID(id); !ok {
				return nil, fmt.Errorf("--source-root 引用了未声明的仓库 %q", id)
			}
		}
		base := repoDir
		if base == "" {
			base = "."
		}
		abs, err := absPath(base, p)
		if err != nil {
			return nil, err
		}
		roots[id] = abs
	}
	return roots, nil
}

func splitSourceRoot(doc schema.Document, v string) (id, path string, err error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return "", "", fmt.Errorf("--source-root 不能为空")
	}
	if cmdArg, rest, ok := strings.Cut(v, "="); ok {
		if strings.TrimSpace(cmdArg) == "" || strings.TrimSpace(rest) == "" {
			return "", "", fmt.Errorf("--source-root %q 应为 <repo-id>=<path>", v)
		}
		return cmdArg, rest, nil
	}
	if doc.MultiRepo() {
		return "", "", fmt.Errorf("多仓评审的 --source-root 需要 <repo-id>=<path>（收到 %q）", v)
	}
	return "", v, nil
}

func absPath(base, p string) (string, error) {
	if filepath.IsAbs(p) {
		return filepath.Clean(p), nil
	}
	return filepath.Abs(filepath.Join(base, p))
}
