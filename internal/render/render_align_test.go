package render

import (
	"intent-review/internal/highlight"
	"intent-review/internal/schema"
	"testing"
)

func TestAlignRows(t *testing.T) {
	hl := newTestHighlighter()
	ch := schema.Change{
		File: "a.go", OldStart: 5, NewStart: 5,
		Lines: []schema.Line{
			{Type: schema.LineContext, Content: "c1"},
			{Type: schema.LineDel, Content: "d1"},
			{Type: schema.LineDel, Content: "d2"},
			{Type: schema.LineAdd, Content: "a1"},
			{Type: schema.LineContext, Content: "c2"},
		},
	}
	rows, ok := alignRows(ch, hl)
	if !ok {
		t.Fatalf("highlight failed for go file")
	}
	if len(rows) != 4 {
		t.Fatalf("rows = %d: %+v", len(rows), rows)
	}
	// context 行两侧同一行号
	if rows[0].Left.No != 5 || rows[0].Right.No != 5 || !rows[0].Context {
		t.Fatalf("row0 = %+v", rows[0])
	}
	// del/add 按索引配对：d1↔a1，d2 右列空白
	if rows[1].Left.Kind != "del" || rows[1].Right.Kind != "add" {
		t.Fatalf("row1 = %+v", rows[1])
	}
	if rows[1].Left.No != 6 || rows[1].Right.No != 6 {
		t.Fatalf("row1 numbers = %+v", rows[1])
	}
	if rows[2].Left.Kind != "del" || rows[2].Right.Kind != "" {
		t.Fatalf("row2 = %+v", rows[2])
	}
	if rows[2].Left.No != 7 || rows[2].Right.No != 0 {
		t.Fatalf("row2 numbers = %+v", rows[2])
	}
	if rows[3].Left.No != 8 || rows[3].Right.No != 7 || !rows[3].Context {
		t.Fatalf("row3 = %+v", rows[3])
	}
}

func TestAlignRowsPureInsert(t *testing.T) {
	rows, _ := alignRows(schema.Change{
		File: "a.go", OldStart: 2, NewStart: 3,
		Lines: []schema.Line{{Type: schema.LineAdd, Content: "x"}},
	}, newTestHighlighter())
	if len(rows) != 1 {
		t.Fatalf("rows = %+v", rows)
	}
	if rows[0].Left.Kind != "" || rows[0].Right.Kind != "add" || rows[0].Right.No != 3 {
		t.Fatalf("row = %+v", rows[0])
	}
}

func TestAlignRowsNewFile(t *testing.T) {
	rows, _ := alignRows(schema.Change{
		File: "a.go", NewFile: true, OldStart: 0, NewStart: 1,
		Lines: []schema.Line{{Type: schema.LineAdd, Content: "package a"}},
	}, newTestHighlighter())
	if rows[0].Left.No != 0 || rows[0].Right.No != 1 {
		t.Fatalf("row = %+v", rows[0])
	}
}

func newTestHighlighter() *highlight.Highlighter { return highlight.New() }
