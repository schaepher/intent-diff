package main

import (
	"flag"
	"fmt"
	"io"

	"intent-review/internal/fill"
	"intent-review/internal/report"
)

func runFill(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("fill", flag.ContinueOnError)
	fs.SetOutput(stderr)
	repoDir := fs.String("repo", ".", "仓库目录（读取 base 提交内容）")
	outPath := fs.String("o", "", "输出文件（缺省写 stdout）")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "用法: intent-review fill <file.json> [--repo .] [-o <filled.json>]")
		fs.PrintDefaults()
	}
	if err := parseFlagSet(fs, args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return 2
	}
	doc, raw, code := loadDoc(fs.Arg(0), stderr)
	if code != 0 {
		return code
	}
	out, res, err := fill.Run(doc, raw, *repoDir)
	if err != nil {
		fmt.Fprintf(stderr, "填充无法执行: %v\n", err)
		return 2
	}
	printDiagnostics(res, false, stdout, stderr)
	if !res.OK {
		fmt.Fprintf(stderr, "fill: 失败（%d 个错误），未写出文件\n", len(res.Errors))
		return report.ExitCode(res)
	}
	if err := writeOut(*outPath, out, stdout); err != nil {
		fmt.Fprintf(stderr, "写出失败: %v\n", err)
		return 2
	}
	target := *outPath
	if target == "" {
		target = "stdout"
	}
	fmt.Fprintf(stderr, "fill: ok → %s（%d 条链，%d 个意图，%d 个警告）\n",
		target, len(doc.Chains), countIntents(doc), len(res.Warnings))
	return 0
}
