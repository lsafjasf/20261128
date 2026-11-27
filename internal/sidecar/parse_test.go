package sidecar

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestParseSumLinesExtInference(t *testing.T) {
	d := strings.Repeat("a", 64)
	f := writeTemp(t, "release.sha256", d+"  dist/app.bin\n"+d+" *dist/lib.so\n# 注释\n\n")
	claims, err := ParseFile(f)
	if err != nil {
		t.Fatal(err)
	}
	if len(claims) != 2 {
		t.Fatalf("期望 2 条断言, 得到 %d", len(claims))
	}
	if claims[0].Algorithm != "sha256" || claims[0].Path != "dist/app.bin" || claims[0].Unknown {
		t.Errorf("断言 0 异常: %+v", claims[0])
	}
	if claims[1].Path != "dist/lib.so" {
		t.Errorf("二进制标记未剥离: %+v", claims[1])
	}
	if claims[0].Line != 1 || claims[1].Line != 2 {
		t.Errorf("行号错误: %d %d", claims[0].Line, claims[1].Line)
	}
}

func TestParseSumLinesLenInference(t *testing.T) {
	f := writeTemp(t, "CHECKSUMS", strings.Repeat("b", 40)+"  a.txt\n")
	claims, err := ParseFile(f)
	if err != nil {
		t.Fatal(err)
	}
	if claims[0].Algorithm != "sha1" {
		t.Errorf("长度推断失败: %+v", claims[0])
	}
}

func TestParseSumLinesUnknownLength(t *testing.T) {
	f := writeTemp(t, "weird.sum", strings.Repeat("c", 17)+"  a.txt\n")
	claims, err := ParseFile(f)
	if err != nil {
		t.Fatal(err)
	}
	if !claims[0].Unknown {
		t.Errorf("奇长摘要应降级为未知算法: %+v", claims[0])
	}
}

func TestParseJSONExplicitAndKeyForms(t *testing.T) {
	d := strings.Repeat("d", 64)
	f := writeTemp(t, "manifest.json", `{"files":[
		{"path":"x.bin","algorithm":"SHA-256","digest":"`+d+`"},
		{"path":"y.bin","sha512":"`+strings.Repeat("e", 128)+`"},
		{"path":"z.bin","algorithm":"blake3","digest":"`+d+`"}
	]}`)
	claims, err := ParseFile(f)
	if err != nil {
		t.Fatal(err)
	}
	if len(claims) != 3 {
		t.Fatalf("期望 3 条, 得到 %d", len(claims))
	}
	if claims[0].Algorithm != "sha256" || claims[0].Unknown {
		t.Errorf("别名 SHA-256 未规范化: %+v", claims[0])
	}
	if claims[1].Algorithm != "sha512" {
		t.Errorf("算法名键形式失败: %+v", claims[1])
	}
	if !claims[2].Unknown || claims[2].Algorithm != "blake3" {
		t.Errorf("未知算法应保留原名并降级: %+v", claims[2])
	}
}

func TestNormalizePath(t *testing.T) {
	cases := map[string]string{
		"./a//b.txt": "a/b.txt",
		`a\b\c.txt`:  "a/b/c.txt",
		"a/./b.txt":  "a/b.txt",
		"x/../y.txt": "y.txt",
	}
	for in, want := range cases {
		if got := NormalizePath(in); got != want {
			t.Errorf("NormalizePath(%q) = %q, 期望 %q", in, got, want)
		}
	}
}
