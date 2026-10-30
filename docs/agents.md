# 各 agent 适配说明

统一契约见 [`../skills/intent-review/SKILL.md`](../skills/intent-review/SKILL.md)。
所有 agent 只依赖三个命令与结构化错误：

```bash
# 统一入口：fill → validate → render（产物固定 .intent-review/review.html，页面可直接刷新）
intent-review build review.json --repo . --source-root .
# 更新摘要：build/watch 会自动打印；要对比任意两版材料用 diff
intent-review diff <旧>.filled.json <新>.filled.json --json
# 清理评审产物（只删产物目录）
intent-review clean
```

需要单步排查时仍可单独执行：

```bash
intent-review fill     review.json --repo . -o .intent-review/review.filled.json
intent-review validate .intent-review/review.filled.json --repo . --json
intent-review render   .intent-review/review.filled.json -o .intent-review/review.html --repo . --source-root .
```

`intent-review` 可替换为 `go run ./cmd/intent-review`（在本仓库内）或已安装的二进制路径。
agent **只写 JSON**，不写 HTML。

跨仓需求：`--repo` 指向**项目根**（文档 `repos[].path` 相对它解析），一条链可跨仓；
渲染时用 `--source-root <repo-id>=<path>` 指定各仓真实源码根（默认用仓库 checkout，
不会跳到重放快照）。单仓时 `--source-root <path>`。

变更要写稳定 `id`（修订时不变），调用方用 `references` 链接到后面的实现分组；页面会展示
本次生效的排除规则与被跳过的文件（`validate --json` 的 `exclude_rules`/`excluded[]`）。

覆盖检查默认排除 `*_test.go` / `**/vendor/**` / `**/node_modules/**`，测试文件不必写意图；
目标仓库需要不同的评审范围时，让 agent 在 `validate`/`render` 上加 `--exclude <glob>` /
`--unexclude <glob>`（或在仓库根放 `.intent-review.json`），不要把测试文件硬塞进意图链。

## 通用前置

- 目标仓库根执行（`--repo` 缺省为当前目录）。
- 先把基线索引/事实准备好：`git log/diff/show` 可读，`base_commit`/`head_ref` 能被
  `git rev-parse` 解析。
- 想确认 schema：`intent-review schema > /tmp/intent-review-schema.json`。

## Claude Code

1. 把 `skills/intent-review/SKILL.md` 放到 `~/.claude/skills/intent-review/SKILL.md`
   （或目标仓库 `.claude/skills/intent-review/SKILL.md`），frontmatter 的 `name`
   与目录名一致。
2. 在 `CLAUDE.md`（或目标仓库 `AGENTS.md`）加一行：
   `代码评审产物用 intent-review skill：先产出意图链 JSON，再 fill/validate/render。`
3. skill 内直接使用 shell 工具执行三命令；校验失败时只读 `--json` 的 `errors[]` 修复。
4. 允许的工具最小集：`Read`、`Bash`（git / intent-review）、`Write`（只写 review JSON）。

## Codex

1. Codex 读仓库根的 `AGENTS.md`。在其中加入下面的片段（指向技能文档）：

   ```md
   ## 意图评审产物
   - 需要产出代码评审页时，遵循 docs/intent-review-skill.md（从本工具仓库复制 SKILL.md）。
   - 只写 review.json；渲染用：intent-review fill/validate --json/render。
   - 每个 change 必须有 business_explanation；一个 change 只含一个 hunk。
   ```

2. 若要仓库自包含，把 `SKILL.md` 复制到目标仓库 `docs/intent-review-skill.md`。
3. 强调用 `--json` 解析错误（Codex 的 shell 输出解析能力较强，但仍应只依赖结构化字段）。

## Pi

1. Pi 的技能目录为 `~/.agents/skills/<name>/SKILL.md`：复制
   `skills/intent-review/SKILL.md` 到 `~/.agents/skills/intent-review/SKILL.md`。
2. 在目标仓库 `AGENTS.md`（或 Pi 的 project context）加同样的片段。
3. Pi 的 bash 工具用等价调用模板：

   ```bash
   intent-review fill review.json --repo "$PWD" -o review.filled.json && \
   intent-review validate review.filled.json --repo "$PWD" --json
   ```

   从 stdout 读 JSON，从 stderr 读人类可读摘要（不要混用）。

## Deepseek Harness

1. 无技能目录约定时，把 `SKILL.md` 放到仓库 `docs/intent-review-skill.md`，并在
   harness 的 system prompt 中引用该文件。
2. 保证 shell 工具可执行三条命令；如果 harness 不支持 `&&` 串联，拆成三次调用并
   检查退出码：`0` 成功、`1` 校验失败、`2` 用法/IO、`3` 版本不支持。
3. 重试循环模板：

   ```text
   loop (最多 5 次):
     run fill → 若非 0 且不是校验错误：停止并报告 stderr
     run validate --json → 解析 errors[]
     if ok: break
     依据 errors[].code / file / line / expected / actual 修改 review.json
   then render
   ```

## 提示词片段（可复用）

```text
你在做“意图链代码评审”。按以下约束产出 review.json：
1) 每条独立需求一个 chain；链内意图严格线性，意图边界按业务理由划分。
2) 意图 i 的 patch 相对 State(i-1)；上一意图的代码是当前意图的 context。
3) 一个 change 一个 hunk；同文件多处修改拆多个 change，每个 change 写 business_explanation（为什么改）。
4) 不得提前放入后续意图的代码；不得按文件或 hunk 机械拆意图。
5) 只输出 JSON。随后运行：
   intent-review fill review.json --repo . -o review.filled.json
   intent-review validate review.filled.json --repo . --json
   按 errors[] 修复并重试直到 ok:true，再 render。
```
