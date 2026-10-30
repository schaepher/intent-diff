package render

import (
	"html/template"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
)

// fileLink 生成 VS Code 跳转链接：vscode://file/<绝对路径>:<行号>。
// sourceRoot 为空（未指定真实源码根）时不生成链接，避免误跳到重放用的临时快照。
func fileLink(sourceRoot, file string, line int) template.URL {
	if sourceRoot == "" || file == "" {
		return ""
	}
	abs := filepath.Join(sourceRoot, filepath.FromSlash(file))
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
	if root == "" || file == "" {
		return false
	}
	_, err := os.Stat(filepath.Join(root, filepath.FromSlash(file)))
	return err != nil
}

// changeLine 返回文件链接应指向的行号：新状态起始行；删除文件退回旧状态行。
func changeLine(oldStart, newStart int) int {
	if newStart > 0 {
		return newStart
	}
	return oldStart
}
