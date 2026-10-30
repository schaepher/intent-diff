package filefilter

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// ConfigFileName 是缺省配置文件名：放在仓库根（--repo）或当前目录。
const ConfigFileName = ".intent-review.json"

// FileFilter 是配置文件里的 file_filter 段：在**内置默认列表**上增删。
type FileFilter struct {
	// Exclude 追加排除项。
	Exclude []string `json:"exclude,omitempty"`
	// Unexclude 从（默认 + Exclude）中按字面值删除，用于让内置项重新纳入评审。
	Unexclude []string `json:"unexclude,omitempty"`
}

// Config 是配置文件结构。未知字段会报错（避免拼写错误被静默忽略）。
type Config struct {
	FileFilter FileFilter `json:"file_filter"`
}

// Resolve 计算配置文件生效后的排除列表：
// 内置默认 → 追加 Exclude → 删除 Unexclude（删除优先）。
func Resolve(cfg FileFilter) *List {
	return Default().Add(cfg.Exclude...).Remove(cfg.Unexclude...)
}

// Load 读取配置：path 非空时按显式路径读取（不存在即错误）；
// 否则在 dir（空则当前目录）下找 ConfigFileName，找不到返回空配置。
// 返回实际使用的配置文件路径（未使用时为空）。
func Load(path, dir string) (FileFilter, string, error) {
	if path != "" {
		cfg, err := readConfig(path)
		if err != nil {
			return FileFilter{}, "", err
		}
		return cfg, path, nil
	}
	if dir == "" {
		dir = "."
	}
	p := filepath.Join(dir, ConfigFileName)
	cfg, err := readConfig(p)
	if errors.Is(err, fs.ErrNotExist) {
		return FileFilter{}, "", nil
	}
	if err != nil {
		return FileFilter{}, "", err
	}
	return cfg, p, nil
}

func readConfig(path string) (FileFilter, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return FileFilter{}, fmt.Errorf("配置文件 %s 不存在: %w", path, err)
		}
		return FileFilter{}, fmt.Errorf("读取配置文件 %s 失败: %w", path, err)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var cfg Config
	// 空文件（或只有空白）视为无配置。
	if err := dec.Decode(&cfg); err != nil && !errors.Is(err, io.EOF) {
		return FileFilter{}, fmt.Errorf("配置文件 %s 无法解析: %w", path, err)
	}
	return cfg.FileFilter, nil
}
