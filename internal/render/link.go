package render

import (
	"html/template"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"intent-review/internal/schema"
)

// sourcePath 把仓库相对文件路径解析到源码根下的绝对路径。
// 绝对路径、Windows 盘符、清理后越界（..）一律拒绝，并再次确认结果仍在 root 内，
// 避免文件链接绕过 --source-root 指向的源码目录。
func sourcePath(sourceRoot, file string) (string, bool) {
	rel, ok := schema.CleanRelPath(file)
	if !ok || strings.TrimSpace(sourceRoot) == "" {
		return "", false
	}
	root, err := filepath.Abs(sourceRoot)
	if err != nil {
		return "", false
	}
	abs := filepath.Join(root, filepath.FromSlash(rel))
	if !within(root, abs) {
		return "", false
	}
	return abs, true
}

func within(root, p string) bool {
	rel, err := filepath.Rel(root, p)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// fileLink 生成 VS Code 跳转链接：vscode://file/<绝对路径>:<行号>。
// 源码根为空或路径非法时不生成链接（避免误跳到重放快照或越界目录）。
func fileLink(sourceRoot, file string, line int) template.URL {
	abs, ok := sourcePath(sourceRoot, file)
	if !ok {
		return ""
	}
	u := url.URL{Scheme: "vscode", Host: "file", Path: filepath.ToSlash(abs)}
	s := u.String()
	if line > 0 {
		s += ":" + strconv.Itoa(line)
	}
	return template.URL(s) // 由 url.URL 转义后构造，允许非 http scheme
}

// withLineLinks 给新状态一侧的每个行号加上精确行跳转链接。
// 旧状态（State(i-1)）不是本地当前文件，故意不加链接，避免误认成历史快照。
func withLineLinks(rows []Row, root, file string) {
	if root == "" || file == "" {
		return
	}
	for i := range rows {
		if rows[i].Right.No > 0 {
			rows[i].Right.Link = fileLink(root, file, rows[i].Right.No)
		}
	}
}

// sourceFileMissing 判断本地源码根下是否真的存在该文件（用于页面提示）。
func sourceFileMissing(root, file string) bool {
	abs, ok := sourcePath(root, file)
	if !ok {
		return false
	}
	_, err := os.Stat(abs)
	return err != nil
}

// changeLine 返回文件链接应指向的行号：新状态起始行；删除文件退回旧状态行。
func changeLine(oldStart, newStart int) int {
	if newStart > 0 {
		return newStart
	}
	return oldStart
}
