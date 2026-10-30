package schema

import (
	"testing"

	"intent-review/internal/report"
)

func TestCleanRelPath(t *testing.T) {
	ok := []struct{ in, want string }{
		{"a.go", "a.go"},
		{"pkg/a.go", "pkg/a.go"},
		{"./pkg/a.go", "pkg/a.go"},
		{"pkg/../a.go", "a.go"},
		{"pkg//a.go", "pkg/a.go"},
		{`pkg\a.go`, "pkg/a.go"},
	}
	for _, c := range ok {
		got, valid := CleanRelPath(c.in)
		if !valid || got != c.want {
			t.Errorf("CleanRelPath(%q) = %q,%v want %q", c.in, got, valid, c.want)
		}
	}
	bad := []string{"", "   ", "/etc/passwd", "../x.go", "..", ".", "a/../../b.go", `..\x.go`, "C:/win.go", "c:x.go"}
	for _, in := range bad {
		if got, valid := CleanRelPath(in); valid {
			t.Errorf("CleanRelPath(%q) = %q, want invalid", in, got)
		}
	}
}

// 结构校验阶段就拒绝绝对路径与越界路径（render 无 repo 时也不会生成坏链接）。
func TestCheckStructureRejectsUnsafeFilePaths(t *testing.T) {
	for _, file := range []string{"/abs.go", "../up.go", "a/../../b.go", "C:/win.go"} {
		ch := validChange()
		ch.File = file
		if errs := CheckStructure(docWith(ch)); !hasCode(errs, report.CodeSchemaInvalid) {
			t.Errorf("file %q 应被拒绝: %v", file, codes(errs))
		}
	}
	ch := validChange()
	ch.File = "pkg/a.go"
	if errs := CheckStructure(docWith(ch)); len(errs) != 0 {
		t.Fatalf("合法路径被拒: %v", codes(errs))
	}
	ch = validChange()
	ch.RenamedFrom = "../old.go"
	if errs := CheckStructure(docWith(ch)); !hasCode(errs, report.CodeSchemaInvalid) {
		t.Fatalf("越界 renamed_from 应被拒绝: %v", codes(errs))
	}
}
