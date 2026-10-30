package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"intent-review/internal/render"
)

func runWatch(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("watch", flag.ContinueOnError)
	fs.SetOutput(stderr)
	repoDir := fs.String("repo", "", "仓库目录（附带校验并把诊断标注到状态机节点）")
	outPath := fs.String("o", "", "输出 HTML 文件（必填）")
	maxLines := fs.Int("max-lines", render.DefaultMaxLines, "大型评审降级阈值（总变更行数）")
	noHighlight := fs.Bool("no-highlight", false, "关闭语法高亮（调试用）")
	interval := fs.Duration("interval", 400*time.Millisecond, "文件轮询间隔")
	ff := &filterFlags{}
	addFilterFlags(fs, ff)
	fs.Usage = func() {
		fmt.Fprintln(stderr, "用法: intent-review watch <file.json> -o <review.html> [--repo .] [--max-lines N] [--config F] [--exclude G] [--unexclude G]")
		fs.PrintDefaults()
	}
	if err := parseFlagSet(fs, args); err != nil {
		return 2
	}
	if fs.NArg() != 1 || *outPath == "" {
		fs.Usage()
		return 2
	}
	input := fs.Arg(0)
	if code := renderFile(input, *outPath, *repoDir, *maxLines, *noHighlight, ff, stdout, stderr); code != 0 {
		return code
	}
	last := fileStamp(input)
	fmt.Fprintf(stderr, "watch: 监听 %s（Ctrl-C 退出）\n", input)
	ticker := time.NewTicker(*interval)
	defer ticker.Stop()
	for range ticker.C {
		now := fileStamp(input)
		if now == last {
			continue
		}
		last = now
		fmt.Fprintf(stderr, "watch: 检测到变更，重新渲染\n")
		if code := renderFile(input, *outPath, *repoDir, *maxLines, *noHighlight, ff, stdout, stderr); code != 0 {
			fmt.Fprintf(stderr, "watch: 本次渲染失败（继续监听）\n")
		}
	}
	return 0
}

func fileStamp(path string) string {
	st, err := os.Stat(path)
	if err != nil {
		return "missing"
	}
	return fmt.Sprintf("%d-%d", st.ModTime().UnixNano(), st.Size())
}
