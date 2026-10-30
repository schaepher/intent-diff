package fill

import (
	"encoding/json"
	"intent-review/internal/schema"
	"intent-review/internal/testutil"
	"intent-review/internal/validate"
	"testing"
)

func TestRunPreservesUnknownFields(t *testing.T) {
	repo, base, head := fixture(t)
	doc := skeleton(base, head)
	raw := mustMarshal(t, doc)
	var tree map[string]any
	if err := json.Unmarshal(raw, &tree); err != nil {
		t.Fatalf("decode: %v", err)
	}
	tree["x-note"] = "keep-me"
	chains := tree["chains"].([]any)
	chain := chains[0].(map[string]any)
	intents := chain["intents"].([]any)
	change := intents[0].(map[string]any)["changes"].([]any)[0].(map[string]any)
	change["x-custom"] = "change-level"
	raw, _ = json.Marshal(tree)

	out, res, err := Run(doc, raw, repo.Dir)
	if err != nil || !res.OK {
		t.Fatalf("run: %v %+v", err, res.Errors)
	}
	var back map[string]any
	if err := json.Unmarshal(out, &back); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if back["x-note"] != "keep-me" {
		t.Fatalf("top-level unknown field lost")
	}
	ch := back["chains"].([]any)[0].(map[string]any)["intents"].([]any)[0].(map[string]any)["changes"].([]any)[0].(map[string]any)
	if ch["x-custom"] != "change-level" {
		t.Fatalf("change-level unknown field lost")
	}
}

func TestRunNewFileAndDeleted(t *testing.T) {
	repo := testutil.NewGitRepo(t, map[string]string{
		"del.go": "package main\n\nvar A = 1\n",
	})
	base := repo.Head()
	repo.Remove("del.go")
	repo.Write("new.go", "package main\n\nvar B = 2\n")
	head := repo.Commit("replace")

	doc := schema.Document{
		SchemaVersion: schema.SchemaVersion,
		Meta:          schema.Meta{Title: "t", BaseCommit: base, HeadRef: head},
		Chains: []schema.Chain{{ID: "c", Title: "c", Intents: []schema.Intent{{
			ID: "i1", Title: "增删", Summary: "新增与删除",
			Changes: []schema.Change{
				{
					File: "new.go", Location: "新文件", BusinessExplanation: "新增常量", NewFile: true,
					Patch: []string{"@@ -0,0 +1,3 @@", "+package main", "+", "+var B = 2"},
				},
				{
					File: "del.go", Location: "旧文件", BusinessExplanation: "删除旧常量", Deleted: true,
					Patch: []string{"@@ -1,3 +0,0 @@", "-package main", "-", "-var A = 1"},
				},
			},
		}}}},
	}
	out, res, err := Run(doc, mustMarshal(t, doc), repo.Dir)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !res.OK {
		t.Fatalf("errors = %+v", res.Errors)
	}
	var filled schema.Document
	if err := json.Unmarshal(out, &filled); err != nil {
		t.Fatalf("decode: %v", err)
	}
	nf := filled.Chains[0].Intents[0].Changes[0]
	if nf.OldStart != 0 || nf.NewStart != 1 {
		t.Fatalf("new_file starts = %d/%d", nf.OldStart, nf.NewStart)
	}
	df := filled.Chains[0].Intents[0].Changes[1]
	if df.OldStart != 1 || df.NewStart != 0 {
		t.Fatalf("deleted starts = %d/%d", df.OldStart, df.NewStart)
	}
	vres, err := validate.Run(filled, repo.Dir, validate.Options{})
	if err != nil || !vres.OK {
		t.Fatalf("validate: %v %+v", err, vres.Errors)
	}
}

func TestRunCRLFFile(t *testing.T) {
	repo := testutil.NewGitRepo(t, map[string]string{"a.txt": "one\r\ntwo\r\n"})
	base := repo.Head()
	repo.Write("a.txt", "one\r\nONE\r\ntwo\r\n")
	head := repo.Commit("crlf")

	doc := schema.Document{
		SchemaVersion: schema.SchemaVersion,
		Meta:          schema.Meta{Title: "crlf", BaseCommit: base, HeadRef: head},
		Chains: []schema.Chain{{ID: "c", Title: "c", Intents: []schema.Intent{{
			ID: "i", Title: "插入", Summary: "CRLF 文件插入一行",
			Changes: []schema.Change{{
				File: "a.txt", Location: "第 1 行后", BusinessExplanation: "验证 CRLF 内容不被改写",
				Patch: []string{"@@ -1,1 +1,2 @@", " one\r", "+ONE\r"},
			}},
		}}}},
	}
	out, res, err := Run(doc, mustMarshal(t, doc), repo.Dir)
	if err != nil || !res.OK {
		t.Fatalf("fill: %v %+v", err, res.Errors)
	}
	var filled schema.Document
	if err := json.Unmarshal(out, &filled); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got := filled.Chains[0].Intents[0].Changes[0].Lines[0].Content; got != "one\r" {
		t.Fatalf("CR 被剥掉: %q", got)
	}
	vres, err := validate.Run(filled, repo.Dir, validate.Options{})
	if err != nil || !vres.OK {
		t.Fatalf("validate: %v %+v", err, vres.Errors)
	}
}
