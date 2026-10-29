package render

import (
	"intent-review/internal/report"
	"intent-review/internal/schema"
	"strings"
	"testing"
)

func TestBuildRenameAndBinaryTags(t *testing.T) {
	doc := testDoc()
	doc.Chains[0].Intents[0].Changes = []schema.Change{
		{
			File: "src/new_handler.go", RenamedFrom: "src/handler.go",
			Location: "移动文件", BusinessExplanation: "按职责拆分文件",
			OldStart: 1, NewStart: 1,
			Lines: []schema.Line{{Type: schema.LineContext, Content: "package main"}},
		},
		{
			File: "assets/logo.png", Location: "图标", BusinessExplanation: "更新图标",
			Binary: true,
		},
	}
	html, _ := build(t, doc, Options{})
	for _, want := range []string{"重命名自 src/handler.go", "tag-ren", "二进制", "binary-note"} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %q", want)
		}
	}
}

func TestBuildHighlightFallbackWarning(t *testing.T) {
	doc := testDoc()
	ch := &doc.Chains[0].Intents[0].Changes[0]
	ch.File = "src/handler.unknownext"
	ch.Lines = []schema.Line{{Type: schema.LineContext, Content: "the quick brown fox jumps over the lazy dog"}}
	html, res := build(t, doc, Options{})
	found := false
	for _, w := range res.Warnings {
		if w.Code == report.CodeHighlightFallback {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected HIGHLIGHT_FALLBACK warning, got %+v", res.Warnings)
	}
	if !strings.Contains(html, "HIGHLIGHT_FALLBACK") {
		t.Fatalf("warning not rendered in page")
	}
}

func TestBuildDisableHighlight(t *testing.T) {
	html, res := build(t, testDoc(), Options{DisableHighlight: true})
	if strings.Contains(html, `<span class="tok-`) {
		t.Fatalf("highlighting should be disabled")
	}
	for _, w := range res.Warnings {
		if w.Code == report.CodeHighlightFallback {
			t.Fatalf("no-highlight must not emit fallback warning")
		}
	}
}

func TestBuildLargeModeWarning(t *testing.T) {
	_, res := build(t, testDoc(), Options{MaxLines: 3})
	found := false
	for _, w := range res.Warnings {
		if w.Code == report.CodeLargeReview && w.Warning() {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing LARGE_REVIEW warning: %+v", res.Warnings)
	}
}
