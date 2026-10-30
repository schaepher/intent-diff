package render

import (
	"sort"

	"intent-review/internal/schema"
)

// displayOrder 返回 change 的展示顺序（声明下标序列）：
// 按 display_order 升序；未设置（0）排在已设置之后，同值保持声明顺序。
// 该顺序只用于页面展示，深链与校验错误仍按声明下标（1 基）。
func displayOrder(chs []schema.Change) []int {
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
