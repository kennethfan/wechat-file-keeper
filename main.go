// Command wechat-file-keeper 是 macOS 微信文件管理工具。
//
// 默认启动图形界面（Wails + WebView）；带子命令时走命令行模式，
// 供脚本与调试使用。
package main

import (
	"embed"
	"fmt"
	"os"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/kennethfan/wechat-file-keeper/internal/apiserver"
	"github.com/kennethfan/wechat-file-keeper/internal/bridge"
	"github.com/kennethfan/wechat-file-keeper/internal/cli"
)

// assets 嵌入前端构建产物。
//
//go:embed all:frontend/dist
var assets embed.FS

// version 由构建时注入。
var version = "0.2.0"

func main() {
	// 有子命令时走 CLI，否则开界面。
	if len(os.Args) > 1 && !isAppFlag(os.Args[1]) {
		if err := cli.Run(os.Args[1:]); err != nil {
			fmt.Fprintf(os.Stderr, "错误: %v\n", err)
			os.Exit(1)
		}
		return
	}

	runGUI()
}

// isAppFlag 判断参数是否是启动器传入的选项（如 -psn_xxx）。
func isAppFlag(arg string) bool {
	return len(arg) > 0 && arg[0] == '-'
}

// runGUI 启动图形界面。
func runGUI() {
	service := bridge.New()

	app := application.New(application.Options{
		Name:        "微清 WeClean",
		Description: "macOS 微信文件清理工具",
		Services: []application.Service{
			application.NewService(service),
		},
		Assets: application.AssetOptions{
			// 用自定义 handler 同时提供静态资源与 JSON API。
			// 不走 Wails 绑定机制：它依赖的 /wails/runtime.js 在本机
			// WebKit 上加载失败，会让所有后端调用静默不生效。
			Handler: apiserver.New(application.AssetFileServerFS(assets), service),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title: "微清 WeClean — 微信文件清理",
		// 与原型一致：1120x740，最小 900x600。
		Width:     1120,
		Height:    740,
		MinWidth:  900,
		MinHeight: 600,
		// 隐藏标题栏，让前端自绘 macOS 红绿灯区域。
		Mac: application.MacWindow{
			// 隐藏系统标题栏，由前端自绘（原型设计里标题栏与内容连成一体）。
			// InvisibleTitleBarHeight 让顶部 44px 仍可拖动窗口，
			// 否则隐藏标题栏后窗口就拖不动了。
			TitleBar: application.MacTitleBar{
				Hide: true,
			},
			InvisibleTitleBarHeight: 44,
			Backdrop:                application.MacBackdropTranslucent,
		},
		BackgroundColour: application.NewRGB(245, 245, 247),
		URL:              "/",
	})

	if err := app.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "启动失败: %v\n", err)
		os.Exit(1)
	}
}
