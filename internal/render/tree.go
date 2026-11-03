package render

import (
	"fmt"
	"net/url"
	"sort"
	"strings"

	"intent-review/internal/schema"
)

// treeView 是“全局意图树”：按 intent.parent 组织的森林（允许分叉）。
// 它只用于全局阅读意图关系，不参与链内重放（重放仍是 changes 数组顺序）。
type treeView struct {
	// Root 是固定在顶部的项目根节点（meta.title），所有链的根意图挂在它下面。
	Root       treeNodeView
	Total      int
	ChainCount int
	Branches   int
}

type treeNodeView struct {
	ChainID    string
	ChainTitle string
	IntentID   string
	Title      string
	Summary    string
	Number     int
	Changes    int
	Repos      []string
	Href       string
	Orphan     bool
	// Synthetic 表示项目根节点（不指向具体意图，只用于把多条链收拢成一棵树）。
	Synthetic bool
	// Meta 是项目根节点上的统计行（链数/意图数/分支数）。
	Meta     string
	Children []treeNodeView
}

type treeNode struct {
	key    schema.IntentKey
	chain  schema.Chain
	intent schema.Intent
	number int
}

// buildTree 构建意图树。默认结构：链内未写 parent 时，意图挂到链内前一个意图下
// （每链首意图是根），因此任何文档都会得到自上而下的树；写 parent 用于分支/跨链。
// 显式 parent 缺失/歧义/环不阻塞渲染：相关节点挂到根上并标记 Orphan，
// 真正的诊断由 validate 报出。
func buildTree(doc schema.Document) treeView {
	nodes := map[schema.IntentKey]treeNode{}
	var order []schema.IntentKey
	for _, c := range doc.Chains {
		for ii, in := range c.Intents {
			key := schema.IntentKey{ChainID: c.ID, IntentID: in.ID}
			nodes[key] = treeNode{key: key, chain: c, intent: in, number: ii + 1}
			order = append(order, key)
		}
	}
	children := map[schema.IntentKey][]schema.IntentKey{}
	orphan := map[schema.IntentKey]bool{}
	lastInChain := map[string]schema.IntentKey{}
	var roots []schema.IntentKey
	for _, key := range order {
		n := nodes[key]
		prev, hasPrev := lastInChain[n.chain.ID]
		lastInChain[n.chain.ID] = key
		if strings.TrimSpace(n.intent.Parent) == "" {
			// 默认结构：链内线性挂到前一个意图；每链首意图是根。
			if hasPrev {
				children[prev] = append(children[prev], key)
			} else {
				roots = append(roots, key)
			}
			continue
		}
		target, st := doc.ResolveParent(key, n.intent.Parent)
		if st != schema.ParentOK {
			orphan[key] = true
			roots = append(roots, key)
			continue
		}
		children[target] = append(children[target], key)
	}

	visited := map[schema.IntentKey]bool{}
	var walk func(key schema.IntentKey, asOrphan bool) treeNodeView
	walk = func(key schema.IntentKey, asOrphan bool) treeNodeView {
		visited[key] = true
		n := nodes[key]
		v := treeNodeView{
			ChainID:    n.chain.ID,
			ChainTitle: n.chain.Title,
			IntentID:   n.intent.ID,
			Title:      n.intent.Title,
			Summary:    n.intent.Summary,
			Number:     n.number,
			Changes:    len(n.intent.Changes),
			Repos:      treeRepos(doc, n.intent),
			Href:       treeHref(n.chain.ID, n.intent.ID),
			Orphan:     asOrphan || orphan[key],
		}
		for _, child := range children[key] {
			if !visited[child] {
				v.Children = append(v.Children, walk(child, false))
			}
		}
		return v
	}

	tv := treeView{Total: len(order), ChainCount: len(doc.Chains)}
	var realRoots []treeNodeView
	for _, key := range roots {
		if !visited[key] {
			realRoots = append(realRoots, walk(key, false))
		}
	}
	// 环内节点从根不可达：仍展示出来（标 Orphan），避免页面无声丢意图。
	for _, key := range order {
		if !visited[key] {
			realRoots = append(realRoots, walk(key, true))
		}
	}
	for _, kids := range children {
		if len(kids) > 1 {
			tv.Branches++
		}
	}
	// 顶部固定一个项目根节点：多链/多根都收拢到它下面，页面始终只有一棵树。
	title := strings.TrimSpace(doc.Meta.Title)
	if title == "" {
		title = "本次评审"
	}
	meta := fmt.Sprintf("%d 条链 · %d 个意图", len(doc.Chains), tv.Total)
	if tv.Branches > 0 {
		meta += fmt.Sprintf(" · %d 处分支", tv.Branches)
	}
	tv.Root = treeNodeView{
		Title:     title,
		Synthetic: true,
		Meta:      meta,
		Summary:   "本轮评审的全局意图关系（自上而下阅读）",
		Children:  realRoots,
	}
	return tv
}

// treeHref 指向对应链意图的深链（change 1 基）。
func treeHref(chainID, intentID string) string {
	return "#" + url.PathEscape(chainID) + "/" + url.PathEscape(intentID) + "/1"
}

func treeRepos(doc schema.Document, in schema.Intent) []string {
	if !doc.MultiRepo() {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for _, ch := range in.Changes {
		if ch.Repo != "" && !seen[ch.Repo] {
			seen[ch.Repo] = true
			out = append(out, ch.Repo)
		}
	}
	sort.Strings(out)
	return out
}
