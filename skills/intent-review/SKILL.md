---
name: intent-review
description: 把一次代码变更拆成有序意图链，产出 intent-review JSON，并经过 fill/validate/render 生成离线评审页
---

# intent-review skill

你的任务：读取仓库的 base→head 变更，写出**有序意图链**的 JSON（含业务解释与增量 patch），
用 `intent-review fill` 与 `intent-review validate --json` 迭代到 `ok: true`，最后 `render`。

工具只做确定性渲染与链条校验；**拆解质量与业务解释由你负责**。

## 硬性约束（违反会被 schema/校验拒绝）

1. **每个 change 必须写 `business_explanation`**，回答“为什么这个位置需要这样改”，
   不许写代码转述（“这里加了 if”），禁止 `null`/空串。
2. **意图 i 的 patch 相对 State(i-1)**，不是相对 base，也不是最终 diff 的片段。
   上一意图已经引入的代码，在当前意图中必须作为 patch 的 context 行出现。
3. **一个 change = 一个 hunk**。同一文件多处修改拆成多个 change（同一意图内数组顺序应用），
   每个 change 单独写业务解释；后一个 change 的行号相对“应用它时的状态”计算。
4. **不要提前泄露后续意图的代码**：意图 i 的 context/add 里不得出现意图 j>i 的变更。
5. **链内严格线性**。完全独立的需求建独立 chain；有依赖的变更放同一链的先后意图。
6. **一个意图只记增量**：可以撤销/改写前序意图引入的代码（表现为 `del` 或覆盖 `add`）。

## 流程

### 1. 收集事实

```bash
git log --oneline <base>..<head>
git diff --stat <base> <head>
git diff <base> <head>            # 只看，不要直接按 hunk 分配给意图！
```

需要时读调用方、测试、配置，确认每个变更的**业务理由**。

覆盖检查默认排除 `*_test.go`、`**/vendor/**`、`**/node_modules/**`：这些文件的变更**不需要**
写意图（要改默认列表用 `--exclude` / `--unexclude` 或 `.intent-review.json`，见 README）。
其余变更文件必须被某条链覆盖；被排除的文件一旦写进 change，仍会逐行重放校验。

### 2. 识别独立需求

- 完全独立的需求 → 各自一条 `chain`（链间状态隔离，各自从自己的 `base_commit` 重放）。
- 有依赖 → 同一条链的先后意图。

### 3. 链内规划意图

- 每个意图一句话业务目标（`title`）+ 整体业务解释（`summary`）。
- 边界按“业务理由不同”划分，**不要按文件或 hunk 划分**。
- 可选 `business_rationale`（为什么现在做）、`risk_notes`（风险/未覆盖场景）。

### 4. 逐意图重放（关键）

从 base 开始，在心里维护文件状态；对每个意图：

- 只写该意图引入的**增量** patch（unified diff 行数组，恰好一个 hunk）。
- 上一意图的结果就是当前意图的旧状态：patch 的 context 行必须与之逐字一致。
- 每个 hunk 至少 3 行 context，降低 fill 定位失败概率。
- 新增文件：`"new_file": true`，hunk 头 `@@ -0,0 +1,N @@`。
- 删除文件：`"deleted": true`，hunk 头 `@@ -1,N +0,0 @@`，全部为 `-` 行。
- 重命名：`"renamed_from": "旧路径"`，`file` 写新路径（保留 rename 语义，不拆成删除+新增）。
- 二进制文件：`"binary": true`，不需要 lines/patch，页面只展示说明块。

### 5. 输出骨架 JSON

```json
{
  "schema_version": "1.0",
  "meta": {
    "title": "本次评审标题",
    "base_commit": "<base sha 或 ref>",
    "head_ref": "<head sha / 分支>",
    "generated_by": "codex",
    "skill_version": "1.0"
  },
  "chains": [
    {
      "id": "chain-rate-limit",
      "title": "请求限流",
      "base_commit": "<可选，缺省继承 meta>",
      "intents": [
        {
          "id": "intent-001",
          "title": "引入请求级限流",
          "summary": "在 Handler 入口加令牌桶限流，防止下游过载。",
          "business_rationale": "高峰期下游被击穿。",
          "risk_notes": "阈值待运维确认。",
          "changes": [
            {
              "file": "src/gateway/handler.go",
              "location": "Handler 入口（限流判断前）",
              "business_explanation": "限流必须紧贴入口且在下游调用之前，才能挡住被放大的请求量。",
              "patch": [
                "@@ -12,6 +12,10 @@",
                " func Handler(w http.ResponseWriter, r *http.Request) {",
                "+    if !limiter.Allow() {",
                "+        http.Error(w, \"rate limited\", 429)",
                "+        return",
                "+    }",
                "     resp := callDownstream(r)"
              ]
            }
          ]
        }
      ]
    }
  ]
}
```

需要的完整 schema：`intent-review schema > schema.json`。

### 6. 填充 + 校验 + 修复循环

```bash
intent-review fill review.json --repo . -o review.filled.json
intent-review validate review.filled.json --repo . --json
```

- `fill` 成功但出现 warning：读 warning（常见是上下文不足 / 行号过期被唯一回退），
  修正 patch 后重跑，不要忽略。
- `validate` 失败：读 `errors[]`，按 `code` 修复：

| code | 含义 | 修复方向 |
|---|---|---|
| `SCHEMA_INVALID` | 结构/类型/必填项错误（含 patch-only 输入） | 补字段；先 `fill` 再 validate |
| `SCHEMA_VERSION_UNSUPPORTED` | major 版本不匹配 | 用 `1.x` |
| `MISSING_BUSINESS_EXPLANATION` | 业务解释为空 | 写“为什么” |
| `DUPLICATE_INTENT_ID` / `DUPLICATE_CHAIN_ID` | id 重复 | 链内/全局唯一 |
| `PATCH_PARSE_ERROR` | hunk 头/hunk 数量/行数不符 | 一个 change 一个 hunk；行首保留空格 |
| `PATCH_CONTEXT_MISMATCH` | fill 应用 patch 时上下文不匹配 | 用上一状态重新生成该 hunk |
| `CONTEXT_MISMATCH` | 重放时 before 与上一状态 after 不一致 | 检查意图顺序、行号、是否提前放入后续代码 |
| `FINAL_STATE_MISMATCH` | 重放结果 ≠ head，或 base..head 有变更未被任何链覆盖 | 补漏掉的意图/change（含删除/新增/重命名；整文件漏拆也会报这条）。测试/依赖目录默认不要求覆盖；其它文件不能靠排除列表绕过 |
| `FILE_NOT_FOUND` | base 中不存在该文件 | 新增文件要 `new_file: true` |
| `RENAME_MISMATCH` | renamed_from 与状态不符 | 修正来源路径或拆成新增/删除 |
| `HIGHLIGHT_FALLBACK`（warning） | 无 lexer | 不阻塞；可忽略 |
| `BINARY_CHANGE_SKIPPED`（warning） | 二进制不参与重放 | 确认文件确实无需逐行评审 |

- 只根据**结构化错误**修复，改完重跑，直到 `ok: true`。
- **不要**为了让校验通过而删减业务解释、合并意图或把后续代码塞进 context。

### 7. 渲染

```bash
intent-review render review.filled.json -o review.html --repo .
```

- 你只输出 JSON；**不要手写 HTML/CSS/JS**。
- 渲染失败按 stderr 提示修 JSON；校验失败仍会生成页面（诊断已标到节点），但交付前应修到 ok。

## 反模式（明确错误）

- 按文件拆意图（一个文件一个意图）——应按业务理由拆。
- 把最终 diff 按 hunk 分给不同意图——中间态已丢失，必须逐意图重放。
- 在意图 i 的 context 中提前放入意图 j>i 的代码——校验可能通过但语义错误。
- 业务解释写成代码转述（“这里加了判断”）——必须写业务理由。
- 缺业务解释时留空/写 `null`——schema 直接拒绝。
- 为了过校验而缩小 context（只剩一行）或删掉 `del` 行——掩盖真实差异。
- 同一文件同一意图多处修改塞进一个多 hunk 的 patch——拆成多个 change。

## 输出

最终交付物：

1. `review.filled.json`（canonical，含 lines/行号）
2. `review.html`（`render` 生成）
3. 一句话说明：`validate` 的 `ok: true`（或仍存在的 warning 及原因）
