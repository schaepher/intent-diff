package schema

import (
	"path"
	"strings"
)

// CleanRelPath 校验并归一化仓库相对路径。
//
// 拒绝：空路径、绝对路径（POSIX 与 Windows 盘符）、清理后仍越界（"."/".."/"../x"）。
// 返回以 "/" 分隔的干净相对路径；ok=false 表示路径非法。
// 该函数是 fill/validate 读取文件与 render 生成源码链接的共同入口，
// 防止 link 通过 ".." 或绝对路径绕过指定的源码根。
func CleanRelPath(p string) (string, bool) {
	p = strings.TrimSpace(strings.ReplaceAll(p, "\\", "/"))
	if p == "" || strings.HasPrefix(p, "/") || hasDriveLetter(p) {
		return "", false
	}
	clean := path.Clean(p)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", false
	}
	return clean, true
}

func hasDriveLetter(p string) bool {
	if len(p) < 2 || p[1] != ':' {
		return false
	}
	c := p[0]
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')
}
