package schema

import "encoding/json"

// JSONSchema 是随二进制分发的 JSON Schema（设计文档 §6.3），
// 供 agent 与编辑器做补全/校验；`intent-review schema` 原样输出。
const JSONSchema = `{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "$id": "https://intent-review/schema/1.1.json",
  "title": "intent-review document",
  "type": "object",
  "required": ["schema_version", "meta", "chains"],
  "additionalProperties": true,
  "properties": {
    "schema_version": {
      "type": "string",
      "pattern": "^1\\.[0-9]+$",
      "description": "major 必须为 1；minor 升级时未知字段被忽略"
    },
    "meta": {
      "type": "object",
      "required": ["title", "base_commit", "head_ref"],
      "additionalProperties": true,
      "properties": {
        "title": {"type": "string", "minLength": 1},
        "base_commit": {"type": "string", "minLength": 1},
        "head_ref": {"type": "string", "minLength": 1},
        "generated_by": {"type": "string"},
        "skill_version": {"type": "string"}
      }
    },
    "repos": {
      "type": "array",
      "description": "跨仓评审涉及的仓库；省略时是单仓评审（--repo 指向该仓库）",
      "minItems": 1,
      "items": {"$ref": "#/definitions/repo"}
    },
    "file_filter": {
      "type": "object",
      "description": "文档内统一过滤配置：只影响 base..head 覆盖检查，不删除源码",
      "additionalProperties": false,
      "properties": {
        "exclude": {"type": "array", "items": {"type": "string"}, "description": "追加排除项（gitignore 风格 glob）"},
        "unexclude": {"type": "array", "items": {"type": "string"}, "description": "从排除列表按字面值删除"}
      }
    },
    "chains": {
      "type": "array",
      "minItems": 1,
      "items": {"$ref": "#/definitions/chain"}
    }
  },
  "definitions": {
    "repo": {
      "type": "object",
      "required": ["id"],
      "additionalProperties": true,
      "properties": {
        "id": {"type": "string", "minLength": 1},
        "title": {"type": "string"},
        "path": {"type": "string", "description": "仓库 checkout 相对项目根（--repo）的路径；空表示项目根本身"},
        "base_commit": {"type": "string", "description": "缺省继承 meta.base_commit"},
        "head_ref": {"type": "string", "description": "缺省继承 meta.head_ref"},
        "source_root": {"type": "string", "description": "VS Code 跳转用的真实源码根（避免跳到重放快照）"}
      }
    },
    "chain": {
      "type": "object",
      "required": ["id", "title", "intents"],
      "additionalProperties": true,
      "properties": {
        "id": {"type": "string", "minLength": 1},
        "title": {"type": "string", "minLength": 1},
        "base_commit": {"type": "string", "description": "缺省继承 meta.base_commit（多仓时对所有仓生效，逐仓用 base_commits）"},
        "base_commits": {
          "type": "object",
          "description": "逐仓覆盖 base：{repoID: commit}",
          "additionalProperties": {"type": "string"}
        },
        "intents": {"type": "array", "minItems": 1, "items": {"$ref": "#/definitions/intent"}}
      }
    },
    "intent": {
      "type": "object",
      "required": ["id", "title", "summary", "changes"],
      "additionalProperties": true,
      "properties": {
        "id": {"type": "string", "minLength": 1},
        "title": {"type": "string", "minLength": 1},
        "summary": {"type": "string", "minLength": 1},
        "business_rationale": {"type": "string"},
        "risk_notes": {"type": "string"},
        "changes": {"type": "array", "minItems": 1, "items": {"$ref": "#/definitions/change"}}
      }
    },
    "change": {
      "type": "object",
      "required": ["file", "location", "business_explanation"],
      "additionalProperties": true,
      "anyOf": [
        {"required": ["lines"]},
        {"required": ["patch"]},
        {"required": ["binary"]}
      ],
      "properties": {
        "id": {"type": "string", "description": "意图内稳定标识（修订时保持不变，供 diff/引用定位）"},
        "repo": {"type": "string", "description": "多仓文档必填：repos[] 中的仓库 id"},
        "file": {"type": "string", "minLength": 1, "description": "仓库相对路径"},
        "location": {"type": "string", "minLength": 1, "description": "人类可读的位置描述"},
        "business_explanation": {
          "type": "string",
          "minLength": 1,
          "description": "为什么这里要改；禁止 null/空串"
        },
        "old_start": {"type": "integer", "minimum": 0, "description": "在 State(i-1) 中的起始行号；new_file 为 0"},
        "new_start": {"type": "integer", "minimum": 0, "description": "在 State(i) 中的起始行号；deleted 为 0"},
        "lines": {"type": "array", "minItems": 1, "items": {"$ref": "#/definitions/line"}},
        "patch": {
          "type": "array",
          "minItems": 1,
          "items": {"type": "string"},
          "description": "unified diff 行数组（恰好一个 hunk）；fill 后转为 lines 并删除"
        },
        "renamed_from": {"type": "string", "description": "重命名来源路径（保留 rename 语义）"},
        "binary": {"type": "boolean"},
        "deleted": {"type": "boolean"},
        "new_file": {"type": "boolean"},
        "display_order": {
          "type": "integer",
          "minimum": 0,
          "description": "页面展示顺序（小的先展示）；不影响 patch 重放顺序。未设置排在已设置之后"
        },
        "display_group": {"type": "string", "description": "页面展示分组（如 页面入口/主流程/辅助函数/配置）"},
        "display_group_note": {"type": "string", "description": "分组说明（同组取首个非空值）"},
        "references": {
          "type": "array",
          "items": {"type": "string"},
          "description": "指向本链后续实现：<intent-id>/<change-id 或 1 基下标> 或 <intent-id>#<display_group>"
        }
      }
    },
    "line": {
      "type": "object",
      "required": ["type", "content"],
      "additionalProperties": false,
      "properties": {
        "type": {"type": "string", "enum": ["context", "add", "del"]},
        "content": {"type": "string"}
      }
    }
  }
}`

// SchemaJSON 返回格式化后的 JSON Schema 字节。
func SchemaJSON() ([]byte, error) {
	var buf map[string]any
	if err := json.Unmarshal([]byte(JSONSchema), &buf); err != nil {
		return nil, err
	}
	return json.MarshalIndent(buf, "", "  ")
}
