package render

import (
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"intent-review/internal/schema"
)

// 源码链接必须限制在 --source-root 之内：绝对路径与越界路径一律不生成链接。
func TestFileLinkStaysInsideSourceRoot(t *testing.T) {
	root := t.TempDir()
	for _, file := range []string{
		"/etc/passwd", "../secret.go", "a/../../b.go", `..\secret.go`,
		"C:/windows/x.go", "c:x.go", "", "   ",
	} {
		if got := fileLink(root, file, 1); got != "" {
			t.Errorf("fileLink(%q) = %q, want empty", file, got)
		}
	}
	// 合法路径统一转换成绝对路径；未越界的 .. 会被清理。
	got := string(fileLink(root, "pkg/../a.go", 3))
	want := "vscode://file" + filepath.ToSlash(filepath.Join(root, "a.go")) + ":3"
	if got != want {
		t.Fatalf("fileLink = %q, want %q", got, want)
	}
	if !strings.Contains(string(fileLink("relative-root", "a.go", 1)), "vscode://file/") {
		t.Fatalf("相对源码根也应转换为绝对路径")
	}
}

// URI 编码：中文、空格、#、?、引号、尖括号都不能原样出现在 href 里。
func TestFileLinkEncodesSpecialCharacters(t *testing.T) {
	root := filepath.Join(t.TempDir(), "src root")
	got := string(fileLink(root, `源码/页面 说明#1?"<>'&.go`, 7))
	for _, want := range []string{"%E6%BA%90%E7%A0%81", "%20", "%23", "%3F", "%22", "%3C", "%3E", "&", ":7"} {
		if !strings.Contains(got, want) {
			t.Fatalf("href 缺少编码 %q: %s", want, got)
		}
	}
	if strings.ContainsAny(got, `"<>`) {
		t.Fatalf("href 含未编码的特殊字符: %s", got)
	}
}

// 页面级转义：特殊文件名既生成正确链接，也不会注入属性/脚本。
func TestBuildEscapesSpecialFileNames(t *testing.T) {
	root := t.TempDir()
	evil := `x" onmouseover="alert(1)<script>.go`
	chinese := `源码/页面 说明#1?.tsx`
	doc := changeDoc("c", "i", []schema.Change{
		{File: evil, Location: "入口", BusinessExplanation: "e", OldStart: 1, NewStart: 1,
			Lines: []schema.Line{{Type: schema.LineAdd, Content: "y"}}},
		{File: chinese, Location: "入口", BusinessExplanation: "e", OldStart: 1, NewStart: 1,
			Lines: []schema.Line{{Type: schema.LineAdd, Content: "z"}}},
	})
	res, err := Build(doc, Options{DisableHighlight: true, SourceRoots: map[string]string{"": root}})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	html := string(res.HTML)
	if strings.Contains(html, "alert(1)<script>.go") || strings.Contains(html, `onmouseover="alert(1)`) {
		t.Fatalf("文件名注入了属性/脚本")
	}
	if !strings.Contains(html, "&lt;script&gt;.go") {
		t.Fatalf("尖括号未转义")
	}
	if !strings.Contains(html, "&#34;") {
		t.Fatalf("引号未转义")
	}
	// 显示文本保留中文/空格/#/?（文本上下文安全），链接里则是百分号编码。
	if !strings.Contains(html, chinese) {
		t.Fatalf("中文文件名显示异常")
	}
	hrefs := regexp.MustCompile(`href="([^"]*)"`).FindAllStringSubmatch(html, -1)
	found := false
	for _, m := range hrefs {
		href := m[1]
		if strings.ContainsAny(href, `"<>`) {
			t.Fatalf("href 含未转义字符: %s", href)
		}
		if strings.Contains(href, "%E6%BA%90%E7%A0%81") && strings.Contains(href, "%23") && strings.Contains(href, "%3F") {
			found = true
		}
	}
	if !found {
		t.Fatalf("未找到编码后的中文链接")
	}
}
