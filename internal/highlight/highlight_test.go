package highlight

import (
	"strings"
	"testing"
)

func TestLinesGo(t *testing.T) {
	h := New()
	lines := []string{"package main", "", "func f() {", "\t/* block", "\tcomment */ return", "}"}
	out, ok := h.Lines("a.go", lines)
	if !ok {
		t.Fatalf("go lexer not found")
	}
	if len(out) != len(lines) {
		t.Fatalf("got %d lines want %d: %#v", len(out), len(lines), out)
	}
	if !strings.Contains(out[0], `<span class="tok-kn">package</span>`) {
		t.Fatalf("keyword not highlighted: %q", out[0])
	}
	if out[1] != "" {
		t.Fatalf("empty line should stay empty: %q", out[1])
	}
	if !strings.Contains(out[3], "tok-c") || !strings.Contains(out[4], "tok-c") {
		t.Fatalf("multi-line comment should keep state across lines: %q %q", out[3], out[4])
	}
	// 行内不能带换行（行数据要能直接放进 <td>）。
	for i, l := range out {
		if strings.Contains(l, "\n") {
			t.Fatalf("line %d contains newline: %q", i, l)
		}
	}
}

func TestLinesUnknownLexerFallsBack(t *testing.T) {
	h := New()
	lines := []string{"a < b && c"}
	out, ok := h.Lines("weird.zzzext", lines)
	if ok {
		t.Fatalf("expected fallback for unknown extension")
	}
	if len(out) != 1 || out[0] != "a &lt; b &amp;&amp; c" {
		t.Fatalf("fallback must be escaped plain text: %#v", out)
	}
}

func TestLinesEscapesHTML(t *testing.T) {
	h := New()
	out, _ := h.Lines("x.go", []string{`s := "<script>"`})
	if strings.Contains(out[0], "<script>") {
		t.Fatalf("html not escaped: %q", out[0])
	}
}
