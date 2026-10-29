package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"intent-review/internal/fill"
	"intent-review/internal/render"
	"intent-review/internal/report"
	"intent-review/internal/schema"
	"intent-review/internal/validate"
)

func runRender(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("render", flag.ContinueOnError)
	fs.SetOutput(stderr)
	repoDir := fs.String("repo", "", "仓库目录（附带校验并把诊断标注到状态机节点）")
	outPath := fs.String("o", "", "输出 HTML 文件（必填）")
	maxLines := fs.Int("max-lines", render.DefaultMaxLines, "大型评审降级阈值（总变更行数）")
	noHighlight := fs.Bool("no-highlight", false, "关闭语法高亮（调试用）")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "用法: intent-review render <file.json> -o <review.html> [--repo .] [--max-lines N]")
		fs.PrintDefaults()
	}
	if err := parseFlagSet(fs, args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return 2
	}
	if *outPath == "" {
		fmt.Fprintln(stderr, "render 需要 -o <review.html>")
		fs.Usage()
		return 2
	}
	return renderFile(fs.Arg(0), *outPath, *repoDir, *maxLines, *noHighlight, stdout, stderr)
}

// isValidateWarning 判断警告是否已经由 validate 打印过（避免重复）。
func isValidateWarning(printed []report.Error, w report.Error) bool {
	for _, p := range printed {
		if p.Code == w.Code && p.File == w.File && p.IntentID == w.IntentID && p.Message == w.Message {
			return true
		}
	}
	return false
}

// renderFile 是 render/watch 共用的渲染流程（含按需 fill 与校验）。
func renderFile(input, outPath, repoDir string, maxLines int, noHighlight bool, stdout, stderr io.Writer) int {
	doc, raw, code := loadDoc(input, stderr)
	if code != 0 {
		return code
	}
	// 设计文档 §11：major 不匹配时拒绝处理（fill/validate 经 CheckStructure，
	// render 没有结构校验，必须显式检查版本）。
	if e := schema.CheckVersion(doc.SchemaVersion); e != nil {
		res := report.NewResult([]report.Error{*e})
		printDiagnostics(res, false, stdout, stderr)
		return report.ExitCode(res)
	}
	if hasPatchOnly(doc) {
		if repoDir == "" {
			fmt.Fprintln(stderr, "输入仍含 patch 且没有 lines；渲染前需要 --repo 自动填充")
			return 2
		}
		filled, res, err := fill.Run(doc, raw, repoDir)
		if err != nil {
			fmt.Fprintf(stderr, "填充无法执行: %v\n", err)
			return 2
		}
		if !res.OK {
			printDiagnostics(res, false, stdout, stderr)
			fmt.Fprintf(stderr, "render: 填充失败（%d 个错误）\n", len(res.Errors))
			return report.ExitCode(res)
		}
		if doc, err = decodeFilled(filled); err != nil {
			fmt.Fprintf(stderr, "%v\n", err)
			return 2
		}
	}

	opts := render.Options{MaxLines: maxLines, DisableHighlight: noHighlight}
	if repoDir != "" {
		vres, err := validate.Run(doc, repoDir)
		if err != nil {
			fmt.Fprintf(stderr, "校验无法执行（继续渲染）: %v\n", err)
		} else {
			printDiagnostics(vres, false, stdout, stderr)
			opts.Warnings = append(opts.Warnings, vres.Warnings...)
			opts.Warnings = append(opts.Warnings, vres.Errors...)
			if !vres.OK {
				fmt.Fprintf(stderr, "render: 校验未通过（%d 个错误），诊断已标注到状态机节点\n", len(vres.Errors))
			}
		}
	}

	res, err := render.Build(doc, opts)
	if err != nil {
		fmt.Fprintf(stderr, "渲染失败: %v\n", err)
		return 2
	}
	for _, w := range res.Warnings {
		if isValidateWarning(opts.Warnings, w) {
			continue
		}
		fmt.Fprintln(stderr, report.FormatHuman(w))
	}
	if err := os.WriteFile(outPath, res.HTML, 0o644); err != nil {
		fmt.Fprintf(stderr, "写出 %s 失败: %v\n", outPath, err)
		return 2
	}
	large := ""
	if res.LargeMode {
		large = "，大型评审模式"
	}
	fmt.Fprintf(stderr, "render: ok → %s（%d 条链，%d 个意图，%d 行 diff%s）\n",
		outPath, len(doc.Chains), countIntents(doc), res.TotalLines, large)
	return 0
}
