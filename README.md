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
| `render <file.json> -o review.html [--repo .] [--max-lines N] [--no-highlight] [--config F] [--exclude G] [--unexclude G] [--source-root P]` | 生成自包含 HTML；输入仍是 patch 时自动在内存里 fill（需 `--repo`）；`--no-highlight` 关闭高亮 |
| `schema [--output schema.json]` | 输出 JSON Schema（供 agent/编辑器补全） |
| `build <file.json> --repo <项目根> [--out-dir .intent-review]` | **统一生成入口**：fill → validate → render，产物固定、页面可刷新，并给出评审范围与更新摘要 |
| `watch <file.json> -o review.html [...]` | 文件变化自动重渲染，并说明本次更新影响了哪些意图 |
| `diff <before.json> <after.json> [--json]` | 比较两版材料：意图增删/文件顺序/函数分组/解释内容 |
| `clean [--out-dir .intent-review]` | 删除评审产物目录（只清理产物，不碰业务代码） |

退出码：`0` 成功 / `1` 校验失败 / `2` 用法或 IO 错误 / `3` schema major 版本不支持。
人类可读诊断写 **stderr**（行尾带错误码，如 `(CONTEXT_MISMATCH)`），`--json` 写 **stdout**。

flag 与位置参数可任意交错（`fill review.json --repo . -o out.json` 或 `fill --repo . review.json -o out.json`）。

## 跨仓评审（多仓项目）

同一需求涉及多个仓库时，用 `repos[]` 把它们组织成一个项目；一条链的意图可以跨仓，
`change.repo` 指明所属仓库。`--repo` 是**项目根**（`repos[].path` 相对它解析）。

```json
{
  "schema_version": "1.1",
  "meta": { "title": "结算流程改造" },
  "repos": [
    { "id": "web", "title": "前端", "path": "web", "base_commit": "…", "head_ref": "…" },
    { "id": "api", "title": "后端", "path": "api", "base_commit": "…", "head_ref": "…" }
  ],
  "chains": [{
    "id": "chain-checkout", "title": "结算流程",
    "intents": [{
      "id": "intent-001", "title": "…", "summary": "…",
      "changes": [
        { "repo": "web", "file": "src/page.ts", "location": "入口常量", "business_explanation": "…", "patch": ["…"] },
        { "repo": "api", "file": "handler.go", "location": "Handler", "business_explanation": "…", "patch": ["…"] }
      ]
    }]
  }]
}
```

- 每个仓库独立重放、独立与自己的 head 比对、独立做 base..head 覆盖检查。
- 链级逐仓覆盖：`chain.base_commits: {"web": "…"}`；`chain.base_commit` 作为多仓共用缺省。
- 单仓文档（没有 `repos`）行为不变：`--repo` 指向仓库本身，`change.repo` 必须省略。
- 仓库 `path` 用**相对项目根**的路径（允许 `../other`），便于 JSON 跨机器共享；不要写绝对路径。
- **不要**为跨仓评审创建合成 Git 仓或临时拼接仓库名：直接用 `repos[]` + `change.repo`，
  来源、base/head 与校验结果都来自真实仓库。

## 展示顺序与分组（由表及里）

patch 重放顺序 = `changes` 数组顺序（校验语义）；页面展示顺序可以不同：

```json
{
  "id": "ch-002",
  "repo": "web", "file": "src/page.ts",
  "display_order": 20, "display_group": "主流程",
  "display_group_note": "Handler 是调用方；Helper 实现在后面的“辅助函数”分组展开。",
  "references": ["intent-001/ch-005"]
}
```

- `display_order` 数值小的先展示；未设置的排在已设置之后（保持声明顺序）；同值按声明顺序。
- `display_group` 渲染成分组标题（同一分组的 change 连续展示），`display_group_note` 写整组说明
  （同组取首个非空值）：有调用关系的函数一起解释，独立函数各自分组/单独解释。
- `references` 生成“继续阅读”链接：`<intent-id>/<change-id 或 1 基下标>` 指向 change，
  `<intent-id>#<display_group>` 指向分组；目标不存在时退化为纯文本（不编造跳转）。
- 深链与校验错误里的 change 下标仍按**声明顺序**（1 基），与展示顺序无关。

## VS Code 跳转

渲染时每个 change 的文件路径是 `vscode://file/<绝对路径>:<行号>` 链接
（行号取 `new_start`，删除文件取 `old_start`）。

- 源码根默认 = 仓库 checkout（项目根 + `repos[].path`；单仓 = `--repo`），**不会**指向重放快照。
- 覆盖方式：`repos[].source_root`，或 CLI `--source-root <path>`（单仓）、
  `--source-root <repo-id>=<path>`（多仓，可重复）。
- 没有源码根信息时（如 `render` 不带 `--repo`）不生成链接，避免误跳。
- 特殊字符（空格/`#`/`?`/中文）按 URL 规则转义；删除文件**不生成**链接（本地已不存在）；
  本地缺文件保留链接但标记 `is-missing` 并提示“本地源码中未找到该文件”。
- 行号定位：文件路径跳到该 change 的起始行（`new_start`）；右列每个行号可点，跳到精确行；
  左列是 State(i-1) 的历史内容，**故意不加**链接，避免误认成本地当前文件。

## 文件过滤（排除列表）

`validate`、`render`/`watch` 的附带校验在做 base→head 覆盖检查前会先过滤路径：
命中排除列表的文件**不要求被意图覆盖**。它**只影响覆盖检查**——重放与最终状态比对不受影响，
已经声明成 change 的测试文件仍会逐行校验；源码不会被删除或改写。

内置默认：`*_test.go`、`**/vendor/**`、`**/node_modules/**`。

统一配置可以写在文档 `file_filter`（跨仓共用）或项目根的 `.intent-review.json`（`--config` 显式指定），
CLI 再加临时项。生效顺序（同层删除优先）：

```text
内置默认 → 文档 file_filter → 配置文件 → --exclude / --unexclude
```

```json
{
  "file_filter": {
    "exclude": ["*.md", "*_test.go", "*.pb.go", "*.test.tsx", "go.mod", "go.sum"]
  }
}
```

```json
// .intent-review.json（与文档 file_filter 同构）
{ "file_filter": { "exclude": ["docs/**"], "unexclude": ["**/vendor/**"] } }
```

```bash
intent-review validate review.json --repo . --exclude 'docs/**' --unexclude '*_test.go'
```

- `validate --json` 会带 `exclude_rules` 与 `excluded[]`（文件 + 命中的规则）；页面侧栏
  “评审范围”面板展示同一份信息，更新后测试/文档/依赖文件不会再悄悄回到评审里。
- `--exclude` / `--unexclude` 均可重复；`unexclude` 按模式**字面值**删除。
- 过滤配置文件未知字段或非法 JSON 直接报用法错误（退出码 2），不静默忽略。
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
- 跨仓：文件路径带仓库标签，可点击用 `vscode://file/…` 打开真实源码
- 展示分组：`display_group` 分组标题 + 分组说明（`display_group_note`），`display_order` 控制阅读顺序（不影响重放与深链）
- 评审范围面板：列出本次生效的排除规则与被跳过的文件，避免更新后测试/文档/依赖文件重新进入评审
- “继续阅读”引用：`references` 从调用处跳到后面的实现 change 或分组
- 源码定位：文件路径与右列行号均可点击，`vscode://file/…` 定位到本地当前文件（非历史快照）

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

- 多仓：顶层 `repos[]`（`id/title/path/base_commit/head_ref/source_root`）+ `change.repo`；
  链可跨仓串联，逐仓 base 覆盖用 `chain.base_commits`
- 稳定标识：`change.id`（修订时保持不变，供 diff/引用定位；意图内唯一）
- 展示：`display_order`（页面顺序，不影响重放）、`display_group` + `display_group_note`（分组与说明）、
  `references`（指向后续实现：`<intent-id>/<change-id>` 或 `<intent-id>#<display_group>`）
- 统一过滤：顶层 `file_filter.exclude` / `file_filter.unexclude`
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

## 更新与反馈

统一入口（一次完成 fill → validate → render，产物路径固定，浏览器刷新即可）：

```bash
intent-review build review.json --repo .            # → .intent-review/review.filled.json + review.html
```

- 输出示例：`build: 评审范围：排除 8 条规则，跳过 4 个文件` /
  `build: 本次更新影响 1 个意图 … 变更项: ch-002(解释内容)` / `build: ok → .intent-review/review.html`。
- 校验失败也会更新页面（诊断上浮到状态机节点），退出码保持 `1`；填充失败则保留上一版产物。
- `watch` 每次变化自动重跑并打印同样的影响摘要；要对比任意两版材料用
  `intent-review diff before.json after.json [--json]`。
- 需要单独跑其中一步时，`fill` / `validate` / `render` 仍然可用。

## 评审产物不入库

- `build` 默认写到 `.intent-review/`（`review.filled.json` / `review.html`），该目录已被 `.gitignore` 忽略；
  历史版本也放这里（如 `.intent-review/history/`）。
- `intent-review clean`（或 `clean --out-dir X`）删除产物目录；只接受默认产物目录名或含
  `review.filled.json`/`review.html` 的目录，防止误删业务目录。
- 业务代码照常 commit / push / 部署；评审产物**不默认**纳入提交。
- 需要分享时单独传自包含 HTML，或把材料放到独立仓库；不要把产物加进业务仓库。

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
5. **`validate` 额外做全局覆盖检查**：每个仓库 `base..head`（逐仓 base/head；单仓即 `meta`）的每个变更路径都必须被至少一条链触及，
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
    `*_test.go` / `**/vendor/**` / `**/node_modules/**`；配置来自文档 `file_filter` 与项目根
    `.intent-review.json`（`--config` 显式覆盖），未知字段视为配置错误（退出码 2）。
    需要让内置项重新参与覆盖检查时用 `unexclude`（按字面值删除）。
11. **跨仓重放按仓库路由**：`repos[]` 非空时每个仓库一个 `replay.Engine`，`change.repo` 路由；
    `changes` 数组仍严格线性（跨仓也按数组顺序推进），单仓文档仓库 id 为空串。诊断用 `repo` 字段区分仓库。
12. **展示顺序与重放顺序分离**：`display_order` / `display_group` 只影响渲染，禁止用来改重放路由；
    深链、`change_index` 仍是声明下标（1 基）。
13. **源码链接不猜**：默认用仓库 checkout 路径生成 `vscode://file/…`；无源码根（含 `render` 无 `--repo`）时
    不生成链接。`--source-root` 覆盖优先级最高，其次 `repos[].source_root`。
14. **更新反馈只读**：`diff` / `watch` 的变更说明只比较文档（title/summary/files/order/group/explanation/diff），
    不触发重放、不改变退出码。
15. **评审产物与业务提交分离**：`build` 默认写 `.intent-review/`（gitignore），`clean` 只删产物目录
    （白名单：目录名为 `.intent-review` 或含 `review.filled.json`/`review.html`）。
16. **`build` 是唯一生成入口**：内部依次 `fill → validate → render`，产物固定为
    `<out-dir>/review.filled.json` 与 `<out-dir>/review.html`；校验失败仍更新页面（退出码 1），
    填充失败不覆盖上一版产物。
17. **稳定 change 标识**：`change.id` 可选但**修订时必须保持不变**（意图内唯一，重复报
    `DUPLICATE_CHANGE_ID`）；`revdiff`/`diff` 优先按 id 匹配，否则退回 `(repo,file,location)`。
    页面 `data-change-id` 暴露该标识，`references` 可用它跳转。
18. **页面评审范围**：`validate` 回传 `exclude_rules` 与 `excluded`（含命中规则），`render`/`build`
    在侧栏展示；删除文件不生成源码链接，本地缺文件保留链接但标记 `is-missing` 与说明；
    右列行号带精确行跳转，左列（历史状态）不加链接。

## 目录结构

```
cmd/intent-review/     CLI 入口（validate/fill/render/schema/watch）
internal/schema        结构体、JSON Schema、版本与结构校验
internal/replay        git base/head 读取、多仓引擎路由、逐意图状态重放（fill 与 validate 共用）
internal/filefilter    覆盖检查的排除列表（内置 *_test.go 等）+ .intent-review.json 读取
internal/diffpatch     unified diff（单 hunk）解析
internal/fill          patch 填充、行号回填、JSON 树合并（保留未知字段）
internal/validate      链条重放校验、最终状态比对、结构化错误
internal/report        错误码/诊断/退出码
internal/render        HTML 模板、两列对齐、诊断上浮（go:embed 内联资源）
internal/highlight     chroma 封装（整列高亮后切行，跨行语法状态正确）
internal/revdiff       两版材料的意图级差异（文件顺序/函数分组/解释内容）
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
- 跨仓只支持本地 checkout + git（`repos[].path` 相对项目根；不负责 clone/fetch）
- `watch` 只监听输入 JSON：改配置、源码或 `--repo` 内提交后需手动重跑
- 不做评论/导出/GitHub 同步（第一版只读）
