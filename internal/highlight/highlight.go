// Package highlight 基于 chroma 做语法高亮，未知语言降级为纯文本。
//
// 高亮按“整列文本”完成再切行：多行字符串/块注释的跨行状态才不会丢失。
package highlight

import (
	"html"
	"path/filepath"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
)

// Highlighter 是无状态的（lexer 由 chroma 内部缓存）。
type Highlighter struct{}

// New 返回高亮器。
func New() *Highlighter { return &Highlighter{} }

// Lines 返回逐行 HTML（已转义）。ok=false 表示没有匹配到 lexer，
// 已降级为纯文本（调用方应上报 HIGHLIGHT_FALLBACK 警告）。
func (h *Highlighter) Lines(path string, lines []string) ([]string, bool) {
	text := strings.Join(lines, "\n")
	if text == "" {
		return make([]string, len(lines)), true
	}
	lexer := lexers.Match(filepath.Base(path))
	if lexer == nil {
		lexer = lexers.Analyse(text)
	}
	if lexer == nil {
		return plain(lines), false
	}
	htmlLines, err := tokenise(chroma.Coalesce(lexer), text, len(lines))
	if err != nil {
		return plain(lines), false
	}
	return htmlLines, true
}

func plain(lines []string) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = html.EscapeString(l)
	}
	return out
}

func tokenise(lexer chroma.Lexer, text string, want int) ([]string, error) {
	iter, err := lexer.Tokenise(nil, text)
	if err != nil {
		return nil, err
	}
	out := make([]string, want)
	var buf strings.Builder
	line := 0
	flush := func() {
		if line < want {
			out[line] = buf.String()
		}
		buf.Reset()
	}
	for tok := iter(); tok != chroma.EOF; tok = iter() {
		class := tokenClass(tok.Type)
		parts := strings.Split(tok.Value, "\n")
		for i, part := range parts {
			if i > 0 {
				flush()
				line++
			}
			if part == "" {
				continue
			}
			if class == "" {
				buf.WriteString(html.EscapeString(part))
				continue
			}
			buf.WriteString(`<span class="tok-` + class + `">`)
			buf.WriteString(html.EscapeString(part))
			buf.WriteString(`</span>`)
		}
	}
	flush()
	return out, nil
}

func tokenClass(t chroma.TokenType) string {
	switch {
	case t.InCategory(chroma.Comment):
		return "c"
	case t.InCategory(chroma.Keyword):
		switch t {
		case chroma.KeywordType:
			return "kt"
		case chroma.KeywordConstant:
			return "kc"
		case chroma.KeywordNamespace:
			return "kn"
		}
		return "k"
	case t.InCategory(chroma.String):
		return "s"
	case t.InCategory(chroma.LiteralNumber):
		return "m"
	case t.InCategory(chroma.Name):
		switch t {
		case chroma.NameFunction:
			return "nf"
		case chroma.NameClass, chroma.NameNamespace, chroma.NameException:
			return "nc"
		case chroma.NameBuiltin:
			return "nb"
		case chroma.NameAttribute:
			return "na"
		case chroma.NameConstant:
			return "no"
		}
		return "n"
	case t.InCategory(chroma.Operator):
		return "o"
	case t.InCategory(chroma.Punctuation):
		return "p"
	case t.InCategory(chroma.Generic):
		return "g"
	}
	return ""
}
