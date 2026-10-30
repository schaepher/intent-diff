// Package testutil 提供测试用的临时 git 仓库夹具。
package testutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Repo 是一个临时 git 仓库。
type Repo struct {
	T   *testing.T
	Dir string
}

// NewGitRepo 创建临时仓库并提交初始文件（map：仓库相对路径 → 内容）。
func NewGitRepo(t *testing.T, files map[string]string) *Repo {
	t.Helper()
	return NewGitRepoAt(t, t.TempDir(), files)
}

// NewGitRepoAt 在指定目录（不存在则创建）初始化仓库并提交初始文件。
// 用于跨仓评审测试（项目根下的多个子仓库）。
func NewGitRepoAt(t *testing.T, dir string, files map[string]string) *Repo {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	r := &Repo{T: t, Dir: dir}
	r.git("init", "-q", "-b", "main")
	r.git("config", "user.name", "test")
	r.git("config", "user.email", "test@example.com")
	r.git("config", "commit.gpgsign", "false")
	for p, content := range files {
		r.Write(p, content)
	}
	r.Commit("init")
	return r
}

// OpenRepo 复用已存在的仓库目录（不执行 init/初始提交）。
func OpenRepo(t *testing.T, dir string) *Repo {
	t.Helper()
	return &Repo{T: t, Dir: dir}
}

// Write 写入工作区文件。
func (r *Repo) Write(p, content string) {
	r.T.Helper()
	full := filepath.Join(r.Dir, p)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		r.T.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		r.T.Fatalf("write %s: %v", p, err)
	}
}

// Remove 删除工作区文件。
func (r *Repo) Remove(p string) {
	r.T.Helper()
	if err := os.Remove(filepath.Join(r.Dir, p)); err != nil {
		r.T.Fatalf("remove %s: %v", p, err)
	}
}

// Commit 提交当前工作区全部改动并返回 commit ID。
func (r *Repo) Commit(msg string) string {
	r.T.Helper()
	r.git("add", "-A")
	r.git("commit", "-q", "-m", msg)
	return r.git("rev-parse", "HEAD")
}

// Head 返回当前 HEAD commit ID。
func (r *Repo) Head() string { return r.git("rev-parse", "HEAD") }

func (r *Repo) git(args ...string) string {
	r.T.Helper()
	cmd := exec.Command("git", append([]string{"-C", r.Dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		r.T.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}
