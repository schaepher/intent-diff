# intent-review 设计文档（v0.1，实现基线）

> 本文档是设计基线（原样归档）。实现状态与偏差说明见
> [`../README.md`](../README.md)「实现约定」与 [`milestones.md`](milestones.md)。
> 已实现 M1–M5（schema/validate/fill/render/watch/skill/demo）。

---

intent-review 设计文档

版本：0.1（草案）
状态：设计已确认，待实现
定位：读取 AI agent 按约定格式产出的意图评审文件，生成单文件、离线可打开的代码评审页面。工具本身不调用 LLM、不做语义分组。

---

1. 摘要

intent-review 是一个 Go 单二进制工具。它消费一份描述“有序意图链”的 JSON 文件，生成自包含 HTML。评审者在页面上通过左侧菜单切换意图链、通过顶部常驻状态机切换链内意图；每次只看到一个意图引入的增量变更，该意图的变更相对上一意图状态计算，上一意图的代码在当前意图中呈现为已有代码。

核心契约：agent 负责拆解意图并写业务解释，工具负责确定性渲染与链条校验。

---

2. 目标与非目标

目标

· 支持多条完全独立的意图链；链内严格线性，链间互不依赖。
· 每个意图的 diff 相对上一意图状态，而非 base；上一意图的变更在下一意图中自动成为 context。
· 一个意图可跨多文件、同文件多处；每处变更强制携带业务解释。
· 顶部状态机常驻，任意节点可点击直达；切换只改显隐，不重建 DOM。
· 单 HTML 文件，内联全部资源，file:// 直接打开，无网络请求。
· 提供链条重放校验，确保意图顺序与代码状态推进一致。
· 允许 agent 先写语义骨架与 patch，由 CLI 自动填充结构化 diff 与行号。

非目标

· 不做最终态（base → head 净 diff）视图；最终态由 git/PR 页面负责。
· 不做评论、导出、GitHub 同步；第一版只读。
· 不做意图自动拆解；拆解质量由 agent skill 负责。
· 不支持链内分支/合并；需要并行时建两条独立链。

---

3. 核心概念

概念 说明
Chain（意图链） 一条独立的评审单元，对应一个完整需求。链内意图严格线性。多条链通过左侧菜单切换。
Intent（意图） 链内的一个阶段，对应一个连贯的业务变更理由。是评审导航的最小单元。
Change（变更点） 意图内某个文件的一处连续修改。一个意图可含多个 change，可跨文件、可同文件多处。
State(i) 应用链内前 i 个意图后的代码状态。State(0) 为链的 base。
增量 diff 意图 i 的 change 展示 State(i-1) → State(i) 的变化。左列是上一状态，右列是当前状态。

关键语义：意图 i 的 context 行来自 State(i-1)，其中可能包含意图 1..i-1 引入的代码。后续意图的代码不得提前出现在当前意图的 context 或 add 中。

---

4. 数据模型

4.1 顶层结构

```json
{
  "schema_version": "1.0",
  "meta": {
    "title": "限流与日志改造",
    "base_commit": "a1b2c3d",
    "head_ref": "feat/rate-limit-and-logging",
    "generated_by": "claude-code",
    "skill_version": "1.0"
  },
  "chains": [
    {
      "id": "chain-rate-limit",
      "title": "请求限流",
      "base_commit": "a1b2c3d",
      "intents": [ /* Intent[]，严格有序 */ ]
    },
    {
      "id": "chain-audit-log",
      "title": "审计日志",
      "base_commit": "a1b2c3d",
      "intents": [ /* ... */ ]
    }
  ]
}
```

· chains 至少一个；顺序即左侧菜单顺序。
· 每条链可独立指定 base_commit，缺省继承 meta.base_commit。链之间状态隔离，各自从自己的 base 重放。
· schema_version major 不匹配时工具拒绝处理；minor 升级时忽略未知字段。

4.2 Intent

```json
{
  "id": "intent-001",
  "title": "引入请求级限流",
  "summary": "在 Handler 入口加令牌桶限流，防止下游过载。此时还没有日志。",
  "business_rationale": "高峰期下游被击穿，需要在入口主动拦截。",
  "risk_notes": "阈值需与运维确认；未覆盖分布式场景。",
  "changes": [ /* Change[]，按渲染顺序 */ ]
}
```

字段 必填 说明
id 是 链内唯一，用于深链与错误定位
title 是 状态机节点标签
summary 是 意图整体业务解释，渲染在 panel 顶部
business_rationale 否 为什么现在做
risk_notes 否 风险提示
changes 是 至少一个

4.3 Change

```json
{
  "file": "src/gateway/handler.go",
  "location": "Handler 入口（限流逻辑前后）",
  "business_explanation": "日志必须包住限流判断，才能区分被限流和正常通过但下游慢。",
  "old_start": 12,
  "new_start": 12,
  "lines": [
    {"type": "context", "content": "func Handler(w http.ResponseWriter, r *http.Request) {"},
    {"type": "add", "content": "    log.Info(\"request start\")"},
    {"type": "context", "content": "    if err := limiter.Wait(r.Context()); err != nil {"},
    {"type": "add", "content": "        log.Error(\"rate limited\", err)"},
    {"type": "context", "content": "        http.Error(w, err.Error(), 429)"},
    {"type": "context", "content": "        return"},
    {"type": "context", "content": "    }"},
    {"type": "context", "content": "    resp := callDownstream(r)"},
    {"type": "add", "content": "    log.Info(\"request done\")"},
    {"type": "context", "content": "    w.Write(resp)"},
    {"type": "context", "content": "}"}
  ]
}
```

字段 必填 说明
file 是 仓库相对路径
location 是 人类可读的位置描述
business_explanation 是 禁止 null / 空串；必须回答“为什么这里要改”
old_start 填充后必填 在 State(i-1) 中的起始行号
new_start 填充后必填 在 State(i) 中的起始行号
lines 填充后必填 结构化对齐行
patch authoring 可选 unified diff 行数组，fill 后转为 lines 并删除
renamed_from 否 重命名来源路径；保留 rename 语义，不拆成删除+新增
binary 否 二进制文件，不渲染两列，仅展示说明
deleted / new_file 否 删除/新增文件标记，影响行号与渲染

行号语义：old_start 相对 State(i-1)，new_start 相对 State(i)。不展示 base 行号。

同一意图内同一文件多次出现：允许，按 changes 数组顺序渲染，每个 change 独立携带业务解释。

撤销：允许。意图 B 可删除/改写意图 A 引入的代码，表现为 del 行或 add 覆盖，无需特殊标记。

4.4 行类型

type 左列 右列 行号递增
context 显示 显示 左右都 +1
del 显示 空白 左 +1
add 空白 显示 右 +1

---

5. 文件格式与填充流程

5.1 两种工作模式

模式 A：直接 canonical
Agent 手写完整 lines 与行号，直接 validate + render。适合小变更或工具链已能生成结构化行。

模式 B：骨架 + 自动填充（推荐）
Agent 写意图、业务解释、以及每个 change 的 patch（unified diff 行数组），省略 lines / old_start / new_start。运行：

```bash
intent-review fill intent-review.json --repo . -o intent-review.filled.json
```

工具从各链 base 开始逐意图应用 patch，维护内存文件状态，解析 diff 生成结构化 lines 与行号，输出完整 JSON。fill 幂等，可重复运行。

patch 用字符串数组而非单个多行字符串，避免 JSON 转义问题：

```json
"patch": [
  "@@ -12,6 +12,10 @@",
  " func Handler(w http.ResponseWriter, r *http.Request) {",
  "+    log.Info(\"request start\")",
  "     if err := limiter.Wait(r.Context()); err != nil {",
  "+        log.Error(\"rate limited\", err)",
  "         http.Error(w, err.Error(), 429)",
  "         return",
  "     }",
  "     resp := callDownstream(r)",
  "+    log.Info(\"request done\")",
  "     w.Write(resp)",
  " }"
]
```

填充规则：

· 从链 base 读取文件内容，作为 State(0)。
· 按意图顺序、change 顺序应用 patch；每个 patch 的上下文必须与当前状态匹配。
· 生成 lines：patch 中的   → context，- → del，+ → add。
· old_start 取 patch hunk 的旧起始行；new_start 取新起始行。
· 应用后更新内存状态，供下一意图使用。
· 若 change 已有 lines 且无 patch，视为手工模式，跳过填充但仍参与状态推进（用 lines 中的 context/del 推断旧内容，用 context/add 推断新内容）。
· 输出文件删除 patch 字段，保留其余字段。

5.2 校验与渲染的输入要求

· validate 和 render 接受含 lines 的 canonical JSON。
· 若输入仍含 patch 而无 lines，render 自动先执行内存内填充；validate 报错提示先运行 fill（保持职责单一）。

---

6. 工具命令与架构

6.1 命令面

```bash
intent-review validate <file.json> --repo . [--json]
intent-review fill     <file.json> --repo . -o <filled.json>
intent-review render   <file.json> -o <review.html> [--repo .]
intent-review schema   [--output schema.json]
intent-review watch    <file.json> -o <review.html> [--repo .]   # 可选
```

命令 职责
validate 链条重放校验 + schema 校验；结构化错误输出；退出码非 0 表示失败
fill 将 patch 转为 lines 并回填行号；不改语义字段
render 生成自包含 HTML；可选 --repo 时附带完整校验并将警告上浮到状态机节点
schema 输出 JSON Schema，供 agent 与编辑器使用
watch 文件变更自动重渲染，便于本地迭代

6.2 内部模块

```
cmd/            CLI 入口（cobra 或标准 flag）
internal/schema 结构体、JSON Schema、版本检查
internal/fill   patch 解析、状态重放、行号计算
internal/validate 链条重放校验、错误模型
internal/render HTML 模板、两列对齐、语法高亮、内联资源
internal/highlight chroma 封装
web/            模板与静态资源（go:embed）
```

6.3 技术选型

· Go 单二进制，go:embed 内联模板、CSS、JS。
· 语法高亮：alecthomas/chroma，按扩展名选择 lexer，未知语言降级纯文本；高亮结果直接内联为 HTML span。
· Diff 解析：自实现 unified diff 解析器（仅需 hunk 级），不引入重型依赖。
· HTML 生成：html/template，所有用户内容转义。
· JSON Schema：由结构体生成或手写，随二进制分发。

---

7. 校验算法

7.1 链条重放

对每条链独立执行：

1. 读取 base_commit 下所有涉及文件的内容，建立 state 映射。
2. 按 intents 顺序，对每个 change：
   · 从 state[file] 取 old_start 起、长度为 leftLines = context + del 的行序列；
   · 与 change 的 lines 中 context + del 的 content 逐行比较，必须完全相等；
   · 用 context + add 替换该区域，更新 state[file]。
3. 所有意图处理完后，state 中每个文件必须等于 head_ref / head_commit 的内容。

7.2 校验覆盖的错误

错误 检测方式
意图顺序错 重放时 context 不匹配
change 的 before 与上一状态不一致 重放时逐行比较失败
遗漏变更（最终不等于 head） 最终状态比对失败
同文件多 change 顺序错 重放时后续 change 定位失败
重命名未声明 最终文件存在性/内容不匹配
删除/新增文件行号错误 old_lines=0 / new_lines=0 规则检查
缺业务解释 schema required 校验
链内 id 重复 schema 唯一性校验

7.3 已知盲区

链条重放无法捕获“把后续意图的代码提前放进当前意图的 context”这种自洽但语义错误的拆分——因为链条仍可能一致、最终仍等于 head。第一版通过 skill 约束 + 评审者肉眼发现；增强方案（每阶段文件快照逐阶段比对）列入后续版本，不阻塞首版。

7.4 错误输出

人类可读：

```
[chain-rate-limit] intent-002 / change-1 src/gateway/handler.go:12
  期望 context: "    if err := limiter.Wait(r.Context()); err != nil {"
  实际内容:     "    if err := legacyLimiter.Wait(r.Context()); err != nil {"
  提示: 意图 1 的 after 与意图 2 的 before 不一致，检查意图顺序或 change 内容。
```

--json：

```json
{
  "ok": false,
  "errors": [
    {
      "code": "CONTEXT_MISMATCH",
      "chain_id": "chain-rate-limit",
      "intent_id": "intent-002",
      "change_index": 0,
      "file": "src/gateway/handler.go",
      "line": 12,
      "expected": "    if err := limiter.Wait(r.Context()); err != nil {",
      "actual": "    if err := legacyLimiter.Wait(r.Context()); err != nil {",
      "hint": "意图 1 的 after 与意图 2 的 before 不一致，检查意图顺序或 change 内容。"
    }
  ]
}
```

---

8. 渲染设计

8.1 页面布局

```
┌──────────────┬──────────────────────────────────────────────┐
│              │  [状态机行：sticky top:0]                     │
│  左侧菜单     │  (1) 限流 ── (2) 日志 ── (3) 配置            │
│  (链列表)     ├──────────────────────────────────────────────┤
│              │                                              │
│ ● 请求限流    │  Intent 2 / 3  补充请求日志                   │
│   审计日志    │  summary: ...                                │
│              │  rationale: ...                              │
│              │  ─────────────────────────────               │
│              │  src/gateway/handler.go · Handler 入口        │
│              │  ⓘ 业务解释: ...                              │
│              │  ┌──────────────┬──────────────┐             │
│              │  │ 左列 State(1) │ 右列 State(2) │             │
│              │  └──────────────┴──────────────┘             │
│              │                                              │
└──────────────┴──────────────────────────────────────────────┘
```

· 左侧菜单：固定宽度（约 220px），全高，列出所有链的标题与意图数量。点击切换链，更新主区域状态机与内容。
· 顶部状态机：位于主区域顶部，position: sticky; top: 0，纵向滚动常驻。只显示当前链的意图节点。
· 内容区：只显示当前链、当前意图的 panel；其余 hidden。
· 移动端：左侧菜单折叠为汉堡抽屉；状态机仍 sticky；节点标签只展开当前节点。

8.2 状态机行

```html
<nav class="intent-timeline" aria-label="意图状态机">
  <ol class="timeline-track">
    <li class="timeline-node is-passed" data-intent="intent-001">
      <button class="node-btn" title="引入请求级限流">
        <span class="node-index">1</span>
        <span class="node-label">引入请求级限流</span>
      </button>
    </li>
    <li class="timeline-node is-active" data-intent="intent-002">
      <button class="node-btn" aria-current="step" title="补充请求日志">
        <span class="node-index">2</span>
        <span class="node-label">补充请求日志</span>
      </button>
    </li>
    <li class="timeline-node" data-intent="intent-003">
      <button class="node-btn" title="调整配置结构">
        <span class="node-index">3</span>
        <span class="node-label">调整配置结构</span>
      </button>
    </li>
  </ol>
</nav>
```

状态 类 / 属性 视觉
当前 .is-active + aria-current="step" 高亮边框与底色
已过 .is-passed 节点与连接线高亮，表达“已变已有代码”
未到 默认 灰色
警告 .has-warning ⚠ 标记，提示缺业务解释或校验失败

Sticky 实现要点：

· position: sticky; top: 0; z-index: 100，背景不透明。
· overflow-x: auto; overflow-y: hidden，节点多时行内横向滚动。
· .timeline-track { display:flex; min-width:max-content; } 保证不被压缩。
· 祖先元素不得有 overflow: hidden/auto/scroll，否则 sticky 失效。
· 两列 diff 的超宽由 diff 容器自身 overflow-x: auto 解决，页面主体不横向滚动。

8.3 切换行为

· 点击节点、左右方向键、URL hash、浏览器后退均可切换意图。
· 切换只改 hidden 与节点状态类，不重建 DOM。
· 切换链同理：所有链的所有 panel 预渲染，只改显隐；状态机按链重建显示（或预渲染多组状态机并切换）。
· 当前节点自动横向滚入状态机可视区：手动计算 scrollLeft，不用 scrollIntoView，避免纵向跳动。
· 切换后内容区回到状态机行下方，从该意图 summary 开始。
· history.pushState 写入 hash；popstate 恢复。

8.4 深链

格式：#<chain-id>/<intent-id>/<change-index>

· #chain-rate-limit：打开该链第一个意图。
· #chain-rate-limit/intent-002：打开该意图顶部。
· #chain-rate-limit/intent-002/change-3：滚动到该意图第 3 个 change（0 基或 1 基？统一 1 基，与 UI 显示一致）。
· 初始化时解析 hash；无效时回退到第一条链的第一个意图。

8.5 两列 diff 渲染

每个 change 的渲染顺序：

1. change 头部：file · location。
2. 业务解释块（business_explanation），强制渲染；缺失时显示红色警告块（schema 已禁止，但手工文件可能绕过）。
3. 两列 diff 表格。

对齐算法：

```
i = 0
while i < len(lines):
  if lines[i].type == context:
    左列输出 context（行号 old_start + leftCount）
    右列输出 context（行号 new_start + rightCount）
    leftCount++; rightCount++; i++
  else:
    收集连续 del 块 D 和紧随的连续 add 块 A
    按索引配对：max(len(D), len(A)) 行
    每行：左列 D[k] 或空白；右列 A[k] 或空白
    左列行号随 D 递增，右列行号随 A 递增
```

· 连续 del 与后续连续 add 并排；数量不等时短侧补空白。
· 纯 add 左列空白；纯 del 右列空白。
· 行号列窄、等宽字体；不显示 base 行号。
· context 折叠：每个 change 首尾各保留 3 行 context，中间超过阈值（默认 6 行）折叠为“展开 N 行”按钮；展开为纯前端行为。

8.6 语法高亮

· 按文件扩展名选择 chroma lexer，未知降级纯文本。
· 高亮在构建 HTML 时完成，内联为 <span class="...">。
· diff 底色（add 绿、del 红）通过行级 class 叠加，不覆盖语法色。
· 高亮失败不阻塞渲染，降级纯文本并记录警告。

8.7 自包含与性能

· 所有 CSS、JS、diff 数据、高亮结果内联；无 CDN、无 fetch。
· 全量预渲染所有链、所有意图；用 hidden 切换。
· 默认假设单次评审总变更 < 5000 行。超过阈值时：
  · 对每个意图 panel 初始折叠为 <details>，展开时渲染；
  · 状态机行显示“大型评审”提示。
· 阈值可通过 --max-lines 配置。

---

9. Agent Skill 契约

9.1 通用流程（适用于 Codex / Claude Code / Pi / Deepseek Harness）

1. 读取 base 与 head，获取完整 diff 与必要上下文（调用方、测试、配置）。
2. 识别独立需求：完全独立的需求各自建一条 chain；有依赖的变更放同一条链。
3. 链内规划有序意图：每个意图一句话业务目标；边界按“业务理由不同”划分，不按文件或 hunk 划分。
4. 从 base 逐意图重放：维护当前代码状态；每个意图只记录该意图引入的增量 patch（相对上一状态），不记录最终 diff 的片段。
5. 同一连续区域多意图修改必须拆成多个 change，分属不同意图；后一 change 的 patch 自然包含前一意图的结果作为上下文。
6. 每个 change 写 business_explanation：回答“为什么这个位置需要这样改”，不是“这里改了什么”。禁止 null 或空串。
7. 每个意图写 summary：整体业务解释；business_rationale / risk_notes 可选。
8. 输出骨架 JSON（含 patch，可省略 lines/行号）。
9. 运行填充与校验：
   ```bash
   intent-review fill review.json --repo . -o review.filled.json
   intent-review validate review.filled.json --repo . --json
   ```
10. 根据结构化错误修复并重试，直到 ok: true。
11. 渲染：
    ```bash
    intent-review render review.filled.json -o review.html
    ```
12. 只输出 JSON，不输出 HTML；渲染与样式由工具负责。

9.2 各 agent 适配要点

Agent 适配说明
Claude Code Skill 以 SKILL.md 分发，内含上述流程、schema 示例、CLI 调用与错误重试指令；利用其文件读写与 shell 能力。
Codex 同 SKILL.md；在仓库根放置 AGENTS.md 片段指向 skill；强调用 --json 解析校验错误。
Pi 提供 CLI 契约文档与最小示例；若其工具调用有特定格式，在 skill 中给出等价调用模板。
Deepseek Harness 同上；重点保证“生成骨架 → fill → validate → 修复”循环可被其 shell 工具执行。

Skill 不绑定工具实现细节，只依赖三个命令：fill、validate --json、render。

9.3 Skill 必须强调的反模式

· 按文件拆意图（一个文件一个意图）——错误；应按业务理由拆。
· 把最终 diff 按 hunk 分给不同意图——错误；中间态已丢失，必须逐意图重放。
· 在意图 i 的 context 中提前放入意图 j>i 的代码——错误；校验可能通过但语义错误。
· 业务解释写成代码转述（“这里加了 if”）——错误；必须写业务理由。
· 缺业务解释时留空或写 null——禁止；schema 拒绝。

---

10. 错误模型

10.1 错误码

code 含义
SCHEMA_INVALID JSON 结构/类型错误
SCHEMA_VERSION_UNSUPPORTED major 版本不匹配
MISSING_BUSINESS_EXPLANATION 业务解释缺失或为空
DUPLICATE_INTENT_ID 链内意图 id 重复
DUPLICATE_CHAIN_ID 链 id 重复
PATCH_PARSE_ERROR patch 格式错误
PATCH_CONTEXT_MISMATCH 应用 patch 时上下文不匹配
CONTEXT_MISMATCH 重放校验时 context/del 与状态不一致
FINAL_STATE_MISMATCH 最终状态不等于 head
FILE_NOT_FOUND 仓库中找不到文件
RENAME_MISMATCH 重命名声明与实际不符
HIGHLIGHT_FALLBACK 警告级：语法高亮降级

10.2 输出约定

· 人类可读输出到 stderr；--json 输出到 stdout。
· 退出码：0 成功；1 校验失败；2 用法/IO 错误；3 schema 版本不支持。
· 每条错误包含 chain、intent、change、文件、行号、期望、实际、提示，便于 agent 精确修复。

---

11. 分发与兼容

· 分发：Go 单二进制，支持 linux/darwin/windows amd64/arm64；go install 或 release 附件。
· schema 版本："1.0"；工具拒绝不认识的 major；minor 升级忽略未知字段。
· 模板与资源：go:embed 打包，二进制自包含。
· HTML 兼容：现代浏览器；file:// 下无模块加载、无 fetch，所有 JS 内联为传统脚本。

---

12. 里程碑

里程碑 内容 验收
M1 schema 结构体、schema 命令、validate 链条重放、结构化错误 手工构造正确/错误样例，错误定位准确
M2 fill：patch 解析、逐意图状态重放、行号回填 骨架 JSON → 完整 JSON，幂等；错误 patch 给出上下文不匹配
M3 render 基础：左侧链菜单、顶部状态机、两列 diff、业务解释块、sticky、深链 单 HTML 离线打开；切换链/意图正常；深链可用
M4 语法高亮、context 折叠、警告上浮、移动端适配、性能降级 高亮正确降级；大评审不卡死
M5 Skill 文档、四种 agent 适配示例、端到端 demo 仓库 任一 agent 按 skill 产出可校验、可渲染的文件

---

13. 风险与开放问题

风险 影响 缓解
Agent 写 patch 上下文不足，fill 定位失败 填充失败 错误提示要求扩大上下文；skill 要求每个 hunk 至少 3 行上下文
同意图同文件多 change 顺序依赖 重放顺序错 按数组顺序应用；错误提示指出前后依赖
多链共享文件时状态隔离误解 评审者困惑 文档明确链间独立；同一文件的不同需求应分链，各自从 base 重放
链条重放无法捕获“提前泄露后续代码” 语义错拆漏检 skill 强约束；后续版本加阶段快照校验
超大评审全量预渲染内存/时间 页面卡顿 阈值降级 <details>；--max-lines 可配
语法高亮 lexer 缺失 可读性下降 降级纯文本 + 警告，不阻塞
四种 agent 的 shell/工具调用差异 skill 适配成本 skill 只依赖三个 CLI 命令与 --json 错误；各 agent 给最小调用模板

开放问题（不阻塞首版）：

· 是否在后续版本引入每阶段文件快照校验，以捕获“提前泄露后续代码”。
· 是否需要 watch 模式之外的本地预览服务器（当前 file:// 已满足）。
· 是否需要 change 级“复制锚点链接”按钮（当前深链已支持，按钮可后续加）。

---

文档结束。 下一步按 M1 → M5 推进；M1/M2 可并行于 skill 草案编写，M3 依赖 M1 的 schema 与 M2 的填充输出。