# AGENTS.md — intent-review 开发指南

本文件供 AI 编码代理（Claude Code、Codex、Pi、DeepSeek Harness 等）修改本仓库前阅读。
**读完再看对应文档；改契约前先确认影响面。**

## 项目一句话

`intent-review` 是一个 Go 单二进制工具：读取 AI agent 按约定格式产出的**意图评审 JSON**，
生成**单文件、离线可打开的代码评审页面**；同时提供**链条重放校验**，保证「意图顺序 = 代码状态推进」。
工具**不调用 LLM、不做语义分组**：拆意图与写业务解释由 agent skill 负责，渲染与校验必须确定性。

**本仓库不是 codeintel 的实现仓库**：`codeintel` 是外部工具（另一个仓库的产物），
本仓库的构建/测试不依赖它。可以用它给本仓库建临时索引做**调用链与死代码审计**
（`.codeintel/` 已 gitignore）；但不要引入 codeintel 的文档/脚本约定。

## 文档导航

| 文档 | 什么时候看 |
|---|---|
| [`README.md`](README.md) | 命令用法、页面能力、**实现约定与设计偏差**（改行为前必读） |
| [`docs/design.md`](docs/design.md) | 设计基线 v0.1（§ 编号被代码注释引用） |
| [`docs/milestones.md`](docs/milestones.md) | M1–M5 验收证据与实测数据（改行为后同步更新） |
| [`docs/agents.md`](docs/agents.md) | 四种 agent 的适配与提示词片段 |
| [`skills/intent-review/SKILL.md`](skills/intent-review/SKILL.md) | agent 侧契约：流程、错误码修复表、反模式（面向使用者，改动需谨慎） |
| [`examples/demo/README.md`](examples/demo/README.md) | 端到端 demo 的构造与产物 |

## 架构地图（改代码前先定位）

```
cmd/intent-review/      CLI：validate/fill/render/schema/watch（只做参数解析、调用、输出）
internal/schema/        数据模型、JSON Schema、版本检查、结构校验
internal/replay/        git base/head 读取 + 逐意图状态重放（fill 与 validate 的唯一语义来源）
internal/diffpatch/     unified diff 单 hunk 解析（含 CRLF 保留）
internal/fill/          patch→lines/行号；JSON 树合并（保留未知字段）
internal/validate/      结构校验 + 重放 + 最终状态比对 + base..head 覆盖检查
internal/report/        错误码、诊断结构、人类可读格式、退出码
internal/render/        HTML 模板、两列对齐、诊断上浮（go:embed：web/*.tmpl|css|js）
internal/highlight/     chroma 封装：整列高亮后切行
internal/testutil/      临时 git 仓库夹具（仅测试用）
examples/demo/          端到端 demo（base/head 两提交 fixture + 骨架 JSON + run.sh）
scripts/                verify.sh / check-file-size.sh / ui-check.mjs
```

## 不可破坏的契约（改之前先想清楚）

1. **change = 一个 hunk**。同一文件多处修改拆多个 change；`fill` 遇到多 hunk 必须报
   `PATCH_PARSE_ERROR` 并提示拆分。不要为了"方便"支持多 hunk——会破坏每 change 一条业务解释。
2. **重放语义唯一来源是 `internal/replay`**。fill 与 validate 必须共用 `Engine.Apply`，
   不要各写一套状态推进/行号校验（历史上这类分叉必然产生"fill 通过、validate 失败"）。
3. **行号语义**：`old_start` 相对"应用该 change 时的状态"（第一个 change 即 State(i-1)）；
   同一意图同文件多 change 按数组顺序推进。`new_start` 由实际位置推导并校验，不要信任 hunk 头里的 `+start`。
4. **validate 只吃 canonical（含 lines）**；patch-only 必须报 `SCHEMA_INVALID` 并提示先 `fill`。
   `render` 可以在内存里自动 fill（需要 `--repo`）。别把"自动填充"塞进 validate。
5. **fill 输出必须保留未知字段且删除 `patch`**（`internal/fill/emit.go` 在原始 JSON 树上改）。
   改成 struct 往返会静默丢字段，破坏 minor 版本兼容。
6. **错误码 / JSON 字段 / 退出码是外部契约**（skill 与 agent 依赖）：
   - 退出码：`0` 成功、`1` 校验失败、`2` 用法/IO、`3` schema major 不支持；
   - `--json` 写 stdout；人类可读诊断写 stderr，行尾带 `(CODE)`；
   - `change_index` 在 JSON 里 **0 基**，人类输出显示 `change-<n>`（1 基）；深链 change **1 基**。
   新增/改错误码要同步：`internal/report`、README、`skills/intent-review/SKILL.md` 的修复表、测试。
7. **渲染必须自包含**：不得引入 CDN/外链字体/`fetch`/动态 `import`；CSS/JS/diff/高亮全部内联。
   `scripts/ui-check.mjs` 会监听请求，出现非 `file://`/`data:` 即失败。
8. **转义边界**：用户内容一律走 `html/template`；唯一允许 `template.HTML` 的是
   `highlight` 产出的已转义行。新增渲染字段时不要绕过这一点。
9. **初始可见性**：渲染时只有第一条链的第一个 panel 不带 `hidden`；其余 panel 与链都 hidden。
   切换只改 `hidden`/`aria-current`/状态类，**不重建 DOM**。改模板时同步 `render` 测试与 `ui-check`。
10. **折叠语义**（`internal/render/web/app.js`）：保留**紧邻变更**的 3 行 context
    （前置留末尾 3、后置留开头 3、中间两侧各 3），连续 context > 6 行才折叠；
    改 `KEEP_CONTEXT`/`FOLD_THRESHOLD` 要同步 README 与 ui-check 断言。
11. **大评审降级**：`--max-lines`（默认 5000）超过后每个意图内容用 `<details>` 初始折叠；
    JS 激活意图时必须 `details.open = true`（否则深链/切换看不到内容）。
12. **校验覆盖检查**：`meta.base_commit..head` 的每个变更路径必须被某条链触及
    （捕捉整文件漏拆、未声明重命名）。**重放已出错时不要跑覆盖检查**，避免级联误报。
13. **binary change 不参与重放**（warning `BINARY_CHANGE_SKIPPED`，最终比对跳过该文件）；
    **CRLF 行内容逐字保留**（只对 diff 头去 `\r`，不要剥行内容）；仓库路径必须是相对路径且禁止越界。
14. **高亮降级不阻塞**：无 lexer → 纯文本 + `HIGHLIGHT_FALLBACK` warning；`--no-highlight` 时不得误报。

## 常用命令（提交前必须过）

```bash
# 构建 / 单测 / 校验
go build ./...
go test ./...
go test -race ./...
bash scripts/verify.sh            # build + vet + gofmt + 全量单测（提交前必跑）
bash scripts/check-file-size.sh   # Go 文件 ≤300 行（含测试；超了就按关注点拆）

# 端到端（临时 git 仓库 + fill→validate→render，产物在 .tmp/demo/out/）
make demo        # = bash examples/demo/run.sh
make ui-check    # = build + demo + node scripts/ui-check.mjs（需要 playwright）

# 手工体验
go build -o bin/intent-review ./cmd/intent-review
bin/intent-review fill     examples/demo/review.json --repo .tmp/demo/repo -o /tmp/f.json
bin/intent-review validate /tmp/f.json --repo .tmp/demo/repo --json
bin/intent-review render   /tmp/f.json -o /tmp/review.html --repo .tmp/demo/repo
bin/intent-review schema > /tmp/schema.json
```

- 浏览器自检依赖 playwright：`npm i playwright`（仓库根；`node_modules` 已 gitignore）。
  没有 playwright 时 `ui-check` 会明确退出码 2，不要伪造通过。
- `TMPDIR` 默认无所谓；临时产物一律放 `.tmp/`（已 gitignore），不要写进 `examples/` 或仓库根。

## 可选的代码审计（外部 codeintel）

改符号前、或做「有没有多余实现」审计时，用外部 codeintel 索引本仓库：

```bash
PATH=$PATH:$(go env GOPATH)/bin codeintel reindex --repo . --progress plain
codeintel query unused  --repo .                 # 死代码候选
codeintel query callers '<canonical-id 或名字>'  # 谁在调用（改符号前必看）
codeintel query callees '<canonical-id>'         # 流程链路（CLI→internal）
codeintel query impact  '<canonical-id>'         # 深度 3 影响面
```

**解读 `unused` 的三个已知误报**（本仓库实测，2026-10-29）：

- **实参位置的调用**（`f(g())`、`append(x, g())`、`fmt.Sprintf("...", g())`）被记为
  `passes_result` 而不是 `calls` → `unused` 会误报。已知：`countIntents`、`withRows`、
  `report.FormatHuman`、`report.ShortSHA`、`validate.compareHead`、`existWord`。
  判死代码前先用 `query callers` + `grep` 确认「真的没有调用点」。
- `_test.go` 不入索引 → `internal/testutil` 的 helper 会全部列为未调用（正常）。
- `examples/demo/files/{base,head}` 是 demo fixture，允许存在未被调用的函数；
  `main`/`run` 作为入口没有 caller 也正常。

**审计前先 `reindex` 而不是 `update`**：本仓库实测增量 update 后出现大量
「跳过边」（69 → 507）与更多 unused 误报（`writeOut`/`runRender` 被误判），
全量 reindex 后消失；结论以 reindex 为准。

## 开发流程（按顺序做）

1. **先读契约**：改哪个行为，先看 README「实现约定」+ 相关 § 注释；涉及外部契约的同步 SKILL。
2. **先写测试再改实现**（bug 修复先写能复现失败的测试）。测试按**关注点拆分**：
   `xxx_test.go`（主流程+共享夹具）、`xxx_edge_test.go`/`xxx_coverage_test.go` 等；共享 helper 放在同包主测试文件。
3. **改完跑**：`scripts/verify.sh`（UI 改动再加 `make demo && make ui-check`）。
4. **同步文档证据**：行为/阈值/数字变化时更新 `docs/milestones.md`；新增偏差写进 README「实现约定」。
5. **一个逻辑改动一个提交**，提交信息写清"改了什么 + 为什么"；**不要攒大提交**。
6. 交付时给出可复现命令与实测输出，不要只说"已实现"。

## 改哪里会连带什么（影响面清单）

| 改动位置 | 必须同步 |
|---|---|
| `internal/replay`（状态推进/错误码） | fill + validate 测试、README 约定、SKILL 修复表、`docs/milestones.md` |
| `internal/schema`（字段/JSON Schema/必填） | fill 保留未知字段策略、SKILL 示例、demo 的 `review.json`、schema 测试 |
| `internal/report`（码/字段/格式） | 各命令测试、README 退出码/输出约定、SKILL 表格 |
| `internal/render/web/*`（DOM/类名/数据属性） | `app.js` 选择器、`render_*_test.go` 断言、`scripts/ui-check.mjs` |
| `internal/render`（Options/默认阈值） | CLI flag、README、milestones 数据、ui-check 的 `--max-lines` 用例 |
| `internal/highlight`（降级/切行） | fallback warning 测试、`--no-highlight` 测试、CSS 的 `tok-*` 类 |
| CLI 参数/子命令 | `parseFlagSet`（flag 与位置参数可交错）、help 文本、CLI 测试的退出码 |
| `examples/demo` 的 fixture | **head 必须是可编译的完整包**（`go build ./...` 会编译它），同步 `review.json` 的 patch 行号 |
| `skills/intent-review/SKILL.md` | README 命令/错误码、`docs/agents.md` 的调用模板 |

## 本项目踩过的坑（别重蹈）

- **示例 fixture 会被 `go build ./...` 编译**：`examples/demo/files/{base,head}` 是真实 Go 包；
  head 目录要自带 `rate.go`/`logging.go` 等所有依赖文件，否则 `go build ./...` 直接红。
- **Go 标准 flag 遇到第一个位置参数就停止解析**：本仓库统一用 `cmd/intent-review/cli.go` 的
  `parseFlagSet`（先分离 flag 再解析）。新增子命令不要直接 `fs.Parse(args)`，否则
  `fill review.json --repo .` 会静默拿不到 `--repo`。
- **flag 测试要覆盖两种顺序**（位置参数在前/在后），并断言未知 flag 退出码 2。
- **fixture 行号必须按实际内容数**：demo 的 patch 行号错一个就会 `PATCH_CONTEXT_MISMATCH`；
  改 fixture 内容后重算 hunk 头并重跑 `make demo`。
- **gofmt 对齐会改变 head 内容**：给 `Config` 加字段会重新对齐已有字段；demo 的配置意图
  必须把"对齐变化"写进 patch（`del`+`add`），否则最终状态与 head 差一个空格。
- **`hidden` 要写在严格祖先上**：面板可见性测试要区分"panel 自身 hidden"与"祖先链 hidden"；
  初始态只有 1 个 panel 可交互，切换后旧链的 panel 不应重新出现。
- **sticky 失效先查祖先 overflow**：`.intent-timeline` 的祖先（`.main`/`.section`）不能有
  `overflow: hidden/auto/scroll`；diff 的超宽滚动只放在 `.diff-wrap`。
- **diff 表头不要 sticky**：会和 sticky 状态机重叠（曾被遮挡），表头保持静态。
- **CSS 类名不是渲染输出**：断言"关闭高亮"要查 `<span class="tok-`，不能查 `tok-k`（CSS 里也有）。
- **`git cat-file -e` + `blob` 是每文件两次进程**：200 文件 validate ≈0.76s，可以接受；
  要做大仓优化再改 `--batch`，并用同一组数字对照（不要凭直觉）。
- **`git diff --name-only --no-renames`** 是覆盖检查的口径：重命名会表现为旧路径删除+新路径新增，
  这样才能抓住"未声明的重命名"；别用默认 rename 检测把旧路径藏掉。
- **CRLF**：`diffpatch` 只对头部去 `\r`；行内容保留，否则 CRLF 仓库的 context 永远匹配不上。
- **测试文件超 300 行要拆**（`check-file-size.sh` 会拦）：按关注点拆，共享 helper 放主测试文件；
  拆完用 `goimports -w <pkg>` 修 import（`gofmt` 不删未使用 import）。
- **UI 改动不能只靠单测**：`node --check internal/render/web/app.js` + `make ui-check` 才算过；
  大评审/折叠/移动端都靠 ui-check 的合成页面验证。

## 禁止事项

- 不要为"通过校验"放宽 constraint（例如允许 patch-only、忽略 context 不匹配、跳过覆盖检查）。
- 不要在 `validate`/`fill` 里另写一套重放逻辑；不要在 cli 层写业务判断（cli 只解析/调用/输出）。
- 不要手写/内联第三方的 JS/CSS 大块库，也不要引入 CDN；需要图/高亮就用现有依赖或内联。
- 不要把用户附件目录（`.cc-connect/`）、`.tmp/`、`bin/`、`node_modules` 提交进仓库。
- 不要修改 `docs/design.md` 的正文（它是基线归档）；实现偏差写 README/milestones。
- 不要用 `git checkout -- <file>` / `git restore` 撤销实验（会丢整份未提交改动）；用精确编辑或先 commit。
