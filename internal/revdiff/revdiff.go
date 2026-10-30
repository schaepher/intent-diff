// Package revdiff 比较两版评审文档，回答“这次更新改了哪些意图、改在哪”。
//
// 比较维度与 SKILL 的反馈要求一致：意图增删、标题/摘要、文件集合、文件顺序
// （display_order）、函数分组（display_group）、解释内容（business_explanation）、
// 代码内容（lines/patch）。只读比较，不依赖 git。
package revdiff

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"intent-review/internal/schema"
)

// Report 是一次比较结果（只列出有变化的意图）。
type Report struct {
	Intents []IntentChange `json:"intents"`
}

// IntentChange 描述一个意图的变化。
type IntentChange struct {
	ChainID      string   `json:"chain_id"`
	IntentID     string   `json:"intent_id"`
	Title        string   `json:"title,omitempty"`
	Status       string   `json:"status"` // added / removed / modified
	Dimensions   []string `json:"dimensions,omitempty"`
	FilesAdded   []string `json:"files_added,omitempty"`
	FilesRemoved []string `json:"files_removed,omitempty"`
	OrderBefore  []string `json:"order_before,omitempty"`
	OrderAfter   []string `json:"order_after,omitempty"`
	// ChangedChanges 逐 change 说明改了什么（优先用稳定 id）：如 "ch-002(解释内容)"。
	ChangedChanges []string `json:"changed_changes,omitempty"`
}

// Empty 表示两版文档没有意图级变化。
func (r Report) Empty() bool { return len(r.Intents) == 0 }

// JSON 返回结构化报告。
func (r Report) JSON() ([]byte, error) { return json.MarshalIndent(r, "", "  ") }

type intentKey struct{ chain, intent string }

type changeEntry struct {
	key   string
	label string
	ch    schema.Change
}

// Compare 比较两版文档。
func Compare(before, after schema.Document) Report {
	oldIdx, oldOrder := indexIntents(before)
	newIdx, newOrder := indexIntents(after)
	var rep Report
	for _, k := range newOrder {
		next := newIdx[k]
		prev, ok := oldIdx[k]
		if !ok {
			rep.Intents = append(rep.Intents, IntentChange{
				ChainID: k.chain, IntentID: k.intent, Title: next.Title, Status: "added",
			})
			continue
		}
		if ic, changed := compareIntent(k, prev, next); changed {
			rep.Intents = append(rep.Intents, ic)
		}
	}
	for _, k := range oldOrder {
		if _, ok := newIdx[k]; ok {
			continue
		}
		rep.Intents = append(rep.Intents, IntentChange{
			ChainID: k.chain, IntentID: k.intent, Title: oldIdx[k].Title, Status: "removed",
		})
	}
	return rep
}

func indexIntents(doc schema.Document) (map[intentKey]schema.Intent, []intentKey) {
	idx := map[intentKey]schema.Intent{}
	var order []intentKey
	for _, c := range doc.Chains {
		for _, in := range c.Intents {
			k := intentKey{c.ID, in.ID}
			idx[k] = in
			order = append(order, k)
		}
	}
	return idx, order
}

func compareIntent(k intentKey, prev, next schema.Intent) (IntentChange, bool) {
	ic := IntentChange{ChainID: k.chain, IntentID: k.intent, Title: next.Title, Status: "modified"}
	dims := map[string]bool{}
	if prev.Title != next.Title {
		dims["title"] = true
	}
	if prev.Summary != next.Summary {
		dims["summary"] = true
	}
	oldIdx, newIdx := indexChanges(prev.Changes), indexChanges(next.Changes)
	matched := map[string]bool{}
	for _, nc := range newIdx.entries {
		oc, ok := matchChange(oldIdx, nc)
		if !ok {
			ic.FilesAdded = append(ic.FilesAdded, nc.label)
			dims["files"] = true
			continue
		}
		matched[oc.key] = true
		var per []string
		if oc.ch.DisplayGroup != nc.ch.DisplayGroup {
			dims["group"] = true
			per = append(per, "函数分组")
		}
		if oc.ch.BusinessExplanation != nc.ch.BusinessExplanation {
			dims["explanation"] = true
			per = append(per, "解释内容")
		}
		if changeBodyDiffers(oc.ch, nc.ch) {
			dims["diff"] = true
			per = append(per, "代码内容")
		}
		if len(per) > 0 {
			ic.ChangedChanges = append(ic.ChangedChanges, fmt.Sprintf("%s(%s)", changeRef(nc.ch), strings.Join(per, "/")))
		}
	}
	for _, oc := range oldIdx.entries {
		if !matched[oc.key] {
			ic.FilesRemoved = append(ic.FilesRemoved, oc.label)
			dims["files"] = true
		}
	}
	// 文件顺序：只在集合相同时比较展示顺序，避免重命名/新增带来的噪声。
	if len(ic.FilesAdded) == 0 && len(ic.FilesRemoved) == 0 && !sameSeq(oldIdx.seq, newIdx.seq) {
		dims["order"] = true
		ic.OrderBefore, ic.OrderAfter = oldIdx.seq, newIdx.seq
	}
	if len(dims) == 0 {
		return ic, false
	}
	for _, d := range dimensionOrder {
		if dims[d] {
			ic.Dimensions = append(ic.Dimensions, d)
		}
	}
	sort.Strings(ic.FilesAdded)
	sort.Strings(ic.FilesRemoved)
	return ic, true
}

// changeIndex 是意图内 change 的索引：按 (repo,file,location) 与稳定 id 双路匹配。
type changeIndex struct {
	byKey   map[string]changeEntry
	byID    map[string]changeEntry
	entries []changeEntry // 展示顺序
	seq     []string      // 展示顺序的标签
}

// indexChanges 建索引；重复三元组追加序号区分（增删场景下仍能稳定匹配）。
func indexChanges(changes []schema.Change) changeIndex {
	idx := changeIndex{byKey: map[string]changeEntry{}, byID: map[string]changeEntry{}}
	for _, k := range displaySequence(changes) {
		ch := changes[k]
		base := ch.Repo + "\x00" + ch.File + "\x00" + ch.Location
		key := base
		for n := 2; ; n++ {
			if _, ok := idx.byKey[key]; !ok {
				break
			}
			key = fmt.Sprintf("%s#%d", base, n)
		}
		entry := changeEntry{key: key, label: changeLabel(ch), ch: ch}
		idx.byKey[key] = entry
		idx.entries = append(idx.entries, entry)
		idx.seq = append(idx.seq, entry.label)
		if id := strings.TrimSpace(ch.ID); id != "" {
			idx.byID[id] = entry
		}
	}
	return idx
}

// matchChange 优先按稳定 id 匹配（改了 location/文件顺序也能对上），否则退回三元组。
func matchChange(old changeIndex, nc changeEntry) (changeEntry, bool) {
	if id := strings.TrimSpace(nc.ch.ID); id != "" {
		if oc, ok := old.byID[id]; ok {
			return oc, true
		}
	}
	oc, ok := old.byKey[nc.key]
	return oc, ok
}

// changeRef 返回摘要里的 change 标识（优先稳定 id）。
func changeRef(ch schema.Change) string {
	if id := strings.TrimSpace(ch.ID); id != "" {
		return id
	}
	return changeLabel(ch)
}

// changeLabel 生成变更标识：repo:file（location）。同一文件多个 change 时靠 location 区分。
func changeLabel(ch schema.Change) string {
	label := ch.File
	if ch.Repo != "" {
		label = ch.Repo + ":" + label
	}
	if loc := strings.TrimSpace(ch.Location); loc != "" {
		label += " @ " + loc
	}
	return label
}

// displaySequence 返回展示顺序的声明下标（与 render 同一规则）。
func displaySequence(chs []schema.Change) []int {
	idx := make([]int, len(chs))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool {
		oa, ob := chs[idx[a]].DisplayOrder, chs[idx[b]].DisplayOrder
		switch {
		case oa == 0:
			return false
		case ob == 0:
			return true
		default:
			return oa < ob
		}
	})
	return idx
}

func sameSeq(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func changeBodyDiffers(a, b schema.Change) bool {
	if len(a.Lines) != len(b.Lines) || len(a.Patch) != len(b.Patch) {
		return true
	}
	for i := range a.Lines {
		if a.Lines[i] != b.Lines[i] {
			return true
		}
	}
	for i := range a.Patch {
		if a.Patch[i] != b.Patch[i] {
			return true
		}
	}
	return false
}
