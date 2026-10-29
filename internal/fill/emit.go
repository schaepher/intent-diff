package fill

import (
	"encoding/json"
	"fmt"

	"intent-review/internal/schema"
)

type key struct {
	Chain  int
	Intent int
	Change int
}

type changeUpdate struct {
	OldStart int
	NewStart int
	Lines    []schema.Line
}

// emit 把填充结果写回原始 JSON 树：只增改 lines/old_start/new_start 并删除
// patch，其余字段（含未知字段、注释型扩展）原样保留。
func emit(raw []byte, updates map[key]changeUpdate) ([]byte, error) {
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, fmt.Errorf("解析原始 JSON 失败: %w", err)
	}
	chains, ok := root["chains"].([]any)
	if !ok {
		return nil, fmt.Errorf("chains 不是数组")
	}
	for ci, cv := range chains {
		chain, ok := cv.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("chains[%d] 不是对象", ci)
		}
		intents, ok := chain["intents"].([]any)
		if !ok {
			return nil, fmt.Errorf("chains[%d].intents 不是数组", ci)
		}
		for ii, iv := range intents {
			intent, ok := iv.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("chains[%d].intents[%d] 不是对象", ci, ii)
			}
			changes, ok := intent["changes"].([]any)
			if !ok {
				return nil, fmt.Errorf("chains[%d].intents[%d].changes 不是数组", ci, ii)
			}
			for k, chv := range changes {
				ch, ok := chv.(map[string]any)
				if !ok {
					return nil, fmt.Errorf("chains[%d].intents[%d].changes[%d] 不是对象", ci, ii, k)
				}
				up, ok := updates[key{ci, ii, k}]
				if !ok {
					continue
				}
				delete(ch, "patch")
				ch["old_start"] = up.OldStart
				ch["new_start"] = up.NewStart
				ch["lines"] = linesJSON(up.Lines)
			}
		}
	}
	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
}

func linesJSON(lines []schema.Line) []any {
	out := make([]any, 0, len(lines))
	for _, l := range lines {
		out = append(out, map[string]any{"type": l.Type, "content": l.Content})
	}
	return out
}
