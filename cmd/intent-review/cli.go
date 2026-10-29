package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"intent-review/internal/report"
	"intent-review/internal/schema"
)

// parseFlagSet 允许 flag 与位置参数任意交错（如 fill review.json --repo . -o out）。
// 标准 flag 包遇到第一个位置参数就停止解析，这里先分离 flag 再统一解析。
func parseFlagSet(fs *flag.FlagSet, args []string) error {
	var flags, positional []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			positional = append(positional, args[i+1:]...)
			break
		}
		if len(a) > 1 && strings.HasPrefix(a, "-") {
			flags = append(flags, a)
			name := strings.TrimLeft(a, "-")
			if strings.Contains(name, "=") {
				continue
			}
			f := fs.Lookup(name)
			if f == nil {
				continue // 交给 fs.Parse 报未知 flag
			}
			if bf, ok := f.Value.(interface{ IsBoolFlag() bool }); ok && bf.IsBoolFlag() {
				continue
			}
			if i+1 < len(args) {
				i++
				flags = append(flags, args[i])
			}
			continue
		}
		positional = append(positional, a)
	}
	ordered := append(flags, "--")
	ordered = append(ordered, positional...)
	return fs.Parse(ordered)
}

// loadDoc 读取评审文件；失败返回退出码。
func loadDoc(path string, stderr io.Writer) (schema.Document, []byte, int) {
	doc, raw, err := schema.Load(path)
	if err != nil {
		fmt.Fprintf(stderr, "读取 %s 失败: %v\n", path, err)
		return schema.Document{}, nil, 2
	}
	return doc, raw, 0
}

// printDiagnostics 输出诊断：--json 走 stdout，否则人类可读走 stderr。
func printDiagnostics(res report.Result, asJSON bool, stdout, stderr io.Writer) {
	if asJSON {
		raw, err := res.JSON()
		if err != nil {
			fmt.Fprintf(stderr, "序列化诊断失败: %v\n", err)
			return
		}
		fmt.Fprintln(stdout, string(raw))
		return
	}
	for _, e := range res.Errors {
		fmt.Fprintln(stderr, report.FormatHuman(e))
	}
	for _, w := range res.Warnings {
		fmt.Fprintln(stderr, report.FormatHuman(w))
	}
}

// writeOut 把内容写到文件或 stdout（path 为空时）。
func writeOut(path string, data []byte, stdout io.Writer) error {
	if path == "" {
		_, err := stdout.Write(data)
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func hasPatchOnly(doc schema.Document) bool {
	for _, c := range doc.Chains {
		for _, in := range c.Intents {
			for _, ch := range in.Changes {
				if len(ch.Patch) > 0 && len(ch.Lines) == 0 {
					return true
				}
			}
		}
	}
	return false
}

// decodeFilled 把 fill 输出重新解码为 Document。
func decodeFilled(raw []byte) (schema.Document, error) {
	var doc schema.Document
	if err := json.Unmarshal(raw, &doc); err != nil {
		return schema.Document{}, fmt.Errorf("解析填充结果失败: %w", err)
	}
	return doc, nil
}

// countIntents 统计整份文档的意图数。
func countIntents(doc schema.Document) int {
	n := 0
	for _, c := range doc.Chains {
		n += len(c.Intents)
	}
	return n
}
