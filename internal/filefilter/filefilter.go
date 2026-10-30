// Package filefilter 决定哪些仓库路径不在评审范围内。
//
// 只有一个消费者语义：validate 的「base..head 覆盖检查」跳过命中排除列表的路径。
// 重放与最终状态比对**不受影响**——排除列表不是「允许漏改」的开关，
// 已经声明成 change 的文件仍会逐行校验。
//
// 模式语义（gitignore 风格，确定性实现，无外部依赖）：
//   - 不含 "/" 的模式匹配文件 basename（任意深度），如 *_test.go；
//   - 含 "/" 的模式锚定仓库根，如 internal/**；
//   - ** 匹配任意多级目录（含 0 级），* 不跨 /，? 匹配单个字符，[abc]/[!abc] 字符集；
//   - 结尾 "/" 等价于追加 "**"（表示整个目录）；
//   - 反斜杠统一当作路径分隔符归一化（兼容 Windows 书写习惯）。
package filefilter

import (
	"regexp"
	"strings"
)

// DefaultPatterns 是内置排除列表：测试文件与第三方依赖目录。
// 用户可通过配置文件或 --exclude / --unexclude 增删（见 config.go 与 README）。
var DefaultPatterns = []string{
	"*_test.go",
	"**/vendor/**",
	"**/node_modules/**",
}

// item 是一个已编译的排除模式。
type item struct {
	pattern string
	re      *regexp.Regexp
	// baseOnly 表示模式不含 "/"，只与路径的 basename 比较。
	baseOnly bool
}

// List 是一组排除模式（有序、去重）。零值不可用，请用 New/Default。
type List struct {
	items []item
}

// Default 返回内置排除列表的副本。
func Default() *List { return New(DefaultPatterns) }

// New 用给定模式构造列表（空白项忽略，重复项保留首个）。
func New(patterns []string) *List {
	l := &List{}
	return l.Add(patterns...)
}

// Add 追加模式（返回自身便于链式调用）。空模式与重复模式被忽略。
func (l *List) Add(patterns ...string) *List {
	if l == nil {
		return l
	}
	for _, p := range patterns {
		p = normalizePattern(p)
		if p == "" || l.has(p) {
			continue
		}
		l.items = append(l.items, item{
			pattern:  p,
			re:       compile(p),
			baseOnly: !strings.Contains(p, "/"),
		})
	}
	return l
}

// Remove 按模式字面值删除（不存在时是空操作）。删除的优先级高于追加：
// 同一层配置里 unexclude 先于 exclude 生效（见 Resolve 与 CLI build）。
func (l *List) Remove(patterns ...string) *List {
	if l == nil {
		return l
	}
	for _, p := range patterns {
		p = normalizePattern(p)
		for i := range l.items {
			if l.items[i].pattern == p {
				l.items = append(l.items[:i], l.items[i+1:]...)
				break
			}
		}
	}
	return l
}

// Patterns 返回当前模式（有序副本），用于输出与调试。
func (l *List) Patterns() []string {
	if l == nil {
		return nil
	}
	out := make([]string, 0, len(l.items))
	for _, it := range l.items {
		out = append(out, it.pattern)
	}
	return out
}

// Len 返回模式数量。
func (l *List) Len() int {
	if l == nil {
		return 0
	}
	return len(l.items)
}

// Match 判断仓库相对路径是否被排除。
func (l *List) Match(p string) bool {
	_, ok := l.MatchReason(p)
	return ok
}

// MatchReason 返回命中的排除模式（未命中时 ok=false），用于在页面上说明“为什么跳过”。
func (l *List) MatchReason(p string) (rule string, ok bool) {
	if l == nil || len(l.items) == 0 {
		return "", false
	}
	p = normalizePath(p)
	base := p[strings.LastIndex(p, "/")+1:]
	for _, it := range l.items {
		target := p
		if it.baseOnly {
			target = base
		}
		if it.re.MatchString(target) {
			return it.pattern, true
		}
	}
	return "", false
}

func (l *List) has(pattern string) bool {
	for _, it := range l.items {
		if it.pattern == pattern {
			return true
		}
	}
	return false
}

func normalizePath(p string) string {
	return strings.ReplaceAll(strings.TrimSpace(p), "\\", "/")
}

func normalizePattern(p string) string {
	p = normalizePath(p)
	for strings.HasPrefix(p, "./") {
		p = p[2:]
	}
	p = strings.TrimPrefix(p, "/")
	if p == "" {
		return ""
	}
	if strings.HasSuffix(p, "/") {
		p += "**"
	}
	return p
}

// compile 把 glob 翻译成锚定的正则。
func compile(pattern string) *regexp.Regexp {
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(pattern); {
		switch c := pattern[i]; c {
		case '*':
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				if i+2 < len(pattern) && pattern[i+2] == '/' {
					b.WriteString("(?:.*/)?") // **/ 允许 0 级目录
					i += 3
					continue
				}
				b.WriteString(".*")
				i += 2
				continue
			}
			b.WriteString("[^/]*")
			i++
		case '?':
			b.WriteString("[^/]")
			i++
		case '[':
			cls, n := charClass(pattern[i:])
			b.WriteString(cls)
			i += n
		default:
			b.WriteString(regexp.QuoteMeta(string(c)))
			i++
		}
	}
	b.WriteString("$")
	return regexp.MustCompile(b.String())
}

// charClass 翻译 [...] 字符集；未闭合时把 "[" 当字面量（返回消耗 1 字节）。
func charClass(s string) (string, int) {
	end := -1
	for i := 1; i < len(s); i++ {
		if s[i] == ']' && i > 1 {
			end = i
			break
		}
	}
	if end < 0 {
		return `\[`, 1
	}
	body := s[1:end]
	neg := strings.HasPrefix(body, "!")
	if neg {
		body = body[1:]
	}
	if body == "" {
		return `\[`, 1
	}
	var b strings.Builder
	b.WriteByte('[')
	if neg {
		b.WriteByte('^')
	}
	for _, r := range body {
		switch r {
		case '\\', ']', '^':
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	b.WriteByte(']')
	return b.String(), end + 1
}
