package diffpatch

import (
	"testing"

	"intent-review/internal/report"
	"intent-review/internal/schema"
)

func TestParseSingleHunk(t *testing.T) {
	patch := []string{
		"@@ -12,2 +12,3 @@",
		" context",
		"-old",
		"+new",
		"+extra",
	}
	h, e := Parse(patch)
	if e != nil {
		t.Fatalf("parse: %+v", e)
	}
	if h.OldStart != 12 || h.OldCount != 2 || h.NewStart != 12 || h.NewCount != 3 {
		t.Fatalf("header = %+v", h)
	}
	want := []schema.Line{
		{Type: schema.LineContext, Content: "context"},
		{Type: schema.LineDel, Content: "old"},
		{Type: schema.LineAdd, Content: "new"},
		{Type: schema.LineAdd, Content: "extra"},
	}
	if len(h.Lines) != len(want) {
		t.Fatalf("lines = %+v", h.Lines)
	}
	for i := range want {
		if h.Lines[i] != want[i] {
			t.Errorf("line %d = %+v want %+v", i, h.Lines[i], want[i])
		}
	}
}

func TestParseIgnoresFileHeaders(t *testing.T) {
	patch := []string{
		"diff --git a/a.go b/a.go",
		"index 111..222 100644",
		"--- a/a.go",
		"+++ b/a.go",
		"@@ -1 +1 @@",
		"-a",
		"+b",
	}
	h, e := Parse(patch)
	if e != nil {
		t.Fatalf("parse: %+v", e)
	}
	if h.OldCount != 1 || h.NewCount != 1 {
		t.Fatalf("header = %+v", h)
	}
}

func TestParseEmptyContextLine(t *testing.T) {
	patch := []string{"@@ -1,3 +1,3 @@", " a", "", "-b", "+c"}
	h, e := Parse(patch)
	if e != nil {
		t.Fatalf("parse: %+v", e)
	}
	if !h.Lines[1].IsContext() || h.Lines[1].Content != "" {
		t.Fatalf("empty line should be empty context: %+v", h.Lines[1])
	}
}

func TestParseRejectsMultipleHunks(t *testing.T) {
	patch := []string{"@@ -1,1 +1,1 @@", "-a", "+b", "@@ -9,1 +9,1 @@", "-c", "+d"}
	_, e := Parse(patch)
	if e == nil || e.Code != report.CodePatchParse {
		t.Fatalf("want PATCH_PARSE_ERROR, got %+v", e)
	}
}

func TestParseRejectsCountMismatch(t *testing.T) {
	patch := []string{"@@ -1,3 +1,1 @@", "-a", "+b"}
	_, e := Parse(patch)
	if e == nil || e.Code != report.CodePatchParse {
		t.Fatalf("want PATCH_PARSE_ERROR, got %+v", e)
	}
}

func TestParseRejectsBadPrefix(t *testing.T) {
	patch := []string{"@@ -1,1 +1,1 @@", "*a"}
	_, e := Parse(patch)
	if e == nil || e.Code != report.CodePatchParse {
		t.Fatalf("want PATCH_PARSE_ERROR, got %+v", e)
	}
}

func TestParsePureInsert(t *testing.T) {
	patch := []string{"@@ -3,0 +4,2 @@", "+x", "+y"}
	h, e := Parse(patch)
	if e != nil {
		t.Fatalf("parse: %+v", e)
	}
	if h.OldCount != 0 || h.NewCount != 2 || h.OldStart != 3 || h.NewStart != 4 {
		t.Fatalf("header = %+v", h)
	}
}

func TestParseNoNewlineMarker(t *testing.T) {
	patch := []string{"@@ -1,1 +1,1 @@", "-a", `\ No newline at end of file`, "+b",
		`\ No newline at end of file`}
	h, e := Parse(patch)
	if e != nil {
		t.Fatalf("parse: %+v", e)
	}
	if len(h.Lines) != 2 {
		t.Fatalf("lines = %+v", h.Lines)
	}
}

func TestParseKeepsCRLFContent(t *testing.T) {
	patch := []string{"@@ -1,1 +1,2 @@\r", " one\r", "+ONE\r"}
	h, e := Parse(patch)
	if e != nil {
		t.Fatalf("parse: %+v", e)
	}
	if h.Lines[0].Content != "one\r" || h.Lines[1].Content != "ONE\r" {
		t.Fatalf("CRLF content must be preserved: %+v", h.Lines)
	}
}
