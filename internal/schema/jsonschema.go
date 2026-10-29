package schema

import "encoding/json"

// JSONSchema 是随二进制分发的 JSON Schema（设计文档 §6.3），
// 供 agent 与编辑器做补全/校验；`intent-review schema` 原样输出。
const JSONSchema = `{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "$id": "https://intent-review/schema/1.0.json",
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
    "chains": {
      "type": "array",
      "minItems": 1,
      "items": {"$ref": "#/definitions/chain"}
    }
  },
  "definitions": {
    "chain": {
      "type": "object",
      "required": ["id", "title", "intents"],
      "additionalProperties": true,
      "properties": {
        "id": {"type": "string", "minLength": 1},
        "title": {"type": "string", "minLength": 1},
        "base_commit": {"type": "string", "description": "缺省继承 meta.base_commit"},
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
        "new_file": {"type": "boolean"}
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
