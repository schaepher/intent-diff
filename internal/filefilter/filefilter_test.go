package filefilter

import (
	"reflect"
	"testing"
)

func TestDefaultExcludesTestFilesAtAnyDepth(t *testing.T) {
	l := Default()
	for _, p := range []string{
		"a_test.go",
		"internal/render/render_test.go",
		"cmd/intent-review/main_test.go",
	} {
		if !l.Match(p) {
			t.Fatalf("默认排除列表应命中 %q", p)
		}
	}
	for _, p := range []string{"main.go", "testing.go", "a_test.go.bak", "testdata/main.go", "a_test_go"} {
		if l.Match(p) {
			t.Fatalf("默认排除列表不应命中 %q", p)
		}
	}
}

func TestDefaultExcludesVendorAndNodeModules(t *testing.T) {
	l := Default()
	for _, p := range []string{
		"vendor/github.com/x/y.go",
		"svc/vendor/x.go",
		"node_modules/x.js",
		"web/node_modules/react/index.js",
	} {
		if !l.Match(p) {
			t.Fatalf("默认排除列表应命中 %q", p)
		}
	}
	for _, p := range []string{"vendorx/a.go", "myvendor/a.go", "internal/node_modules.go"} {
		if l.Match(p) {
			t.Fatalf("默认排除列表不应命中 %q", p)
		}
	}
}

func TestPatternSemantics(t *testing.T) {
	cases := []struct {
		pattern string
		path    string
		want    bool
	}{
		// 不含 "/" 的模式匹配任意深度的文件名。
		{"*.go", "a/b/c.go", true},
		{"*.go", "a/b/c.js", false},
		{"a?c.go", "abc.go", true},
		{"a?c.go", "abbc.go", false},
		{"[ab].go", "a.go", true},
		{"[ab].go", "c.go", false},
		{"[!ab].go", "c.go", true},
		{"[!ab].go", "a.go", false},
		// 含 "/" 的模式锚定仓库根。
		{"docs/*.md", "docs/a.md", true},
		{"docs/*.md", "sub/docs/a.md", false},
		{"docs/**", "docs/a/b/c.md", true},
		{"internal/*", "internal/a.go", true},
		{"internal/*", "internal/sub/a.go", false},
		// ** 可以出现在中间，且允许 0 级目录。
		{"a/**/b.go", "a/b.go", true},
		{"a/**/b.go", "a/x/y/b.go", true},
		{"**/gen/**", "gen/a.go", true},
		{"**/gen/**", "pkg/gen/deep/a.go", true},
		{"**/gen/**", "pkg/genx/a.go", false},
		// 结尾 "/" 等价于 "<dir>/**"。
		{"tmp/", "tmp/a/b.txt", true},
		{"tmp/", "tmp", false},
		// 未闭合的字符集按字面量处理，不 panic。
		{"[abc", "[abc", true},
	}
	for _, c := range cases {
		got := New([]string{c.pattern}).Match(c.path)
		if got != c.want {
			t.Errorf("New(%q).Match(%q) = %v, want %v", c.pattern, c.path, got, c.want)
		}
	}
}

func TestWindowsSeparatorsAreNormalized(t *testing.T) {
	l := New([]string{"internal/*.go"})
	if !l.Match(`internal\render.go`) {
		t.Fatalf("反斜杠路径应归一化后命中")
	}
	if !New([]string{`internal\*.go`}).Match("internal/render.go") {
		t.Fatalf("反斜杠模式应归一化后命中")
	}
}

func TestAddAndRemove(t *testing.T) {
	l := Default()
	l.Add("docs/**")
	if !l.Match("docs/a.md") {
		t.Fatalf("Add 之后应命中 docs/a.md")
	}
	l.Remove("docs/**")
	if l.Match("docs/a.md") {
		t.Fatalf("Remove 之后不应再命中 docs/a.md")
	}
	// 删除内置项：_test.go 重新进入评审范围。
	l.Remove("*_test.go")
	if l.Match("internal/foo_test.go") {
		t.Fatalf("删除内置项后不应命中测试文件")
	}
	if !l.Match("vendor/x/y.go") {
		t.Fatalf("删除单项不应影响其它内置项")
	}
}

func TestPatternsOrderAndDedup(t *testing.T) {
	l := New([]string{"b", "a", "b", " ", ""})
	want := []string{"b", "a"}
	if got := l.Patterns(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Patterns() = %v, want %v", got, want)
	}
}

func TestEmptyAndNilListMatchNothing(t *testing.T) {
	if New(nil).Match("a_test.go") {
		t.Fatalf("空列表不应命中任何路径")
	}
	var l *List
	if l.Match("a_test.go") {
		t.Fatalf("nil 列表不应命中任何路径")
	}
	if got := l.Patterns(); len(got) != 0 {
		t.Fatalf("nil 列表 Patterns() = %v", got)
	}
	l.Add("x")
	l.Remove("x")
}

func TestMatchReasonReturnsHitPattern(t *testing.T) {
	l := Default()
	if rule, ok := l.MatchReason("pkg/a_test.go"); !ok || rule != "*_test.go" {
		t.Fatalf("MatchReason = %q,%v", rule, ok)
	}
	if rule, ok := l.MatchReason("pkg/a.go"); ok || rule != "" {
		t.Fatalf("未命中时不应返回规则: %q,%v", rule, ok)
	}
	custom := New([]string{"docs/**"})
	if rule, ok := custom.MatchReason("docs/a.md"); !ok || rule != "docs/**" {
		t.Fatalf("MatchReason = %q,%v", rule, ok)
	}
}
