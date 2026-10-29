package main

import (
	"flag"
	"fmt"
	"io"

	"intent-review/internal/schema"
)

func runSchema(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("schema", flag.ContinueOnError)
	fs.SetOutput(stderr)
	outPath := fs.String("output", "", "输出文件（缺省写 stdout）")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "用法: intent-review schema [--output schema.json]")
		fs.PrintDefaults()
	}
	if err := parseFlagSet(fs, args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fs.Usage()
		return 2
	}
	raw, err := schema.SchemaJSON()
	if err != nil {
		fmt.Fprintf(stderr, "生成 schema 失败: %v\n", err)
		return 2
	}
	raw = append(raw, '\n')
	if err := writeOut(*outPath, raw, stdout); err != nil {
		fmt.Fprintf(stderr, "写出失败: %v\n", err)
		return 2
	}
	return 0
}
