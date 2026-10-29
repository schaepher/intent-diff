package replay

import (
	"testing"

	"intent-review/internal/report"
	"intent-review/internal/schema"
	"intent-review/internal/testutil"
)

func ctx() Ctx { return Ctx{ChainID: "c", IntentID: "i"} }

func newEngine(t *testing.T, files map[string]string) (*Engine, *testutil.Repo) {
	t.Helper()
	repo := testutil.NewGitRepo(t, files)
	r, err := Open(repo.Dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	base, err := r.Resolve("HEAD")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	return NewEngine(r, base), repo
}

func TestApplyReplaceAndInsert(t *testing.T) {
	eng, _ := newEngine(t, map[string]string{"a.txt": "one\ntwo\nthree\n"})
	ch := &schema.Change{
		File: "a.txt", OldStart: 2, NewStart: 2,
		Lines: []schema.Line{
			{Type: schema.LineDel, Content: "two"},
			{Type: schema.LineAdd, Content: "TWO"},
			{Type: schema.LineAdd, Content: "two-and-half"},
		},
	}
	if err := eng.Apply(ctx(), ch); err != nil {
		t.Fatalf("apply: %+v", err)
	}
	got := eng.State()["a.txt"].Lines
	want := []string{"one", "TWO", "two-and-half", "three"}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("line %d = %q want %q", i, got[i], want[i])
		}
	}
}

func TestApplyPureInsert(t *testing.T) {
	eng, _ := newEngine(t, map[string]string{"a.txt": "one\ntwo\n"})
	ch := &schema.Change{
		File: "a.txt", OldStart: 1, NewStart: 2,
		Lines: []schema.Line{{Type: schema.LineAdd, Content: "inserted"}},
	}
	if err := eng.Apply(ctx(), ch); err != nil {
		t.Fatalf("apply: %+v", err)
	}
	got := eng.State()["a.txt"].Lines
	if len(got) != 3 || got[1] != "inserted" {
		t.Fatalf("got %v", got)
	}
}

func TestApplyNewFileAndDelete(t *testing.T) {
	eng, _ := newEngine(t, map[string]string{"old.txt": "x\ny\n"})
	ch := &schema.Change{
		File: "new.txt", NewFile: true, OldStart: 0, NewStart: 1,
		Lines: []schema.Line{{Type: schema.LineAdd, Content: "hello"}},
	}
	if err := eng.Apply(ctx(), ch); err != nil {
		t.Fatalf("new_file: %+v", err)
	}
	del := &schema.Change{
		File: "old.txt", Deleted: true, OldStart: 1, NewStart: 0,
		Lines: []schema.Line{
			{Type: schema.LineDel, Content: "x"},
			{Type: schema.LineDel, Content: "y"},
		},
	}
	if err := eng.Apply(ctx(), del); err != nil {
		t.Fatalf("deleted: %+v", err)
	}
	st := eng.State()
	if !st["new.txt"].Exists || st["old.txt"].Exists {
		t.Fatalf("state = %+v", st)
	}
}

func TestApplyRename(t *testing.T) {
	eng, _ := newEngine(t, map[string]string{"a.txt": "keep\n"})
	ren := &schema.Change{
		File: "b.txt", RenamedFrom: "a.txt", OldStart: 1, NewStart: 1,
		Lines: []schema.Line{
			{Type: schema.LineContext, Content: "keep"},
			{Type: schema.LineAdd, Content: "more"},
		},
	}
	if err := eng.Apply(ctx(), ren); err != nil {
		t.Fatalf("rename: %+v", err)
	}
	st := eng.State()
	if st["a.txt"].Exists || !st["b.txt"].Exists || len(st["b.txt"].Lines) != 2 {
		t.Fatalf("state = %+v", st)
	}
}

func TestApplyContextMismatch(t *testing.T) {
	eng, _ := newEngine(t, map[string]string{"a.txt": "one\ntwo\n"})
	ch := &schema.Change{
		File: "a.txt", OldStart: 1, NewStart: 1,
		Lines: []schema.Line{
			{Type: schema.LineContext, Content: "ONE"},
			{Type: schema.LineAdd, Content: "x"},
		},
	}
	e := eng.Apply(ctx(), ch)
	if e == nil || e.Code != report.CodeContextMismatch {
		t.Fatalf("want CONTEXT_MISMATCH, got %+v", e)
	}
	if e.Line != 1 || e.Expected != "ONE" || e.Actual != "one" {
		t.Fatalf("diagnostic = %+v", e)
	}
	if len(eng.State()["a.txt"].Lines) != 2 {
		t.Fatalf("state must not change on failure")
	}
}

func TestApplyOutOfRange(t *testing.T) {
	eng, _ := newEngine(t, map[string]string{"a.txt": "one\n"})
	ch := &schema.Change{
		File: "a.txt", OldStart: 5, NewStart: 5,
		Lines: []schema.Line{{Type: schema.LineAdd, Content: "x"}},
	}
	if e := eng.Apply(ctx(), ch); e == nil || e.Code != report.CodeContextMismatch {
		t.Fatalf("want CONTEXT_MISMATCH, got %+v", e)
	}
}

func TestApplyNewStartMismatch(t *testing.T) {
	eng, _ := newEngine(t, map[string]string{"a.txt": "one\n"})
	ch := &schema.Change{
		File: "a.txt", OldStart: 1, NewStart: 9,
		Lines: []schema.Line{{Type: schema.LineAdd, Content: "x"}},
	}
	if e := eng.Apply(ctx(), ch); e == nil || e.Code != report.CodeContextMismatch {
		t.Fatalf("want CONTEXT_MISMATCH, got %+v", e)
	}
}

func TestApplyMissingFile(t *testing.T) {
	eng, _ := newEngine(t, map[string]string{"a.txt": "one\n"})
	ch := &schema.Change{
		File: "nope.txt", OldStart: 1, NewStart: 1,
		Lines: []schema.Line{{Type: schema.LineAdd, Content: "x"}},
	}
	if e := eng.Apply(ctx(), ch); e == nil || e.Code != report.CodeFileNotFound {
		t.Fatalf("want FILE_NOT_FOUND, got %+v", e)
	}
}

func TestSplitLines(t *testing.T) {
	cases := map[string][]string{
		"":       nil,
		"a":      {"a"},
		"a\n":    {"a"},
		"a\nb\n": {"a", "b"},
		"a\n\n":  {"a", ""},
		"\n":     {""},
	}
	for in, want := range cases {
		got := SplitLines(in)
		if len(got) != len(want) {
			t.Errorf("SplitLines(%q) = %#v want %#v", in, got, want)
			continue
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("SplitLines(%q)[%d] = %q want %q", in, i, got[i], want[i])
			}
		}
		if want == nil {
			if JoinLines(want) != "" {
				t.Errorf("JoinLines(nil) = %q", JoinLines(want))
			}
		}
	}
	if JoinLines([]string{"a", "b"}) != "a\nb\n" {
		t.Errorf("JoinLines = %q", JoinLines([]string{"a", "b"}))
	}
}
