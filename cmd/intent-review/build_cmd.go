package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"intent-review/internal/fill"
	"intent-review/internal/render"
	"intent-review/internal/report"
	"intent-review/internal/revdiff"
	"intent-review/internal/schema"
	"intent-review/internal/validate"
)

// defaultArtifactDir 是评审产物目录（已 gitignore，不随业务代码提交）。
const defaultArtifactDir = ".intent-review"

func runBuild(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("build", flag.ContinueOnError)
	fs.SetOutput(stderr)
	req := renderRequest{}
	outDir := fs.String("out-dir", defaultArtifactDir, "评审产物目录（gitignore；含 filled JSON 与 HTML）")
	newCommonFlags(fs, &req)
	fs.Usage = func() {
		fmt.Fprintln(stderr, "用法: intent-review build <file.json> --repo <项目根> [--out-dir .intent-review] [--max-lines N] [--config F] [--exclude G] [--unexclude G] [--source-root P]")
		fs.PrintDefaults()
	}
	if err := parseFlagSet(fs, args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return 2
	}
	req.input = fs.Arg(0)
	if req.repoDir == "" {
		fmt.Fprintln(stderr, "build 需要 --repo <项目根>（重放与覆盖检查依赖 base/head 提交）")
		return 2
	}
	return buildProject(req, *outDir, stdout, stderr)
}

// buildProject 是“生成/更新”单一入口：fill → validate → render，产物固定在
// <out-dir>/review.filled.json 与 <out-dir>/review.html，页面地址可反复刷新。
func buildProject(req renderRequest, outDir string, stdout, stderr io.Writer) int {
	doc, raw, code := loadDoc(req.input, stderr)
	if code != 0 {
		return code
	}
	if e := schema.CheckVersion(doc.SchemaVersion); e != nil {
		res := report.NewResult([]report.Error{*e})
		printDiagnostics(res, false, stdout, stderr)
		return report.ExitCode(res)
	}
	filledPath := filepath.Join(outDir, "review.filled.json")
	htmlPath := filepath.Join(outDir, "review.html")
	prev := loadQuiet(filledPath)

	filled, fres, err := fill.Run(doc, raw, req.repoDir)
	if err != nil {
		fmt.Fprintf(stderr, "填充无法执行: %v\n", err)
		return 2
	}
	printDiagnostics(fres, false, stdout, stderr)
	if !fres.OK {
		fmt.Fprintf(stderr, "build: 填充失败（%d 个错误），未更新产物\n", len(fres.Errors))
		return report.ExitCode(fres)
	}
	canonical, err := schema.Decode(filled)
	if err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return 2
	}
	files, _, err := req.filter.build(req.repoDir, canonical.FileFilter)
	if err != nil {
		fmt.Fprintf(stderr, "文件过滤配置无效: %v\n", err)
		return 2
	}
	roots, err := resolveSourceRoots(canonical, req.repoDir, req.sourceRoots)
	if err != nil {
		fmt.Fprintf(stderr, "源码根解析失败: %v\n", err)
		return 2
	}
	vres, err := validate.Run(canonical, req.repoDir, validate.Options{Files: files})
	if err != nil {
		fmt.Fprintf(stderr, "校验无法执行: %v\n", err)
		return 2
	}
	printDiagnostics(vres, false, stdout, stderr)

	opts := render.Options{
		MaxLines: req.maxLines, DisableHighlight: req.noHighlight, SourceRoots: roots,
		Warnings:     append(append([]report.Error{}, vres.Warnings...), vres.Errors...),
		ExcludeRules: vres.ExcludeRules, Excluded: vres.Excluded,
	}
	rres, err := render.Build(canonical, opts)
	if err != nil {
		fmt.Fprintf(stderr, "渲染失败: %v\n", err)
		return 2
	}
	for _, w := range rres.Warnings {
		if !isValidateWarning(opts.Warnings, w) {
			fmt.Fprintln(stderr, report.FormatHuman(w))
		}
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		fmt.Fprintf(stderr, "创建产物目录 %s 失败: %v\n", outDir, err)
		return 2
	}
	if err := os.WriteFile(filledPath, filled, 0o644); err != nil {
		fmt.Fprintf(stderr, "写出 %s 失败: %v\n", filledPath, err)
		return 2
	}
	if err := os.WriteFile(htmlPath, rres.HTML, 0o644); err != nil {
		fmt.Fprintf(stderr, "写出 %s 失败: %v\n", htmlPath, err)
		return 2
	}
	fmt.Fprintf(stderr, "build: 评审范围：排除 %d 条规则，跳过 %d 个文件\n", len(vres.ExcludeRules), len(vres.Excluded))
	if prev != nil {
		rep := revdiff.Compare(*prev, canonical)
		fmt.Fprintf(stderr, "build: 本次更新影响 %d 个意图\n%s\n", len(rep.Intents), rep.FormatHuman())
	} else {
		fmt.Fprintf(stderr, "build: 首次生成，共 %d 个意图\n", countIntents(canonical))
	}
	status := "ok"
	if !vres.OK {
		status = "校验未通过"
	}
	fmt.Fprintf(stderr, "build: %s → %s（%d 条链，%d 个意图，%d 行 diff）\n",
		status, htmlPath, len(canonical.Chains), countIntents(canonical), rres.TotalLines)
	fmt.Fprintf(stderr, "build: 产物已写入 %s（该目录已 gitignore，勿提交）\n", outDir)
	return report.ExitCode(vres)
}

func runClean(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("clean", flag.ContinueOnError)
	fs.SetOutput(stderr)
	outDir := fs.String("out-dir", defaultArtifactDir, "要清理的评审产物目录")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "用法: intent-review clean [--out-dir .intent-review]")
		fs.PrintDefaults()
	}
	if err := parseFlagSet(fs, args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fs.Usage()
		return 2
	}
	dir := filepath.Clean(*outDir)
	if !safeArtifactDir(dir) {
		fmt.Fprintf(stderr, "clean: 拒绝删除 %q（只清理 .intent-review 或含 review.filled.json/review.html 的目录）\n", dir)
		return 2
	}
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		fmt.Fprintf(stderr, "clean: 无产物（%s 不存在）\n", dir)
		return 0
	}
	if err := os.RemoveAll(dir); err != nil {
		fmt.Fprintf(stderr, "clean: 删除 %s 失败: %v\n", dir, err)
		return 2
	}
	fmt.Fprintf(stderr, "clean: 已删除 %s（评审产物不会进入业务提交）\n", dir)
	return 0
}

// safeArtifactDir 防止 clean 误删仓库/根目录：只接受默认产物目录名或含产物的目录。
func safeArtifactDir(dir string) bool {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return false
	}
	if abs == string(filepath.Separator) {
		return false
	}
	if home, err := os.UserHomeDir(); err == nil && abs == home {
		return false
	}
	if filepath.Base(abs) == defaultArtifactDir {
		return true
	}
	for _, name := range []string{"review.filled.json", "review.html"} {
		if _, err := os.Stat(filepath.Join(abs, name)); err == nil {
			return true
		}
	}
	return false
}
