# 里程碑与验收证据

设计基线：[`design.md`](design.md)。实现约定与偏差：[`../README.md`](../README.md)。

验证入口：

```bash
bash scripts/verify.sh    # build + vet + gofmt + go test ./...
make demo                 # 端到端 fill → validate → render（临时 git 仓库）
make ui-check             # 浏览器自检（playwright）
bash scripts/check-file-size.sh
```

## M1 schema + validate（链条重放 + 结构化错误）

| 验收项 | 证据 |
|---|---|
| 数据结构/JSON Schema/版本检查 | `internal/schema`；`intent-review schema` 输出合法 JSON（`TestSchemaJSONValid`） |
| major 不匹配拒绝、minor 忽略未知字段 | `TestCheckVersion`、`TestCLIUnsupportedSchemaVersion`（退出码 3） |
| 重放校验（context/del 逐行比对） | `internal/validate`；`TestRunOK`、`TestRunContextMismatch` |
| 最终状态等于 head | `TestRunFinalStateMismatch`、`TestRunDeclaredRenamePasses` |
| 遗漏变更（整文件漏拆/未声明重命名） | `TestRunUncoveredFile`、`TestRunUndeclaredRenameCaught`（`FINAL_STATE_MISMATCH`） |
| 链间隔离 + 链级 base 覆盖 | `TestRunChainIsolation`、`TestRunChainBaseCommitOverride` |
| 同文件多 change 行号过期 | `TestRunSameFileStaleLineNumber` |
| 缺业务解释 / 重复 id / 非法行类型 | `TestCheckStructureMissingExplanation`、`TestCheckStructureDuplicateIDs`、`TestCheckStructureInvalidLineType` |
| 错误定位（chain/intent/change/file/line/expected/actual/hint） | `TestRunContextMismatch`、`TestFormatHuman`、`TestJSONShape` |
| 人类输出 stderr / JSON stdout、退出码 0/1/2/3 | `TestCLIHumanDiagnostics`、`TestExitCode` |

## M2 fill（patch 解析、逐意图状态重放、行号回填）

| 验收项 | 证据 |
|---|---|
| hunk 解析（单 hunk、忽略文件头、空 context 行、无换行标记） | `internal/diffpatch` 全部测试 |
| 多 hunk / 行数不符报 `PATCH_PARSE_ERROR` | `TestParseRejectsMultipleHunks`、`TestParseRejectsCountMismatch`、`TestRunMultipleHunksRejected` |
| patch → lines/old_start/new_start，并推进状态 | `TestRunFillsLines`、`TestRunNewFileAndDeleted` |
| 幂等（重复 fill 字节一致） | `TestRunIdempotent` |
| 上下文不匹配报 `PATCH_CONTEXT_MISMATCH` | `TestRunPatchContextMismatch` |
| 唯一上下文回退（warning 级，不猜测多义） | `TestRunRelocatesUniqueContext` |
| 保留未知字段、删除 patch | `TestRunPreservesUnknownFields`、`TestRunFillsLines` |
| validate 拒绝 patch-only（提示先 fill） | `TestRunUnfilledPatchHint`、`TestCLIValidateRequiresLines` |
| CLI 全流程（flag 交错、退出码） | `cmd/intent-review`：`TestCLIFillValidateRenderFlow` |

## M3 render 基础（菜单/状态机/两列 diff/sticky/深链）

| 验收项 | 证据 |
|---|---|
| 单 HTML 自包含（无 http/fetch/import、内联 CSS/JS） | `TestBuildProducesSelfContainedHTML`、`TestCLIRender` 断言 |
| 所有链/意图预渲染，只改显隐 | `TestBuildProducesSelfContainedHTML`（inactive chain `hidden`）、`ui-check` |
| 两列 diff 对齐与行号（del/add 配对、短侧补空） | `TestAlignRows`、`TestAlignRowsPureInsert`、`TestAlignRowsNewFile` |
| 业务解释强制渲染（缺失红色警告块） | `TestBuildEscapesUserContent`、模板 `.explain-missing` |
| sticky 状态机 + 滚动常驻 | `ui-check`（`position: sticky` 且滚动后 `top=0`） |
| 点击/方向键/hash/后退切换 | `ui-check`（点击、ArrowRight、深链） |
| 深链 `#chain/intent/change`（change 1 基） | `TestBuildDeepLinkAnchors`、`ui-check` 深链用例 |
| 重命名/新增/删除/二进制标记 | `TestBuildRenameAndBinaryTags` |
| 大链标题/数量/当前意图提示 | `ui-check` + 模板渲染断言 |

## M4 高亮、context 折叠、警告上浮、移动端、大评审降级

| 验收项 | 证据 |
|---|---|
| chroma 高亮；跨行注释状态保持；未知降级纯文本 | `TestLinesGo`、`TestLinesUnknownLexerFallsBack`、`TestBuildHighlightFallbackWarning` |
| 关闭高亮 `--no-highlight` 且不产生误报警告 | `TestBuildDisableHighlight` |
| 保留紧邻变更的 3 行（前置末尾/后置开头/中间两侧）、超过 6 行折叠、前端展开/收起 | `ui-check`（折叠按钮、隐藏行数、展开后按钮文案） |
| 校验诊断上浮到节点（`has-warning` + ⚠ + 列表） | `TestBuildWarningsFloatToIntent` |
| 移动端汉堡抽屉 + 当前节点才展开标签 | `ui-check`（抽屉开合、`node-label` display） |
| 大评审（> `--max-lines`）`<details>` 折叠 + 徽标 | `TestBuildLargeMode`、`TestBuildLargeModeWarning`、`ui-check` |
| 性能（本机实测） | 4100 行 / 100 意图：render 0.38s、峰值 RSS 22MB、HTML 1.8MB；200 文件重放：validate 0.76s |

## M5 Skill 文档、四种 agent 适配、端到端 demo

| 验收项 | 证据 |
|---|---|
| 通用流程 + schema 示例 + 错误重试 + 反模式 | `skills/intent-review/SKILL.md` |
| Claude Code / Codex / Pi / DeepSeek 适配 | `docs/agents.md` |
| 端到端 demo（两提交 fixture + 骨架 JSON） | `examples/demo/run.sh` → `.tmp/demo/out/{review.filled.json,validate.json,review.html}` |
| demo 的 validate `ok: true` | `bash examples/demo/run.sh` 输出 `{"ok": true, "errors": []}` |

## 复现记录（2026-10-29，本机）

```text
$ bash scripts/verify.sh
== go build ==
== go vet ==
== gofmt ==
== go test ==
ok  intent-review/cmd/intent-review
ok  intent-review/internal/{diffpatch,fill,highlight,render,replay,report,schema,validate}
verify: ok

$ go test -race ./...        # 全绿（同上包列表）
$ bash examples/demo/run.sh  # fill ok / validate ok:true / render ok
$ node scripts/ui-check.mjs  # ui-check: ok
$ bash scripts/check-file-size.sh  # ok (limit=300)
```

注：`go build ./...` 也会编译 `examples/demo/files/{base,head}` 两个 fixture 包，
保证 demo 源码本身是合法 Go（head 是完整最终态）。
