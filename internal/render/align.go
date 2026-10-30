package render

import (
	"html"
	"html/template"

	"intent-review/internal/highlight"
	"intent-review/internal/schema"
)

// Cell 是两列 diff 中的一格。Kind 为空表示空白（该侧没有这一行）。
type Cell struct {
	No   int
	Kind string
	Sign string
	HTML template.HTML
	// Link 是该行在本地源码中的跳转链接（仅新状态一侧提供）。
	Link template.URL
}

// Row 是一行对齐结果（设计文档 §8.5）。
type Row struct {
	Context bool
	Left    Cell
	Right   Cell
}

// alignRows 把结构化 lines 排成两列：连续 del 块与紧随的 add 块按索引配对，
// 数量不等时短侧补空白；context 两侧同步递增。
func alignRows(ch schema.Change, hl *highlight.Highlighter) ([]Row, bool) {
	leftLines, rightLines := ch.LeftLines(), ch.RightLines()
	var leftHTML, rightHTML []string
	ok := true
	if hl == nil {
		leftHTML, rightHTML = plainLines(leftLines), plainLines(rightLines)
	} else {
		var okLeft, okRight bool
		leftHTML, okLeft = hl.Lines(ch.File, leftLines)
		rightHTML, okRight = hl.Lines(ch.File, rightLines)
		ok = okLeft && okRight
	}

	rows := make([]Row, 0, len(ch.Lines))
	leftNo, rightNo := ch.OldStart, ch.NewStart
	li, ri := 0, 0
	for i := 0; i < len(ch.Lines); {
		if ch.Lines[i].IsContext() {
			rows = append(rows, Row{
				Context: true,
				Left:    Cell{No: leftNo, Kind: schema.LineContext, HTML: htmlOf(leftHTML, li)},
				Right:   Cell{No: rightNo, Kind: schema.LineContext, HTML: htmlOf(rightHTML, ri)},
			})
			leftNo++
			rightNo++
			li++
			ri++
			i++
			continue
		}
		start := i
		for i < len(ch.Lines) && !ch.Lines[i].IsContext() {
			i++
		}
		var dels, adds []schema.Line
		for _, l := range ch.Lines[start:i] {
			if l.IsDel() {
				dels = append(dels, l)
			} else if l.IsAdd() {
				adds = append(adds, l)
			}
		}
		n := len(dels)
		if len(adds) > n {
			n = len(adds)
		}
		for k := 0; k < n; k++ {
			var row Row
			if k < len(dels) {
				row.Left = Cell{No: leftNo, Kind: schema.LineDel, Sign: "-", HTML: htmlOf(leftHTML, li)}
				leftNo++
				li++
			}
			if k < len(adds) {
				row.Right = Cell{No: rightNo, Kind: schema.LineAdd, Sign: "+", HTML: htmlOf(rightHTML, ri)}
				rightNo++
				ri++
			}
			rows = append(rows, row)
		}
	}
	return rows, ok
}

func plainLines(lines []string) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = html.EscapeString(l)
	}
	return out
}

func htmlOf(lines []string, idx int) template.HTML {
	if idx < 0 || idx >= len(lines) {
		return ""
	}
	return template.HTML(lines[idx]) // 已由 highlight 转义
}
