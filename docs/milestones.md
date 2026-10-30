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

## M6 文件过滤（排除列表）

| 验收项 | 证据 |
|---|---|
| 内置排除 `*_test.go`（任意深度） | `TestDefaultExcludesTestFilesAtAnyDepth`、`TestRunCoverageSkipsDefaultExcludedTestFile` |
| 内置排除 vendor/node_modules | `TestDefaultExcludesVendorAndNodeModules` |
| 模式语义（basename/锚定/`**`/`?`/字符集/结尾 `/`） | `TestPatternSemantics`、`TestWindowsSeparatorsAreNormalized` |
| 只影响覆盖检查，不放过已声明 change | `TestRunFilterDoesNotSkipReplayOfExcludedFile` |
| 配置文件增删（`.intent-review.json` / `--config`） | `TestLoadExplicitConfig`、`TestLoadDiscoversConfigInDir`、`TestCLIValidateConfigExclude` |
| CLI 重复 `--exclude`、`--unexclude` 恢复内置项 | `TestCLIExcludeFlagRepeatable`、`TestCLIUnexcludeFlagRestoresCoverage` |
| 配置未知字段/非法 JSON → 退出码 2 | `TestLoadRejectsUnknownFieldAndBadJSON`、`TestCLIInvalidConfigFails` |
| render 附带校验同样过滤；无 `--repo` 不读配置 | `TestCLIRenderUsesFilter` |
| 配置覆盖优先级（默认 → 配置 → CLI，删除优先） | `TestResolveAppliesExcludeThenUnexclude`、`TestResolveEmptyConfigKeepsDefaults` |

## M7 跨仓项目、展示顺序、源码跳转、更新反馈

| 验收项 | 证据 |
|---|---|
| 多仓组织为一个项目（repos + change.repo，逐仓 base/head） | `TestRunMultiRepoOK`、`TestCLIMultiRepoFillValidateRender`（跨仓 fill→validate→render） |
| 逐仓覆盖检查（未覆盖文件带 repo 定位） | `TestRunMultiRepoCoverageIsPerRepo`、`TestRunMultiRepoFilterAppliesToAllRepos` |
| 多仓结构校验（id 唯一、必须声明 repo、path 相对、逐仓 base） | `TestCheckStructureMultiRepo`、`TestCheckStructureRepoIDsAndBases`、`TestCheckStructureChainBaseCommits` |
| 文档内统一 file_filter（跨仓共用，本次列表 *.md/*_test.go/*.pb.go/*.test.tsx/go.mod/go.sum） | `TestCLIDocFileFilter`、`TestRunMultiRepoFilterAppliesToAllRepos` |
| 展示顺序与重放顺序分离（display_order / display_group） | `TestBuildOrdersChangesByDisplayOrder`、`TestCheckStructureDisplayOrder` |
| 跨仓仓库标签 + vscode://file 链接 + 独立源码根 | `TestBuildMultiRepoLabelsAndVSCodeLinks`、`TestFileLinkEscapesAndSkipsEmptyRoot`、`TestResolveSourceRootsDefaults/Overrides/Errors`、`TestCLIMultiRepoFillValidateRender` |
| 更新反馈（文件顺序/函数分组/解释内容） | `internal/revdiff` 测试、`TestCLIDiffReportsIntentChanges`、watch 输出 |
| 评审产物独立于业务提交 | README「评审产物不入库」、`.gitignore` 的 `.intent-review/` |

### 复现记录（跨仓实测，2026-11-02）

临时项目根下 web / api 两个仓库；文档 `repos[]` + `file_filter`（本次使用的 6 条规则）；
三个 change 的 `display_order` 为 10/20/30，声明顺序为 page.ts、Helper、Handler：

```text
$ intent-review fill review.json --repo "$P" -o filled.json
fill: ok → filled.json（1 条链，1 个意图，0 个警告）
$ intent-review validate filled.json --repo "$P"
validate: ok（1 条链，1 个意图，排除 8 项）
$ intent-review render filled.json -o review.html --repo "$P" --source-root "web=$P/web"
render: ok → review.html（1 条链，1 个意图，6 行 diff）

页面顺序：page.ts → handler.go(Handler) → handler.go(Helper)
链接：vscode://file/$P/web/page.ts:1、vscode://file/$P/api/handler.go:5、…/handler.go:3
标签：页面入口 / 主流程 / 辅助函数
覆盖：page.test.tsx、handler_test.go、api.pb.go、go.mod 均按过滤跳过

$ intent-review diff filled.json revision.json
意图 [chain-checkout] intent-001 修改（…）: 文件顺序、函数分组、解释内容
```

## M8 统一 build 入口、评审范围面板、稳定标识、分组/引用

| 验收项 | 证据 |
|---|---|
| `build` = fill→validate→render，产物固定、校验失败仍更新页面 | `TestCLIBuildArtifactsScopeAndSummary`、`TestCLIBuildWritesPageOnValidationFailure`、`TestCLIBuildRequiresRepo` |
| 页面展示评审范围（规则 + 跳过文件 + 命中规则） | `TestBuildShowsReviewScope`、`TestBuildHidesScopeWhenNoRules`、`TestRunReportsExcludedPathsAndRules`、`TestMatchReasonReturnsHitPattern` |
| 项目级过滤固化（文档 file_filter 由 build 每次读取） | `TestCLIBuildArtifactsScopeAndSummary`（新增测试文件后仍被排除） |
| 稳定 change id + revdiff 优先按 id 匹配 | `TestCheckStructureChangeIDAndReferences`、`TestCompareUsesStableChangeIDs` |
| 分组标题/分组说明/按调用链解释 | `TestBuildGroupHeadersAndNotes`、`TestBuildOrdersChangesByDisplayOrder` |
| 调用处引用后续实现（change / 分组 / 缺失引用） | `TestBuildResolvesReferences` |
| 源码跳转边界（删除文件、文件不存在、精确行号） | `TestBuildSourceLinksEdgeCases`、`TestFileLinkEscapesAndSkipsEmptyRoot` |
| 更新摘要区分维度并带 change id | `TestCompareUsesStableChangeIDs`、`TestCLIBuildArtifactsScopeAndSummary`、`TestCLIDiffReportsIntentChanges` |
| `clean` 只删产物目录（白名单防误删） | `TestCLIBuildArtifactsScopeAndSummary`（`--out-dir <项目根>` 被拒绝） |

### 复现记录（build 实测，2026-11-02）

```text
$ intent-review build review2.json --repo "$P" --out-dir .intent-review
build: 评审范围：排除 8 条规则，跳过 4 个文件
build: 首次生成，共 1 个意图
build: ok → .intent-review/review.html（1 条链，1 个意图，6 行 diff）
build: 产物已写入 .intent-review（该目录已 gitignore，勿提交）

$ intent-review build review2.json --repo "$P" --out-dir .intent-review   # 只改 ch-002 的解释
build: 本次更新影响 1 个意图
意图 [chain-checkout] intent-001 修改（…）: 解释内容
  变更项: ch-002(解释内容)

$ grep -o 'scope-file">[^<]*' .intent-review/review.html
web:go.mod / web:page.test.tsx / api:api.pb.go / api:handler_test.go
$ grep -o 'ref-link" href="[^"]*">[^<]*' .intent-review/review.html
ref-link" href="#change-c0-i0-ch2">api:handler.go @ Helper 返回值

$ intent-review clean --out-dir .intent-review
clean: 已删除 .intent-review（评审产物不会进入业务提交）
```

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

# 文件过滤实测（临时仓库：改 a.txt + 新增 a_test.go + docs/g.md，只覆盖 a.txt）
$ intent-review validate review.json --repo .            # 仅 docs/g.md 报未覆盖，exit 1（测试文件已排除）
$ intent-review validate review.json --repo . --exclude 'docs/**'   # ok，exit 0
$ intent-review validate review.json --repo . --exclude 'docs/**' --unexclude '*_test.go'  # a_test.go 重新报未覆盖，exit 1
$ printf '{"file_filter":{"exclude":["docs/**"]}}' > .intent-review.json \
  && intent-review validate review.json --repo .          # ok（file filter: 已加载…）
```

注：`go build ./...` 也会编译 `examples/demo/files/{base,head}` 两个 fixture 包，
保证 demo 源码本身是合法 Go（head 是完整最终态）。
