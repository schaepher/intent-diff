package filefilter

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, dir, body string) string {
	t.Helper()
	path := filepath.Join(dir, ConfigFileName)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func TestLoadExplicitConfig(t *testing.T) {
	dir := t.TempDir()
	path := writeConfig(t, dir, `{
	  "file_filter": {
	    "exclude": ["**/*.pb.go"],
	    "unexclude": ["*_test.go"]
	  }
	}`)
	cfg, used, err := Load(path, "")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if used != path {
		t.Fatalf("used = %q, want %q", used, path)
	}
	if len(cfg.Exclude) != 1 || cfg.Exclude[0] != "**/*.pb.go" {
		t.Fatalf("exclude = %v", cfg.Exclude)
	}
	if len(cfg.Unexclude) != 1 || cfg.Unexclude[0] != "*_test.go" {
		t.Fatalf("unexclude = %v", cfg.Unexclude)
	}
}

func TestLoadDiscoversConfigInDir(t *testing.T) {
	dir := t.TempDir()
	path := writeConfig(t, dir, `{"file_filter": {"exclude": ["docs/**"]}}`)
	cfg, used, err := Load("", dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if used != path || len(cfg.Exclude) != 1 {
		t.Fatalf("used=%q cfg=%+v", used, cfg)
	}
}

func TestLoadMissingConfig(t *testing.T) {
	dir := t.TempDir()
	cfg, used, err := Load("", dir)
	if err != nil {
		t.Fatalf("缺省配置文件不存在不应报错: %v", err)
	}
	if used != "" || len(cfg.Exclude) != 0 || len(cfg.Unexclude) != 0 {
		t.Fatalf("used=%q cfg=%+v", used, cfg)
	}
	// 显式指定的路径不存在必须报错。
	if _, _, err := Load(filepath.Join(dir, "nope.json"), ""); err == nil {
		t.Fatalf("显式配置缺失应报错")
	}
}

func TestLoadRejectsUnknownFieldAndBadJSON(t *testing.T) {
	dir := t.TempDir()
	bad := writeConfig(t, dir, `{"file_filter": {"exclud": ["x"]}}`)
	if _, _, err := Load(bad, ""); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("未知字段应报错，got %v", err)
	}
	broken := writeConfig(t, dir, `{"file_filter": `)
	if _, _, err := Load(broken, ""); err == nil {
		t.Fatalf("非法 JSON 应报错")
	}
}

func TestLoadEmptyConfigFileMeansNoConfig(t *testing.T) {
	dir := t.TempDir()
	path := writeConfig(t, dir, "  ")
	cfg, used, err := Load(path, "")
	if err != nil {
		t.Fatalf("空配置不应报错: %v", err)
	}
	if used != path || len(cfg.Exclude) != 0 || len(cfg.Unexclude) != 0 {
		t.Fatalf("used=%q cfg=%+v", used, cfg)
	}
}

func TestResolveAppliesExcludeThenUnexclude(t *testing.T) {
	l := Resolve(FileFilter{Exclude: []string{"docs/**", "*_test.go"}, Unexclude: []string{"*_test.go"}})
	if !l.Match("docs/a.md") {
		t.Fatalf("配置 exclude 应生效")
	}
	if l.Match("pkg/a_test.go") {
		t.Fatalf("unexclude 应移除 _test.go")
	}
	if !l.Match("vendor/x/y.go") {
		t.Fatalf("未删除的内置项应保留")
	}
}

func TestResolveEmptyConfigKeepsDefaults(t *testing.T) {
	l := Resolve(FileFilter{})
	if !l.Match("a_test.go") || len(l.Patterns()) != len(DefaultPatterns) {
		t.Fatalf("空配置应等于内置默认列表: %v", l.Patterns())
	}
}
