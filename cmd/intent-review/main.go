// Command intent-review 读取 AI agent 产出的意图评审 JSON，
// 生成单文件、离线可打开的代码评审页面。
package main

import (
	"fmt"
	"io"
	"os"
)

// version 由 -ldflags "-X main.version=..." 覆盖。
var version = "0.1.0"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		usage(stderr)
		return 2
	}
	cmd, rest := args[0], args[1:]
	switch cmd {
	case "validate":
		return runValidate(rest, stdout, stderr)
	case "fill":
		return runFill(rest, stdout, stderr)
	case "render":
		return runRender(rest, stdout, stderr)
	case "schema":
		return runSchema(rest, stdout, stderr)
	case "watch":
		return runWatch(rest, stdout, stderr)
	case "version", "-v", "--version":
		fmt.Fprintf(stdout, "intent-review %s\n", version)
		return 0
	case "help", "-h", "--help":
		usage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "未知命令 %q\n\n", cmd)
		usage(stderr)
		return 2
	}
}

func usage(w io.Writer) {
	fmt.Fprint(w, `intent-review — 意图链代码评审生成器

用法:
  intent-review validate <file.json> [--repo .] [--json] [--config F] [--exclude G] [--unexclude G]
  intent-review fill     <file.json> [--repo .] [-o <filled.json>]
  intent-review render   <file.json> -o <review.html> [--repo .] [--max-lines N] [--exclude G]
  intent-review schema   [--output schema.json]
  intent-review watch    <file.json> -o <review.html> [--repo .] [--max-lines N] [--exclude G]
  intent-review version

命令:
  validate  链条重放校验 + schema 校验（--json 输出结构化错误）
  fill      把 patch 转为结构化 lines 与行号；不改语义字段
  render    生成自包含 HTML（--repo 时附带校验并把诊断标注到状态机节点）
  schema    输出 JSON Schema
  watch     文件变化时自动重渲染

退出码: 0 成功 / 1 校验失败 / 2 用法或 IO 错误 / 3 schema 版本不支持

文件过滤（validate/render/watch 的覆盖检查）:
  内置排除 *_test.go、**/vendor/**、**/node_modules/**；
  用 --exclude 追加、--unexclude 删除，或在 repo 目录放 .intent-review.json（--config 指定路径）。
`)
}
