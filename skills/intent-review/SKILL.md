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
7. **修订直接合入原意图**：实现方案调整后，改原意图的 patch/解释并重新 `fill`，
   **不要**在链尾追加“修正意图”让前面继续展示过期代码。需要留档的历史方案放
   `.intent-review/history/` 或 git 历史里。
8. **新增函数按调用关系拆分解释**：不要把一个新增文件当成默认解释单位。有调用关系的函数
   串成一条“调用链”解释（同 `display_group` + `display_group_note`）；独立函数各自一个 change
   （同一意图内可用多个 change 顺序追加，各自 `display_group`）。
9. **调用处交代新函数职责并链接实现**：上层引用新函数时，`business_explanation` 说明“它解决什么问题”，
   并用 `references` 指向后面的实现（`"<intent-id>/<change-id>"` 或 `"<intent-id>#<display_group>"`），
   让评审者顺着主流程读、需要时再跳转深入。
10. **展示顺序由表及里**：用 `display_order` 把“页面入口 / 主流程 / 调用方”排在前面，
   再排“辅助函数 / 策略 / 配置定义”；`display_group` 写分组名。**不要**为了展示顺序调整
   `changes` 数组顺序——数组顺序就是重放顺序，会让行号语义变复杂。
11. **跨仓按需声明**：同一需求涉及多个仓库时，顶层写 `repos[]`（id/path/base_commit/head_ref），
   每个 change 写 `repo`；同一意图可以跨仓。单仓文档不要写 `repos`/`repo`。
12. **change 用稳定 id**：每个 change 写 `id`（如 `ch-001`），**修订时保持同一个 id**；
   id 在同一意图内唯一。这样 `diff`/`build` 能准确说明“哪个 change 的解释/分组/代码变了”，
   而不是靠位置猜测。
13. **分组说明写整体逻辑**：同一 `display_group` 的首个 change 写 `display_group_note`，
   说明这组函数/文件在调用链里的位置；单个函数可以再在自己的 `business_explanation` 里讲细节。

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

多仓与展示顺序（可选字段，按需使用）：

```json
{
  "schema_version": "1.1",
  "repos": [
    { "id": "web", "path": "web", "base_commit": "<sha>", "head_ref": "<sha>" },
    { "id": "api", "path": "api", "base_commit": "<sha>", "head_ref": "<sha>" }
  ],
  "file_filter": { "exclude": ["*.md", "*_test.go", "*.pb.go", "*.test.tsx", "go.mod", "go.sum"] },
  "chains": [{
    "id": "chain-x", "title": "需求",
    "intents": [{
      "id": "intent-001", "title": "…", "summary": "…",
      "changes": [
        { "id": "ch-001", "repo": "web", "file": "src/page.ts", "location": "页面入口",
          "business_explanation": "入口先读；Helper 的实现在 intent-002#辅助函数 展开。",
          "references": ["intent-002#辅助函数"],
          "display_order": 10, "display_group": "页面入口",
          "display_group_note": "从页面入口往下读，先入口、再主流程。", "patch": ["…"] },
        { "id": "ch-002", "repo": "api", "file": "handler.go", "location": "Handler 调用方",
          "business_explanation": "Handler 调用 Helper；Helper 负责…，实现在 ch-003。",
          "references": ["intent-001/ch-003"],
          "display_order": 20, "display_group": "主流程", "patch": ["…"] },
        { "id": "ch-003", "repo": "api", "file": "handler.go", "location": "Helper 实现",
          "business_explanation": "Helper 的实际实现。",
          "display_order": 30, "display_group": "辅助函数", "patch": ["…"] }
      ]
    }]
  }]
}
```

单仓示例（完整骨架）：

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
| `DUPLICATE_INTENT_ID` / `DUPLICATE_CHAIN_ID` / `DUPLICATE_REPO_ID` / `DUPLICATE_CHANGE_ID` | id 重复 | 链内/全局/仓库/意图内唯一 |
| `REPO_UNAVAILABLE` | 仓库目录/提交不可用，或 change.repo 未声明 | 检查 `repos[]` 的 path/base/head 与 `--repo` 项目根 |
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
- **每轮调整都重跑 `build`（= fill → validate → render）**，输出路径不变（浏览器刷新即可）；
  `build`/`watch` 会打印“本次更新影响哪些意图，变在文件顺序 / 函数分组 / 解释内容 / 代码内容”，
  不要只说“已更新”。需要对比任意两版材料用 `intent-review diff`。
- 评审产物放 `.intent-review/`（已 gitignore）；收尾可以用 `intent-review clean` 清理。
  业务代码照常 commit/push/部署，**不要提交评审产物**。
- 需要 VS Code 跳转时给 `--source-root`（单仓 `<path>`，多仓 `<repo-id>=<path>`），
  不指定时默认仓库 checkout，不会跳到重放快照。页面里的链接打开的是**本地当前文件**；
  首次点击时浏览器会询问是否允许打开 VS Code，属于浏览器行为（页面不发起网络请求）。
  `change.file` 必须是仓库相对路径（不能绝对路径/`..`），否则链接不会生成。

## 反模式（明确错误）

- 按文件拆意图（一个文件一个意图）——应按业务理由拆。
- 把最终 diff 按 hunk 分给不同意图——中间态已丢失，必须逐意图重放。
- 在意图 i 的 context 中提前放入意图 j>i 的代码——校验可能通过但语义错误。
- 业务解释写成代码转述（“这里加了判断”）——必须写业务理由。
- 缺业务解释时留空/写 `null`——schema 直接拒绝。
- 为了过校验而缩小 context（只剩一行）或删掉 `del` 行——掩盖真实差异。
- 同一文件同一意图多处修改塞进一个多 hunk 的 patch——拆成多个 change。
- 方案调整后在链尾追加“修正意图”——直接改原意图并重新 fill，前面不要留过期代码。
- 把一个新增文件整体当成一个 change——按调用关系拆成多个 change，各自解释。
- 为了页面顺序调整 `changes` 数组顺序——用 `display_order`，数组顺序是重放语义。
- 修订时改 change 的 `id`（或干脆不写 id）——保持稳定 id，否则更新摘要会误报文件增删。
- 新增函数只写“这里实现了 X”，不写它在调用链里的位置/职责——补齐职责说明 + `references`。
- 多仓时把仓库拼进 `file`（如 `web:src/a.ts`）——用 `repo` + `file`。
- 为跨仓评审创建合成/临时 Git 仓，或把多个仓的路径硬拼成一个——用 `repos[]` 指真实 checkout。

## 输出

最终交付物：

1. `.intent-review/review.filled.json`（canonical，含 lines/行号）
2. `.intent-review/review.html`（`render` 生成，可刷新）
3. 一句话说明：`validate` 的 `ok: true`（或仍存在的 warning 及原因）
4. 本轮调整说明：改了哪些意图，以及是文件顺序 / 函数分组 / 解释内容 / 代码内容（用 `diff`/`watch` 输出）
