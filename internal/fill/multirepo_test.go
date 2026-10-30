package fill

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"intent-review/internal/schema"
	"intent-review/internal/testutil"
)

func TestRunMultiRepoFillsPatchInBothRepos(t *testing.T) {
	project := t.TempDir()
	web := testutil.NewGitRepoAt(t, filepath.Join(project, "web"), map[string]string{"src/a.ts": "one\n"})
	api := testutil.NewGitRepoAt(t, filepath.Join(project, "api"), map[string]string{"b.go": "package api\n\nvar A = 1\n"})
	webBase, apiBase := web.Head(), api.Head()
	web.Write("src/a.ts", "two\n")
	api.Write("b.go", "package api\n\nvar A = 2\n")
	webHead, apiHead := web.Commit("web"), api.Commit("api")

	doc := schema.Document{
		SchemaVersion: schema.SchemaVersion,
		Meta:          schema.Meta{Title: "跨仓需求"},
		Repos: []schema.Repo{
			{ID: "web", Path: "web", BaseCommit: webBase, HeadRef: webHead},
			{ID: "api", Path: "api", BaseCommit: apiBase, HeadRef: apiHead},
		},
		Chains: []schema.Chain{{ID: "c1", Title: "跨仓", Intents: []schema.Intent{{
			ID: "i1", Title: "两端", Summary: "同一需求跨仓",
			Changes: []schema.Change{
				{
					Repo: "api", File: "b.go", Location: "A", BusinessExplanation: "后端改值", DisplayOrder: 2,
					Patch: []string{"@@ -3,1 +3,1 @@", "-var A = 1", "+var A = 2"},
				},
				{
					Repo: "web", File: "src/a.ts", Location: "第 1 行", BusinessExplanation: "前端改值", DisplayOrder: 1,
					Patch: []string{"@@ -1,1 +1,1 @@", "-one", "+two"},
				},
			},
		}}}},
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	out, res, err := Run(doc, raw, project)
	if err != nil {
		t.Fatalf("fill: %v", err)
	}
	if !res.OK {
		t.Fatalf("errors = %+v", res.Errors)
	}
	var got struct {
		Chains []struct {
			Intents []struct {
				Changes []struct {
					Repo         string          `json:"repo"`
					File         string          `json:"file"`
					DisplayOrder int             `json:"display_order"`
					NewStart     int             `json:"new_start"`
					Patch        json.RawMessage `json:"patch"`
					Lines        []schema.Line   `json:"lines"`
				} `json:"changes"`
			} `json:"intents"`
		} `json:"chains"`
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, out)
	}
	changes := got.Chains[0].Intents[0].Changes
	if len(changes) != 2 {
		t.Fatalf("changes = %d", len(changes))
	}
	if changes[0].Repo != "api" || changes[0].Patch != nil || len(changes[0].Lines) == 0 || changes[0].NewStart != 3 {
		t.Fatalf("api change = %+v", changes[0])
	}
	if changes[1].Repo != "web" || changes[1].DisplayOrder != 1 || len(changes[1].Lines) == 0 {
		t.Fatalf("web change = %+v", changes[1])
	}
	// 展示顺序字段必须原样保留（重放顺序仍是数组顺序）。
	if changes[0].DisplayOrder != 2 {
		t.Fatalf("display_order 丢失: %+v", changes[0])
	}
}

func TestRunMultiRepoUnknownRepoIsReported(t *testing.T) {
	project := t.TempDir()
	web := testutil.NewGitRepoAt(t, filepath.Join(project, "web"), map[string]string{"a.txt": "one\n"})
	base := web.Head()
	web.Write("a.txt", "two\n")
	head := web.Commit("change")
	doc := schema.Document{
		SchemaVersion: schema.SchemaVersion,
		Meta:          schema.Meta{Title: "t"},
		Repos:         []schema.Repo{{ID: "web", Path: "web", BaseCommit: base, HeadRef: head}},
		Chains: []schema.Chain{{ID: "c", Title: "c", Intents: []schema.Intent{{
			ID: "i", Title: "i", Summary: "s",
			Changes: []schema.Change{{
				Repo: "nope", File: "a.txt", Location: "1", BusinessExplanation: "x",
				Patch: []string{"@@ -1,1 +1,1 @@", "-one", "+two"},
			}},
		}}}},
	}
	raw, _ := json.Marshal(doc)
	_, res, err := Run(doc, raw, project)
	if err != nil {
		t.Fatalf("fill: %v", err)
	}
	if res.OK || len(res.Errors) == 0 {
		t.Fatalf("未声明仓库应报错: %+v", res)
	}
}
