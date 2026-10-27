package opener

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kennethfan/wechat-file-keeper/internal/model"
)

// 图片文件头。
var (
	jpegMagic = []byte{0xFF, 0xD8, 0xFF}
	pngMagic  = []byte{0x89, 0x50, 0x4E, 0x47}
	// 微信私有编码文件头。
	weixinMagic = []byte{0x07, 0x08, 0x56, 0x32}
)

// writeDat 写一个带指定文件头的 .dat，返回路径。
func writeDat(t *testing.T, dir, name string, magic []byte) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("创建目录失败: %v", err)
	}
	content := append(append([]byte{}, magic...), []byte("image-payload")...)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("写文件失败: %v", err)
	}
	return path
}

func TestPrepareDat_ConvertsJpegToCorrectExtension(t *testing.T) {
	dir := t.TempDir()
	path := writeDat(t, dir, "619f2dc1_t_M.dat", jpegMagic)

	got, err := prepareDat(path)
	if err != nil {
		t.Fatalf("转换不应报错: %v", err)
	}
	if !strings.HasSuffix(got, ".jpg") {
		t.Fatalf("JPEG 应转成 .jpg，实际 %s", got)
	}
	if _, statErr := os.Stat(got); statErr != nil {
		t.Fatalf("临时文件应存在: %v", statErr)
	}
}

func TestPrepareDat_ConvertsPngToCorrectExtension(t *testing.T) {
	dir := t.TempDir()
	path := writeDat(t, dir, "abc_M.dat", pngMagic)

	got, err := prepareDat(path)
	if err != nil {
		t.Fatalf("转换不应报错: %v", err)
	}
	if !strings.HasSuffix(got, ".png") {
		t.Fatalf("PNG 应转成 .png，实际 %s", got)
	}
}

func TestPrepareDat_PreservesBaseName(t *testing.T) {
	dir := t.TempDir()
	path := writeDat(t, dir, "619f2dc1_t_M.dat", jpegMagic)

	got, err := prepareDat(path)
	if err != nil {
		t.Fatalf("转换不应报错: %v", err)
	}
	// 基名应保留，便于用户认出是哪个文件。
	if filepath.Base(got) != "619f2dc1_t_M.jpg" {
		t.Fatalf("基名应保留为 619f2dc1_t_M.jpg，实际 %s", filepath.Base(got))
	}
}

func TestPrepareDat_WeixinPrivateReportsError(t *testing.T) {
	dir := t.TempDir()
	path := writeDat(t, dir, "abc_h.dat", weixinMagic)

	_, err := prepareDat(path)
	if err == nil {
		t.Fatal("微信私有编码应报错，不能静默失败")
	}
	if !strings.Contains(err.Error(), "私有编码") {
		t.Fatalf("错误信息应说明是私有编码，实际: %v", err)
	}
}

func TestPrepareDat_MissingFileReportsError(t *testing.T) {
	_, err := prepareDat("/not/exist/x.dat")
	if err == nil {
		t.Fatal("文件不存在时应报错")
	}
}

func TestOpen_MissingFileReportsError(t *testing.T) {
	err := Open(model.Entry{Path: "/not/exist/x.pdf"})
	if err == nil {
		t.Fatal("文件不存在时应报错")
	}
}

func TestOpen_WeixinPrivateDatReportsError(t *testing.T) {
	dir := t.TempDir()
	path := writeDat(t, dir, "abc_h.dat", weixinMagic)

	err := Open(model.Entry{Path: path})
	if err == nil {
		t.Fatal("私有编码应报错")
	}
}

func TestReveal_MissingFileReportsError(t *testing.T) {
	err := Reveal(model.Entry{Path: "/not/exist/x.pdf"})
	if err == nil {
		t.Fatal("文件不存在时应报错")
	}
}

func TestCleanTemp_RemovesDirectory(t *testing.T) {
	// 先造一个临时目录。
	dir, err := tempDir()
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "x.jpg"), []byte("x"), 0o644); err != nil {
		t.Fatalf("写文件失败: %v", err)
	}

	if err := CleanTemp(); err != nil {
		t.Fatalf("清理不应报错: %v", err)
	}
	if _, statErr := os.Stat(dir); !os.IsNotExist(statErr) {
		t.Fatal("临时目录应被删除")
	}
}

func TestCleanTemp_NoDirectoryIsNotAnError(t *testing.T) {
	// 目录本来就不存在时不应报错。
	CleanTemp()

	if err := CleanTemp(); err != nil {
		t.Fatalf("重复清理不应报错: %v", err)
	}
}

func TestBaseWithoutExt(t *testing.T) {
	cases := map[string]string{
		"/a/b/abc_t_M.dat": "abc_t_M",
		"/a/b/x.pdf":       "x",
		"/a/b/noext":       "noext",
	}
	for in, want := range cases {
		if got := baseWithoutExt(in); got != want {
			t.Fatalf("baseWithoutExt(%q) = %q，期望 %q", in, got, want)
		}
	}
}
