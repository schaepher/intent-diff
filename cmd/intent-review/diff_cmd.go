package main

import (
	"flag"
	"fmt"
	"io"

	"intent-review/internal/revdiff"
	"intent-review/internal/schema"
)

func runDiff(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("diff", flag.ContinueOnError)
	fs.SetOutput(stderr)
	asJSON := fs.Bool("json", false, "结构化变更报告输出到 stdout")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "用法: intent-review diff <before.json> <after.json> [--json]")
		fs.PrintDefaults()
	}
	if err := parseFlagSet(fs, args); err != nil {
		return 2
	}
	if fs.NArg() != 2 {
		fs.Usage()
		return 2
	}
	before, _, err := schema.Load(fs.Arg(0))
	if err != nil {
		fmt.Fprintf(stderr, "读取 %s 失败: %v\n", fs.Arg(0), err)
		return 2
	}
	after, _, err := schema.Load(fs.Arg(1))
	if err != nil {
		fmt.Fprintf(stderr, "读取 %s 失败: %v\n", fs.Arg(1), err)
		return 2
	}
	rep := revdiff.Compare(before, after)
	if *asJSON {
		raw, err := rep.JSON()
		if err != nil {
			fmt.Fprintf(stderr, "序列化变更报告失败: %v\n", err)
			return 2
		}
		fmt.Fprintln(stdout, string(raw))
		return 0
	}
	fmt.Fprintln(stdout, rep.FormatHuman())
	return 0
}
