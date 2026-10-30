# intent-review

读取 AI agent 按约定格式产出的**意图评审文件**，生成**单文件、离线可打开的代码评审页面**。
工具本身不调用 LLM、不做语义分组：拆解意图与写业务解释由 agent skill 负责，
渲染与链条校验由本工具确定性完成。

- 输入：描述“有序意图链”的 JSON（schema `1.0`）
- 输出：自包含 HTML（内联 CSS/JS/diff/高亮，`file://` 直接打开，无网络请求）
- 核心语义：意图 i 的 diff 相对 **State(i-1)** 计算；上一意图的变更自动成为当前意图的 context

## 快速开始

```bash
go build -o bin/intent-review ./cmd/intent-review

# 骨架（含 patch）→ 结构化 lines + 行号
bin/intent-review fill review.json --repo . -o review.filled.json

# 链条重放校验（结构化错误）
bin/intent-review validate review.filled.json --repo . --json

# 生成单文件 HTML（--repo 时附带校验并把诊断标注到状态机节点）
bin/intent-review render review.filled.json -o review.html --repo .
open review.html   # 或浏览器打开 file:///.../review.html
```

端到端示例（自动构造临时 git 仓库并跑 fill → validate → render）：

```bash
bash examples/demo/run.sh
# 产物：.tmp/demo/out/review.filled.json / validate.json / review.html
```

## 命令

| 命令 | 职责 |
|---|---|
| `validate <file.json> --repo . [--json] [--config F] [--exclude G] [--unexclude G]` | schema + 链条重放 + 最终状态比对；`--json` 输出结构化诊断 |
| `fill <file.json> --repo . [-o out.json]` | patch → `lines`/`old_start`/`new_start`，逐意图推进状态；幂等 |
| `render <file.json> -o review.html [--repo .] [--max-lines N] [--no-highlight] [--config F] [--exclude G] [--unexclude G]` | 生成自包含 HTML；输入仍是 patch 时自动在内存里 fill（需 `--repo`）；`--no-highlight` 关闭高亮 |
| `schema [--output schema.json]` | 输出 JSON Schema（供 agent/编辑器补全） |
| `watch <file.json> -o review.html [...]` | 文件变化自动重渲染 |

退出码：`0` 成功 / `1` 校验失败 / `2` 用法或 IO 错误 / `3` schema major 版本不支持。
人类可读诊断写 **stderr**（行尾带错误码，如 `(CONTEXT_MISMATCH)`），`--json` 写 **stdout**。

flag 与位置参数可任意交错（`fill review.json --repo . -o out.json` 或 `fill --repo . review.json -o out.json`）。

## 文件过滤（排除列表）

`validate`、`render`/`watch` 的附带校验在做 base→head 覆盖检查前会先过滤路径：
命中排除列表的文件**不要求被意图覆盖**。注意它**只影响覆盖检查**——重放与最终状态比对不受影响，
已经声明成 change 的测试文件仍会逐行校验。

内置默认：`*_test.go`、`**/vendor/**`、`**/node_modules/**`。

增删方式（从左到右依次生效：内置默认 → 配置 `exclude` → 配置 `unexclude` → CLI `--exclude` → CLI `--unexclude`；
同一层内删除优先）：配置文件放在仓库根，或用 `--config <path>` 显式指定（此时不再读仓库内配置）。

```json
{ "file_filter": { "exclude": ["docs/**", "*.pb.go"], "unexclude": ["*_test.go"] } }
```

```bash
intent-review validate review.json --repo . --exclude 'docs/**' --unexclude '*_test.go'
```

- `--exclude` / `--unexclude` 均可重复；`unexclude` 按模式**字面值**删除。
- 配置文件名为 `.intent-review.json`；未知字段或非法 JSON 直接报用法错误（退出码 2），不静默忽略。
- 模式语义：不含 `/` 时匹配**文件名**（任意深度，如 `*_test.go`）；含 `/` 时锚定仓库根；
  `**` 跨多级目录（含 0 级）、`*` 不跨 `/`、`?` 单字符、`[abc]`/`[!abc]` 字符集；结尾 `/` 等价于 `<dir>/**`。

## 页面能力

- 左侧链菜单（多链独立重放、按需切换）
- 顶部 **sticky 意图状态机**：任意节点直达，已过/当前/未到三态；点击、`←`/`→`、URL hash、浏览器前进后退均可切换
- 两列增量 diff：左列 State(i-1)、右列 State(i)，行号只给当前状态（不展示 base 行号）
- 每个 change 强制展示业务解释（缺失时红色警告块）
- 语法高亮（chroma，按扩展名选 lexer；未知降级纯文本 + 警告）
- context 折叠（保留紧邻变更的 3 行：前置留末尾 3 行、后置留开头 3 行、中间留两侧各 3 行；超过 6 行才折叠，前端展开）
- 深链：`#<chain-id>/<intent-id>/<change-index>`（change 1 基，与 UI 一致）
- 大评审降级：总 diff 行数超过 `--max-lines`（默认 5000）时每个意图内容用 `<details>` 折叠
- 移动端：左侧菜单变抽屉，状态机仍 sticky

## 数据模型（摘要）

完整定义见 `intent-review schema`（JSON Schema）与设计文档 §4。

```json
{
  "schema_version": "1.0",
  "meta": { "title": "...", "base_commit": "HEAD~1", "head_ref": "HEAD" },
  "chains": [{
    "id": "chain-rate-limit",
    "title": "请求限流",
    "base_commit": "HEAD~1",
    "intents": [{
      "id": "intent-001",
      "title": "引入请求级限流",
      "summary": "在 Handler 入口加令牌桶限流。",
      "changes": [{
        "file": "handler.go",
        "location": "Handler 入口",
        "business_explanation": "限流必须在入口且在下游调用之前。",
        "patch": ["@@ -4,4 +4,9 @@", " ...", "+ ..."],
        "old_start": 4, "new_start": 4,
        "lines": [{"type": "context", "content": "..."}, {"type": "add", "content": "..."}]
      }]
    }]
  }]
}
```

- 行类型：`context`（两侧显示、双列行号 +1）、`del`（仅左）、`add`（仅右）
- `patch` 是 unified diff 的**行数组**（避免 JSON 转义地狱）；fill 后删除该字段
- `renamed_from` / `new_file` / `deleted` / `binary` 均有对应渲染与重放语义
- 未知字段在读取时忽略；`fill` 输出保留未知字段（便于 minor 版本扩展）

## Agent 工作流

完整契约见 [`skills/intent-review/SKILL.md`](skills/intent-review/SKILL.md)，
各 agent 适配见 [`docs/agents.md`](docs/agents.md)。核心循环：

```bash
intent-review fill review.json --repo . -o review.filled.json
intent-review validate review.filled.json --repo . --json
# 按 errors[] 修复 → 重新 fill/validate，直到 ok: true
intent-review render review.filled.json -o review.html
```

agent **只输出 JSON**，不写 HTML；渲染与样式由工具负责。

## 实现约定（对设计文档的补充/偏差）

1. **一个 change 只允许一个 hunk**：同一文件多处修改拆成多个 change（各自带业务解释）。
   fill 遇到多 hunk 直接报 `PATCH_PARSE_ERROR` 并提示拆分。
2. **同意图同文件多 change 顺序推进**：第 k 个 change 的行号相对“应用它时的当前状态”
   （第一个 change 即 State(i-1)）。fill/validate 都会校验 `new_start` 与重放位置一致，
   行号过期会给出 `CONTEXT_MISMATCH` 与提示。
3. **fill 的唯一上下文回退**：patch 的 `old_start` 处不匹配、但该块在当前状态中**唯一出现**时，
   按实际位置回填 `old_start`/`new_start` 并给出 **warning 级** `PATCH_CONTEXT_MISMATCH`；
   出现多次则不猜测，直接失败。
4. **`validate` 拒绝 patch-only 输入**：先 `fill`（保持 fill/validate 职责单一）；
   `render` 则会自动在内存里 fill。
5. **`validate` 额外做全局覆盖检查**：`meta.base_commit..head` 的每个变更路径都必须被至少一条链触及，
   否则报 `FINAL_STATE_MISMATCH`（捕捉“整个文件漏拆”与“未声明的重命名”）。重放已失败时跳过该检查，
   避免级联误报。
6. **`render --repo` 校验失败仍渲染**：诊断打印到 stderr 并上浮到状态机节点（`⚠` / 红色列表），
   退出码仍为 0——页面本身就是用来发现问题的产物。
7. **binary change 不参与重放**：给予 warning 级 `BINARY_CHANGE_SKIPPED`，仅渲染说明块；
   最终状态比对跳过该文件，避免“无法校验”被误报成失败。
8. **`new_start` 由实际位置推导**：fill 忽略 hunk 头里可能过期的 `+start`（其余字段按头解析），
   保证与重放语义一致。
9. **已知盲区**（设计文档 §7.3）：链条重放无法捕获“把后续意图的代码提前放进当前 context”
   这类自洽但语义错误的拆分。第一版靠 skill 约束 + 评审者肉眼发现；阶段快照校验列入后续版本。
10. **文件过滤只作用于覆盖检查**：`internal/filefilter` 的排除列表只让 `base..head` 覆盖检查跳过命中路径；
    重放、最终状态比对、渲染都不受影响（已声明的 change 仍逐行校验）。内置默认
    `*_test.go` / `**/vendor/**` / `**/node_modules/**`；配置文件固定名 `.intent-review.json`，
    在 `--repo` 目录发现（`--config` 显式覆盖，此时不再读仓库内配置），未知字段视为配置错误（退出码 2）。
    需要让内置项重新参与覆盖检查时用 `unexclude`（按字面值删除）。

## 目录结构

```
cmd/intent-review/     CLI 入口（validate/fill/render/schema/watch）
internal/schema        结构体、JSON Schema、版本与结构校验
internal/replay        git base/head 读取、逐意图状态重放（fill 与 validate 共用）
internal/filefilter    覆盖检查的排除列表（内置 *_test.go 等）+ .intent-review.json 读取
internal/diffpatch     unified diff（单 hunk）解析
internal/fill          patch 填充、行号回填、JSON 树合并（保留未知字段）
internal/validate      链条重放校验、最终状态比对、结构化错误
internal/report        错误码/诊断/退出码
internal/render        HTML 模板、两列对齐、诊断上浮（go:embed 内联资源）
internal/highlight     chroma 封装（整列高亮后切行，跨行语法状态正确）
examples/demo          端到端 demo（脚本 + 两提交 fixture + 骨架 JSON）
skills/intent-review   agent skill（流程 / schema / 错误重试 / 反模式）
docs/agents.md         四种 agent 的适配说明
scripts/               verify / 文件行数门槛 / 浏览器 UI 自检
```

## 验证

```bash
make test     # go test ./...
make vet      # go vet ./...
make demo     # 端到端 fill → validate → render
make ui-check # 浏览器自检（需要 playwright；见 scripts/ui-check.mjs）
```

## 已知限制

- 依赖 git 读取 base/head 内容（`--repo` 必须指向包含这些提交的仓库）
- 一行一个 `<tr>`，超大文件（数万行）会有明显 DOM 体积；靠 `--max-lines` 折叠缓解
- 不支持链内分支/合并：并行需求建两条独立链
- 不做评论/导出/GitHub 同步（第一版只读）
