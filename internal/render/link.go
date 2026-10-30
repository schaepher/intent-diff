package render

import (
	"html/template"
	"net/url"
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

// changeLine 返回文件链接应指向的行号：新状态起始行；删除文件退回旧状态行。
func changeLine(oldStart, newStart int) int {
	if newStart > 0 {
		return newStart
	}
	return oldStart
}
