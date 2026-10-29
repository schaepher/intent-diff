// Package replay 用 git 中的 base 提交重放意图链，维护逐意图的代码状态。
//
// 重放规则见设计文档 §7.1：同一链内按 intents 顺序、changes 数组顺序
// 应用；每个 change 的 old_start 指向“应用它时的当前状态”。
package replay

import (
	"bytes"
	"fmt"
	"os/exec"
	"path"
	"strings"
)

// Repo 是一个本地 git 仓库（工作区）。内容一律从提交读取，不受工作区脏文件影响。
type Repo struct {
	// Dir 是仓库根（git rev-parse --show-toplevel）。
	Dir string
}

// Open 校验 dir 位于 git 工作区内并返回 Repo。
func Open(dir string) (*Repo, error) {
	if dir == "" {
		dir = "."
	}
	out, err := git(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, fmt.Errorf("%s 不是 git 仓库: %w", dir, err)
	}
	return &Repo{Dir: strings.TrimSpace(out)}, nil
}

// Resolve 把 ref/commit 解析成完整的 commit ID。
func (r *Repo) Resolve(ref string) (string, error) {
	if strings.TrimSpace(ref) == "" {
		return "", fmt.Errorf("ref 为空")
	}
	out, err := git(r.Dir, "rev-parse", "--verify", ref+"^{commit}")
	if err != nil {
		return "", fmt.Errorf("无法解析提交 %q: %w", ref, err)
	}
	return strings.TrimSpace(out), nil
}

// ReadFile 读取 commit 下某个仓库相对路径的内容。
// exists=false 表示该提交中不存在该路径（与空文件区分）。
func (r *Repo) ReadFile(commit, p string) (lines []string, exists bool, err error) {
	if err := checkRelPath(p); err != nil {
		return nil, false, err
	}
	spec := commit + ":" + p
	if _, err := git(r.Dir, "cat-file", "-e", spec); err != nil {
		return nil, false, nil
	}
	out, err := git(r.Dir, "cat-file", "blob", spec)
	if err != nil {
		return nil, false, err
	}
	return SplitLines(out), true, nil
}

// ChangedPaths 返回 base→head 之间内容变化的路径（--no-renames：重命名会拆成
// 旧路径删除 + 新路径新增，便于发现“未声明的重命名”）。
func (r *Repo) ChangedPaths(base, head string) ([]string, error) {
	out, err := git(r.Dir, "diff", "--name-only", "--no-renames", "-z", base, head)
	if err != nil {
		return nil, err
	}
	parts := strings.Split(out, "\x00")
	paths := make([]string, 0, len(parts))
	for _, p := range parts {
		if p == "" {
			continue
		}
		paths = append(paths, p)
	}
	return paths, nil
}

func checkRelPath(p string) error {
	if p == "" {
		return fmt.Errorf("文件路径为空")
	}
	if strings.HasPrefix(p, "/") || path.IsAbs(p) {
		return fmt.Errorf("文件路径 %q 必须是仓库相对路径", p)
	}
	clean := path.Clean(p)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return fmt.Errorf("文件路径 %q 越出仓库根", p)
	}
	return nil
}

func git(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("%s", msg)
	}
	return stdout.String(), nil
}

// SplitLines 把文件内容切分成行：末尾换行不产生额外的空行，
// 空文件得到空切片（不是 [" "]）。
func SplitLines(content string) []string {
	if content == "" {
		return nil
	}
	s := strings.Split(content, "\n")
	if len(s) > 0 && s[len(s)-1] == "" {
		s = s[:len(s)-1]
	}
	return s
}

// JoinLines 反向拼回文件内容（保留末尾换行）。
func JoinLines(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}
