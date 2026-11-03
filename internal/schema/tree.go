package schema

import (
	"fmt"
	"strings"

	"intent-review/internal/report"
)

// IntentKey 是全局唯一的意图定位（链 id + 意图 id）。
type IntentKey struct {
	ChainID  string
	IntentID string
}

// ParentStatus 是 intent.parent 的解析结果。
type ParentStatus int

const (
	// ParentOK 表示解析到唯一父意图。
	ParentOK ParentStatus = iota
	// ParentMissing 表示找不到父意图（或引用格式为空）。
	ParentMissing
	// ParentAmbiguous 表示裸 intent id 在多个链里重名。
	ParentAmbiguous
	// ParentSelf 表示 parent 指向自己。
	ParentSelf
)

// ResolveParent 解析 intent.parent：
//   - "<chain-id>/<intent-id>"：跨链/同链的精确引用；
//   - "<intent-id>"：仅当该 id 在整个文档中唯一时才成立。
//
// 该关系只用于“全局意图树”展示，不参与链内重放顺序（重放仍是 changes 数组顺序）。
func (d Document) ResolveParent(from IntentKey, parent string) (IntentKey, ParentStatus) {
	parent = strings.TrimSpace(parent)
	if parent == "" {
		return IntentKey{}, ParentMissing
	}
	if strings.Contains(parent, "/") {
		parts := strings.SplitN(parent, "/", 2)
		key := IntentKey{ChainID: strings.TrimSpace(parts[0]), IntentID: strings.TrimSpace(parts[1])}
		if key.ChainID == "" || key.IntentID == "" {
			return key, ParentMissing
		}
		if _, ok := d.intentByKey(key); !ok {
			return key, ParentMissing
		}
		if key == from {
			return key, ParentSelf
		}
		return key, ParentOK
	}
	var found []IntentKey
	for _, c := range d.Chains {
		for _, in := range c.Intents {
			if in.ID == parent {
				found = append(found, IntentKey{ChainID: c.ID, IntentID: in.ID})
			}
		}
	}
	switch len(found) {
	case 0:
		return IntentKey{}, ParentMissing
	case 1:
		if found[0] == from {
			return found[0], ParentSelf
		}
		return found[0], ParentOK
	default:
		return IntentKey{}, ParentAmbiguous
	}
}

func (d Document) intentByKey(k IntentKey) (Intent, bool) {
	for _, c := range d.Chains {
		if c.ID != k.ChainID {
			continue
		}
		for _, in := range c.Intents {
			if in.ID == k.IntentID {
				return in, true
			}
		}
	}
	return Intent{}, false
}

// CheckIntentTree 校验 parent 引用：缺失/歧义/自引用/环。
// 只是展示关系，所以复用 SCHEMA_INVALID（不新增错误码）。
func CheckIntentTree(doc Document) []report.Error {
	var errs []report.Error
	parents := map[IntentKey]IntentKey{}
	var order []IntentKey
	for _, c := range doc.Chains {
		for _, in := range c.Intents {
			key := IntentKey{ChainID: c.ID, IntentID: in.ID}
			order = append(order, key)
			if strings.TrimSpace(in.Parent) == "" {
				continue
			}
			target, st := doc.ResolveParent(key, in.Parent)
			loc := func() report.Error {
				return report.Error{Code: report.CodeSchemaInvalid, ChainID: c.ID, IntentID: in.ID}
			}
			switch st {
			case ParentOK:
				parents[key] = target
			case ParentSelf:
				e := loc()
				e.Message = "intent.parent 不能指向自己"
				e.Hint = "用 <chain-id>/<intent-id> 指向父意图；分支用多个子意图表达"
				errs = append(errs, e)
			case ParentAmbiguous:
				e := loc()
				e.Message = fmt.Sprintf("intent.parent %q 在多个链里重名", in.Parent)
				e.Hint = "改用 <chain-id>/<intent-id> 精确引用"
				errs = append(errs, e)
			default:
				e := loc()
				e.Message = fmt.Sprintf("intent.parent %q 找不到对应意图", in.Parent)
				e.Hint = "parent 写法：<chain-id>/<intent-id>，或全局唯一的 intent id"
				errs = append(errs, e)
			}
		}
	}
	reported := map[IntentKey]bool{}
	for _, start := range order {
		if reported[start] {
			continue
		}
		seen := map[IntentKey]bool{}
		cur := start
		for {
			if reported[cur] {
				break
			}
			if seen[cur] {
				e := report.Error{
					Code: report.CodeSchemaInvalid, ChainID: cur.ChainID, IntentID: cur.IntentID,
					Message: "intent.parent 关系形成环（意图树必须无环）",
					Hint:    "分支用多个子意图表示；父子之间不要互相引用",
				}
				errs = append(errs, e)
				for k := range seen {
					reported[k] = true
				}
				break
			}
			seen[cur] = true
			next, ok := parents[cur]
			if !ok {
				break
			}
			cur = next
		}
	}
	return errs
}
