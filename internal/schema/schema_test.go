package schema

import (
	"encoding/json"
	"testing"

	"intent-review/internal/report"
)

func docWith(change Change) Document {
	return Document{
		SchemaVersion: SchemaVersion,
		Meta:          Meta{Title: "t", BaseCommit: "abc", HeadRef: "main"},
		Chains: []Chain{{
			ID: "c1", Title: "chain", Intents: []Intent{{
				ID: "i1", Title: "intent", Summary: "sum", Changes: []Change{change},
			}},
		}},
	}
}

func validChange() Change {
	return Change{
		File: "a.go", Location: "入口", BusinessExplanation: "为什么",
		OldStart: 1, NewStart: 1,
		Lines: []Line{
			{Type: LineContext, Content: "a"},
			{Type: LineAdd, Content: "b"},
		},
	}
}

func codes(errs []report.Error) []string {
	out := make([]string, 0, len(errs))
	for _, e := range errs {
		out = append(out, e.Code)
	}
	return out
}

func hasCode(errs []report.Error, code string) bool {
	for _, e := range errs {
		if e.Code == code {
			return true
		}
	}
	return false
}

func TestCheckVersion(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"1.0", ""},
		{"1.7", ""},
		{"2.0", report.CodeSchemaVersion},
		{"", report.CodeSchemaInvalid},
		{"abc", report.CodeSchemaInvalid},
	}
	for _, c := range cases {
		e := CheckVersion(c.in)
		got := ""
		if e != nil {
			got = e.Code
		}
		if got != c.want {
			t.Errorf("CheckVersion(%q)=%q want %q", c.in, got, c.want)
		}
	}
}

func TestCheckStructureValid(t *testing.T) {
	errs := CheckStructure(docWith(validChange()))
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", codes(errs))
	}
}

func TestCheckStructureDuplicateIDs(t *testing.T) {
	doc := docWith(validChange())
	doc.Chains = append(doc.Chains, doc.Chains[0])
	errs := CheckStructure(doc)
	if !hasCode(errs, report.CodeDuplicateChainID) {
		t.Fatalf("want DUPLICATE_CHAIN_ID, got %v", codes(errs))
	}

	doc = docWith(validChange())
	doc.Chains[0].Intents = append(doc.Chains[0].Intents, doc.Chains[0].Intents[0])
	errs = CheckStructure(doc)
	if !hasCode(errs, report.CodeDuplicateIntentID) {
		t.Fatalf("want DUPLICATE_INTENT_ID, got %v", codes(errs))
	}
}

func TestCheckStructureMissingExplanation(t *testing.T) {
	ch := validChange()
	ch.BusinessExplanation = "  "
	errs := CheckStructure(docWith(ch))
	if !hasCode(errs, report.CodeMissingExplanation) {
		t.Fatalf("want MISSING_BUSINESS_EXPLANATION, got %v", codes(errs))
	}
}

func TestCheckStructureLineNumbers(t *testing.T) {
	ch := validChange()
	ch.OldStart = 0
	errs := CheckStructure(docWith(ch))
	if !hasCode(errs, report.CodeSchemaInvalid) {
		t.Fatalf("want SCHEMA_INVALID for missing old_start, got %v", codes(errs))
	}

	ch = validChange()
	ch.NewStart = 0
	errs = CheckStructure(docWith(ch))
	if !hasCode(errs, report.CodeSchemaInvalid) {
		t.Fatalf("want SCHEMA_INVALID for missing new_start, got %v", codes(errs))
	}
}

func TestCheckStructureNewFileRules(t *testing.T) {
	ch := Change{
		File: "new.go", Location: "x", BusinessExplanation: "y", NewFile: true,
		OldStart: 0, NewStart: 1,
		Lines: []Line{{Type: LineAdd, Content: "a"}},
	}
	if errs := CheckStructure(docWith(ch)); len(errs) != 0 {
		t.Fatalf("valid new_file rejected: %v", codes(errs))
	}
	ch.OldStart = 1
	if errs := CheckStructure(docWith(ch)); !hasCode(errs, report.CodeContextMismatch) {
		t.Fatalf("want CONTEXT_MISMATCH for new_file old_start, got %v", codes(errs))
	}
	ch.OldStart = 0
	ch.Lines = []Line{{Type: LineContext, Content: "a"}}
	if errs := CheckStructure(docWith(ch)); !hasCode(errs, report.CodeContextMismatch) {
		t.Fatalf("want CONTEXT_MISMATCH for new_file context line, got %v", codes(errs))
	}
}

func TestCheckStructureDeletedRules(t *testing.T) {
	ch := Change{
		File: "old.go", Location: "x", BusinessExplanation: "y", Deleted: true,
		OldStart: 1, NewStart: 0,
		Lines: []Line{{Type: LineDel, Content: "a"}},
	}
	if errs := CheckStructure(docWith(ch)); len(errs) != 0 {
		t.Fatalf("valid deleted rejected: %v", codes(errs))
	}
	ch.NewStart = 1
	if errs := CheckStructure(docWith(ch)); !hasCode(errs, report.CodeContextMismatch) {
		t.Fatalf("want CONTEXT_MISMATCH for deleted new_start, got %v", codes(errs))
	}
}

func TestCheckStructureInvalidLineType(t *testing.T) {
	ch := validChange()
	ch.Lines[0].Type = "keep"
	errs := CheckStructure(docWith(ch))
	if !hasCode(errs, report.CodeSchemaInvalid) {
		t.Fatalf("want SCHEMA_INVALID for bad line type, got %v", codes(errs))
	}
}

func TestSchemaJSONValid(t *testing.T) {
	raw, err := SchemaJSON()
	if err != nil {
		t.Fatalf("SchemaJSON: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("schema is not valid JSON: %v", err)
	}
	props := parsed["properties"].(map[string]any)
	for _, want := range []string{"schema_version", "meta", "chains"} {
		if _, ok := props[want]; !ok {
			t.Errorf("schema missing top-level property %q", want)
		}
	}
	changeProps := parsed["definitions"].(map[string]any)["change"].(map[string]any)["properties"].(map[string]any)
	for _, want := range []string{"file", "location", "business_explanation", "lines", "patch", "new_file", "deleted", "renamed_from"} {
		if _, ok := changeProps[want]; !ok {
			t.Errorf("schema missing change property %q", want)
		}
	}
}

func TestChangeNeedsFill(t *testing.T) {
	cases := []struct {
		name string
		ch   Change
		want bool
	}{
		{"patch-only", Change{Patch: []string{"@@ -1 +1 @@"}}, true},
		{"with-lines", Change{Patch: []string{"x"}, Lines: []Line{{Type: LineAdd, Content: "a"}}}, false},
		{"no-patch", Change{}, false},
		{"binary", Change{Patch: []string{"x"}, Binary: true}, false},
	}
	for _, c := range cases {
		if got := c.ch.NeedsFill(); got != c.want {
			t.Errorf("%s: NeedsFill = %v want %v", c.name, got, c.want)
		}
	}
}
