// Command wechat-file-keeper 是 macOS 微信本地文件管理工具。
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/kennethfan/wechat-file-keeper/internal/account"
	"github.com/kennethfan/wechat-file-keeper/internal/format"
	"github.com/kennethfan/wechat-file-keeper/internal/scan"
)

// usage 打印命令帮助。
func usage() {
	fmt.Fprint(os.Stderr, `微信文件管家 — 只读扫描、显式确认才动手

用法:
  wechat-file-keeper scan              扫描账号库并显示空间统计

选项:
  --root <路径>   指定微信数据根目录（默认为本机标准位置）
  --lib <路径>    直接指定账号库目录，跳过自动发现
  -h, --help      显示本帮助
`)
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		os.Exit(1)
	}
}

// run 分发子命令。
func run(args []string) error {
	if len(args) == 0 {
		usage()
		return errors.New("缺少子命令")
	}

	cmd := args[0]
	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	root := fs.String("root", "", "微信数据根目录")
	libPath := fs.String("lib", "", "直接指定账号库目录")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}

	switch cmd {
	case "scan":
		return runScan(*root, *libPath)
	case "-h", "--help", "help":
		usage()
		return nil
	default:
		usage()
		return fmt.Errorf("未知子命令 %q", cmd)
	}
}

// resolveLibrary 解析出要操作的账号库。
//
// 优先使用显式指定的账号库目录；否则在根目录下发现账号库并选实占最大的。
func resolveLibrary(root, libPath string) (account.Library, error) {
	if libPath != "" {
		info, err := os.Stat(libPath)
		if err != nil {
			return account.Library{}, fmt.Errorf("无法访问账号库 %s: %w", libPath, err)
		}
		if !info.IsDir() {
			return account.Library{}, fmt.Errorf("%s 不是目录", libPath)
		}
		return account.Library{Path: libPath, WXID: pathBase(libPath)}, nil
	}

	if root == "" {
		defaultRoot, err := account.Root()
		if err != nil {
			return account.Library{}, err
		}
		root = defaultRoot
	}
	return account.Largest(root)
}

// pathBase 取路径最后一段，避免额外引入 path/filepath 别名混淆。
func pathBase(p string) string {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' {
			return p[i+1:]
		}
	}
	return p
}

// runScan 执行只读扫描并打印统计。
func runScan(root, libPath string) error {
	lib, err := resolveLibrary(root, libPath)
	if err != nil {
		return err
	}

	res, err := scan.New(lib.Path).Run()
	if err != nil {
		return err
	}

	sp := res.Space
	fmt.Printf("账号库    %s\n", lib.WXID)
	fmt.Printf("路径      %s\n", lib.Path)
	fmt.Println()
	fmt.Printf("实占大小  %s\n", format.Bytes(sp.Actual))
	fmt.Printf("名义大小  %s\n", format.Bytes(sp.Nominal))
	if sp.HardlinkOvercount > 0 {
		fmt.Printf("硬链接虚增 %s  （同一 inode 挂了多个名字，删除不释放空间）\n",
			format.Bytes(sp.HardlinkOvercount))
	}
	fmt.Println()
	fmt.Printf("条目 %s 条 / 按 inode 归并 %s 个文件\n",
		format.Count(sp.EntryCount), format.Count(sp.FileCount))
	fmt.Printf("扫描耗时  %s\n", res.Duration.Round(1e6))
	return nil
}
