package render

import (
	"fmt"
	"strconv"
	"strings"

	"intent-review/internal/schema"
)

// groupPlan 是一个意图内按展示顺序出现的分组信息。
type groupPlan struct {
	names []string
	index map[string]int
	notes map[string]string
}

// planGroups 按展示顺序收集 display_group：分组顺序、分组索引、分组说明（首个非空）。
func planGroups(chs []schema.Change) groupPlan {
	p := groupPlan{index: map[string]int{}, notes: map[string]string{}}
	for _, k := range displayOrder(chs) {
		ch := chs[k]
		g := strings.TrimSpace(ch.DisplayGroup)
		if g == "" {
			continue
		}
		if _, ok := p.index[g]; !ok {
			p.index[g] = len(p.names)
			p.names = append(p.names, g)
		}
		if _, ok := p.notes[g]; !ok {
			if note := strings.TrimSpace(ch.DisplayGroupNote); note != "" {
				p.notes[g] = note
			}
		}
	}
	return p
}

// groupAnchor 返回分组标题的页面锚点（无分组时为空）。
func groupAnchor(ci, ii int, plan groupPlan, group string) string {
	gi, ok := plan.index[group]
	if !ok {
		return ""
	}
	return fmt.Sprintf("group-c%d-i%d-g%d", ci, ii, gi)
}

// chainRefs 是链内引用索引：把 references 解析成页面链接。
type chainRefs struct {
	change map[string]string // intentID\x00id:<id> / idx:<n> -> change 锚点
	group  map[string]string // intentID\x00<group> -> 分组锚点
	label  map[string]string // intentID\x00id/idx -> 目标说明
}

func newChainRefs(c *schema.Chain, ci int) *chainRefs {
	r := &chainRefs{change: map[string]string{}, group: map[string]string{}, label: map[string]string{}}
	for ii := range c.Intents {
		in := &c.Intents[ii]
		plan := planGroups(in.Changes)
		for gi, name := range plan.names {
			r.group[in.ID+"\x00"+name] = fmt.Sprintf("group-c%d-i%d-g%d", ci, ii, gi)
		}
		for k := range in.Changes {
			ch := &in.Changes[k]
			anchor := fmt.Sprintf("change-c%d-i%d-ch%d", ci, ii, k+1)
			label := ch.File
			if ch.Repo != "" {
				label = ch.Repo + ":" + label
			}
			if loc := strings.TrimSpace(ch.Location); loc != "" {
				label += " @ " + loc
			}
			keys := []string{"idx:" + strconv.Itoa(k+1)}
			if id := strings.TrimSpace(ch.ID); id != "" {
				keys = append(keys, "id:"+id)
			}
			for _, key := range keys {
				r.change[in.ID+"\x00"+key] = anchor
				r.label[in.ID+"\x00"+key] = label
			}
		}
	}
	return r
}

// resolve 解析 references；无法定位的引用退化为纯文本（不编造跳转）。
func (r *chainRefs) resolve(refs []string) []refView {
	out := make([]refView, 0, len(refs))
	for _, ref := range refs {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			continue
		}
		if intentID, group, ok := strings.Cut(ref, "#"); ok {
			intentID, group = strings.TrimSpace(intentID), strings.TrimSpace(group)
			rv := refView{Label: intentID + " · " + group}
			if anchor, found := r.group[intentID+"\x00"+group]; found {
				rv.Href = "#" + anchor
			}
			out = append(out, rv)
			continue
		}
		intentID, target, ok := strings.Cut(ref, "/")
		if !ok {
			out = append(out, refView{Label: ref})
			continue
		}
		intentID, target = strings.TrimSpace(intentID), strings.TrimSpace(target)
		rv := refView{Label: ref}
		for _, key := range []string{"id:" + target, "idx:" + target} {
			if anchor, found := r.change[intentID+"\x00"+key]; found {
				rv.Href = "#" + anchor
				if label := r.label[intentID+"\x00"+key]; label != "" {
					rv.Label = label
				}
				break
			}
		}
		out = append(out, rv)
	}
	return out
}
