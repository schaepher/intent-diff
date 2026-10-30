package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"intent-review/internal/revdiff"
	"intent-review/internal/schema"
)

func runWatch(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("watch", flag.ContinueOnError)
	fs.SetOutput(stderr)
	req := renderRequest{}
	interval := fs.Duration("interval", 400*time.Millisecond, "文件轮询间隔")
	newRenderFlags(fs, &req)
	fs.Usage = func() {
		fmt.Fprintln(stderr, "用法: intent-review watch <file.json> -o <review.html> [--repo .] [--max-lines N] [--config F] [--exclude G] [--unexclude G] [--source-root P]")
		fs.PrintDefaults()
	}
	if err := parseFlagSet(fs, args); err != nil {
		return 2
	}
	if fs.NArg() != 1 || req.outPath == "" {
		fs.Usage()
		return 2
	}
	req.input = fs.Arg(0)
	if code := renderFile(req, stdout, stderr); code != 0 {
		return code
	}
	prev := loadQuiet(req.input)
	last := fileStamp(req.input)
	fmt.Fprintf(stderr, "watch: 监听 %s（Ctrl-C 退出；输出固定为 %s，可直接刷新）\n", req.input, req.outPath)
	ticker := time.NewTicker(*interval)
	defer ticker.Stop()
	for range ticker.C {
		now := fileStamp(req.input)
		if now == last {
			continue
		}
		last = now
		fmt.Fprintf(stderr, "watch: 检测到变更，重新 fill/validate/render\n")
		if code := renderFile(req, stdout, stderr); code != 0 {
			fmt.Fprintf(stderr, "watch: 本次渲染失败（继续监听）\n")
		}
		next := loadQuiet(req.input)
		if prev != nil && next != nil {
			rep := revdiff.Compare(*prev, *next)
			fmt.Fprintf(stderr, "watch: 本次更新影响 %d 个意图\n%s\n", len(rep.Intents), rep.FormatHuman())
		}
		prev = next
	}
	return 0
}

// loadQuiet 读取文档用于变更比较；失败时返回 nil（渲染流程会报告真正的错误）。
func loadQuiet(path string) *schema.Document {
	doc, _, err := schema.Load(path)
	if err != nil {
		return nil
	}
	return &doc
}

func fileStamp(path string) string {
	st, err := os.Stat(path)
	if err != nil {
		return "missing"
	}
	return fmt.Sprintf("%d-%d", st.ModTime().UnixNano(), st.Size())
}
