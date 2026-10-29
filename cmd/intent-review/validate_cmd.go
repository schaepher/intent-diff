package main

import (
	"flag"
	"fmt"
	"io"

	"intent-review/internal/report"
	"intent-review/internal/validate"
)

func runValidate(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("validate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	repoDir := fs.String("repo", ".", "仓库目录（读取 base/head 提交内容）")
	asJSON := fs.Bool("json", false, "结构化错误输出到 stdout")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "用法: intent-review validate <file.json> [--repo .] [--json]")
		fs.PrintDefaults()
	}
	if err := parseFlagSet(fs, args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return 2
	}
	doc, _, code := loadDoc(fs.Arg(0), stderr)
	if code != 0 {
		return code
	}
	res, err := validate.Run(doc, *repoDir)
	if err != nil {
		fmt.Fprintf(stderr, "校验无法执行: %v\n", err)
		return 2
	}
	printDiagnostics(res, *asJSON, stdout, stderr)
	if !*asJSON {
		if res.OK {
			fmt.Fprintf(stderr, "validate: ok（%d 条链，%d 个意图）\n", len(doc.Chains), countIntents(doc))
		} else {
			fmt.Fprintf(stderr, "validate: 失败（%d 个错误）\n", len(res.Errors))
		}
	}
	return report.ExitCode(res)
}
