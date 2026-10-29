package report

import (
	"strings"
	"testing"
)

func TestNewResultSplitsWarnings(t *testing.T) {
	res := NewResult([]Error{
		{Code: CodeContextMismatch, ChainID: "c", IntentID: "i"},
		{Code: CodeHighlightFallback, Severity: SeverityWarning},
	})
	if res.OK {
		t.Fatalf("error must make result not ok")
	}
	if len(res.Errors) != 1 || len(res.Warnings) != 1 {
		t.Fatalf("res = %+v", res)
	}
	if res.Warnings[0].Warning() != true {
		t.Fatalf("warning flag lost")
	}
}

func TestExitCode(t *testing.T) {
	cases := []struct {
		res  Result
		want int
	}{
		{NewResult(nil), 0},
		{NewResult([]Error{{Code: CodeContextMismatch}}), 1},
		{NewResult([]Error{{Code: CodeHighlightFallback, Severity: SeverityWarning}}), 0},
		{NewResult([]Error{{Code: CodeSchemaVersion}}), 3},
		{NewResult([]Error{{Code: CodeSchemaInvalid}, {Code: CodeSchemaVersion}}), 3},
	}
	for _, c := range cases {
		if got := ExitCode(c.res); got != c.want {
			t.Errorf("ExitCode(%+v) = %d want %d", c.res, got, c.want)
		}
	}
}

func TestFormatHuman(t *testing.T) {
	idx := 1
	e := Error{
		Code: CodeContextMismatch, ChainID: "chain-rate-limit", IntentID: "intent-002",
		ChangeIndex: &idx, File: "handler.go", Line: 12,
		Expected: "limiter.Wait()", Actual: "legacyLimiter.Wait()", Hint: "检查意图顺序",
	}
	out := FormatHuman(e)
	for _, want := range []string{
		"[chain-rate-limit] intent-002 change-2 handler.go:12 (CONTEXT_MISMATCH)",
		`期望: "limiter.Wait()"`, `实际: "legacyLimiter.Wait()"`, "提示: 检查意图顺序",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in %q", want, out)
		}
	}
}

func TestFormatHumanWarning(t *testing.T) {
	out := FormatHuman(Error{Code: CodeHighlightFallback, Severity: SeverityWarning, Message: "降级"})
	if !strings.HasPrefix(out, "WARNING (HIGHLIGHT_FALLBACK)\n") || !strings.Contains(out, "降级") {
		t.Fatalf("out = %q", out)
	}
}

func TestJSONShape(t *testing.T) {
	idx := 0
	res := NewResult([]Error{{
		Code: CodeContextMismatch, ChainID: "c", IntentID: "i", ChangeIndex: &idx,
		File: "a.go", Line: 3, Expected: "x", Actual: "y", Hint: "h",
	}})
	raw, err := res.JSON()
	if err != nil {
		t.Fatalf("json: %v", err)
	}
	out := string(raw)
	for _, want := range []string{`"ok": false`, `"errors": [`, `"code": "CONTEXT_MISMATCH"`, `"change_index": 0`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in %s", want, out)
		}
	}
	if strings.Contains(out, `"warnings"`) {
		t.Errorf("empty warnings should be omitted: %s", out)
	}
}

func TestIndexAndShortSHA(t *testing.T) {
	i := Index(7)
	if i == nil || *i != 7 {
		t.Fatalf("Index = %v", i)
	}
	if got := ShortSHA("  6325d83453c45f49715688c4c51c437ff6984411 "); got != "6325d834" {
		t.Fatalf("ShortSHA = %q", got)
	}
	if got := ShortSHA("abc"); got != "abc" {
		t.Fatalf("ShortSHA(short) = %q", got)
	}
}

func TestHasBlocking(t *testing.T) {
	if HasBlocking(nil) {
		t.Fatalf("nil must not block")
	}
	if HasBlocking([]Error{{Code: CodeHighlightFallback, Severity: SeverityWarning}}) {
		t.Fatalf("warnings must not block")
	}
	if !HasBlocking([]Error{{Code: CodeContextMismatch}}) {
		t.Fatalf("errors must block")
	}
}
