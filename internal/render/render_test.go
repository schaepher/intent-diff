package render

import (
	"strings"
	"testing"

	"intent-review/internal/report"
	"intent-review/internal/schema"
)

func testDoc() schema.Document {
	return schema.Document{
		SchemaVersion: schema.SchemaVersion,
		Meta:          schema.Meta{Title: "限流与日志 <改造>", BaseCommit: "a1b2c3d4", HeadRef: "feat/x"},
		Chains: []schema.Chain{
			{
				ID: "chain-rate-limit", Title: "请求限流", Intents: []schema.Intent{
					{
						ID: "intent-001", Title: "引入请求级限流", Summary: "入口加令牌桶。",
						BusinessRationale: "高峰期下游被击穿。", RiskNotes: "阈值待确认。",
						Changes: []schema.Change{{
							File: "src/handler.go", Location: "Handler 入口",
							BusinessExplanation: "日志必须包住限流判断。",
							OldStart:            12, NewStart: 12,
							Lines: []schema.Line{
								{Type: schema.LineContext, Content: "func Handler(w http.ResponseWriter) {"},
								{Type: schema.LineDel, Content: "    legacyLimiter.Wait()"},
								{Type: schema.LineAdd, Content: "    limiter.Wait()"},
								{Type: schema.LineAdd, Content: "    log.Info(\"start\")"},
								{Type: schema.LineContext, Content: "    resp := callDownstream()"},
								{Type: schema.LineContext, Content: "}"},
							},
						}},
					},
					{
						ID: "intent-002", Title: "补充请求日志", Summary: "记录结束。",
						Changes: []schema.Change{{
							File: "src/handler.go", Location: "Handler 尾部",
							BusinessExplanation: "记录完成耗时。",
							OldStart:            15, NewStart: 15,
							Lines: []schema.Line{
								{Type: schema.LineContext, Content: "    resp := callDownstream()"},
								{Type: schema.LineAdd, Content: "    log.Info(\"done\")"},
								{Type: schema.LineContext, Content: "}"},
							},
						}},
					},
				},
			},
			{
				ID: "chain-audit", Title: "审计日志", Intents: []schema.Intent{{
					ID: "i1", Title: "写审计", Summary: "落库。",
					Changes: []schema.Change{{
						File: "src/audit.go", Location: "Audit", BusinessExplanation: "满足合规。",
						OldStart: 1, NewStart: 1,
						Lines: []schema.Line{{Type: schema.LineAdd, Content: "package audit"}},
					}},
				}},
			},
		},
	}
}

func build(t *testing.T, doc schema.Document, opt Options) (string, Result) {
	t.Helper()
	res, err := Build(doc, opt)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	return string(res.HTML), res
}

func TestBuildProducesSelfContainedHTML(t *testing.T) {
	html, res := build(t, testDoc(), Options{})
	if res.LargeMode {
		t.Fatalf("small review must not be large mode")
	}
	for _, want := range []string{
		"<!DOCTYPE html>", "<style>", "<script>", "intent-timeline", "timeline-node",
		"data-panel", "data-chain-section", "chain-rate-limit", "chain-audit",
		"业务解释", "日志必须包住限流判断。", "请求限流", "请求日志",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %q", want)
		}
	}
	// 离线：不得有外部资源引用 / fetch / 动态 import。
	for _, bad := range []string{"http://", "https://", "src=\"//", "fetch(", "import("} {
		if strings.Contains(html, bad) {
			t.Errorf("self-contained violation: %q", bad)
		}
	}
	// 未激活的链必须 hidden；初始只有一个 panel 可见。
	if !strings.Contains(html, `data-chain-section="chain-audit" hidden`) {
		t.Errorf("inactive chain must be hidden")
	}
	if n := strings.Count(html, `data-panel data-chain=`); n != 3 {
		t.Fatalf("expected 3 panels, got %d", n)
	}
	if n := strings.Count(html, `<article class="intent-panel"`); n != 3 {
		t.Fatalf("expected 3 article panels, got %d", n)
	}
	visible := 0
	for i := 0; i < len(html); {
		idx := strings.Index(html[i:], `<article class="intent-panel"`)
		if idx < 0 {
			break
		}
		start := i + idx
		end := strings.Index(html[start:], ">")
		tag := html[start : start+end]
		if !strings.Contains(tag, " hidden") {
			visible++
		}
		i = start + 1
	}
	if visible != 1 {
		t.Fatalf("only the first panel should be visible at render time, got %d", visible)
	}
	// sticky 状态机 + 两列 + 行号。
	for _, want := range []string{"position: sticky", "State(0)", "State(1)", `<td class="ln">12</td>`, "code-del", "code-add", "code-blank"} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %q", want)
		}
	}
}

func TestBuildEscapesUserContent(t *testing.T) {
	doc := testDoc()
	doc.Chains[0].Title = `<script>alert("x")</script>`
	doc.Chains[0].Intents[0].Changes[0].BusinessExplanation = `<img src=x onerror="alert(1)">`
	doc.Chains[0].Intents[0].Changes[0].Lines[0].Content = `<b>not bold</b>`
	html, _ := build(t, doc, Options{})
	if strings.Contains(html, `<script>alert("x")</script>`) {
		t.Fatalf("chain title not escaped")
	}
	if strings.Contains(html, `<img src=x onerror=`) {
		t.Fatalf("business explanation not escaped")
	}
	if strings.Contains(html, `<b>not bold</b>`) {
		t.Fatalf("code content not escaped")
	}
}

func TestBuildDeepLinkAnchors(t *testing.T) {
	html, _ := build(t, testDoc(), Options{})
	for _, want := range []string{
		`id="change-c0-i0-ch1"`, `data-change-index="1"`, `id="change-c0-i1-ch1"`,
		`title="change 序号（深链 #chain/intent/1）"`,
		`data-intent="intent-001"`, `data-intent="intent-002"`, `id="panel-0-0"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %q", want)
		}
	}
}

func TestBuildLargeMode(t *testing.T) {
	html, res := build(t, testDoc(), Options{MaxLines: 3})
	if !res.LargeMode {
		t.Fatalf("expected large mode")
	}
	if !strings.Contains(html, "大型评审") || !strings.Contains(html, "<details class=\"panel-details\">") {
		t.Fatalf("large mode decoration missing")
	}
}

func TestBuildWarningsFloatToIntent(t *testing.T) {
	warn := report.Error{
		Code: report.CodeContextMismatch, ChainID: "chain-rate-limit", IntentID: "intent-002",
		File: "src/handler.go", Line: 15, Message: "上下文不一致",
	}
	html, res := build(t, testDoc(), Options{Warnings: []report.Error{warn}})
	if len(res.Warnings) != 1 {
		t.Fatalf("warnings = %+v", res.Warnings)
	}
	idx := strings.Index(html, `data-intent="intent-002"`)
	if idx < 0 {
		t.Fatalf("intent node missing")
	}
	node := html[strings.LastIndex(html[:idx], "<li") : strings.Index(html[idx:], "</li>")+idx]
	if !strings.Contains(node, "has-warning") || !strings.Contains(node, "⚠") {
		t.Fatalf("warning not floated to node: %q", node)
	}
	if !strings.Contains(html, "CONTEXT_MISMATCH") {
		t.Fatalf("warning list missing")
	}
}

func TestBuildPatchOnlyMarker(t *testing.T) {
	doc := testDoc()
	ch := &doc.Chains[0].Intents[0].Changes[0]
	ch.Lines = nil
	ch.Patch = []string{"@@ -1 +1 @@", "-a", "+b"}
	html, _ := build(t, doc, Options{})
	if !strings.Contains(html, "code-blank") {
		t.Fatalf("patch-only change should render empty diff")
	}
}
