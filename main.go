package main

import (
	"embed"
	"fmt"
	"os"
	"runtime"

	"leiAgent/internal/appruntime"
	"leiAgent/internal/proxy"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/linux"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
)

//go:embed all:frontend/dist
var assets embed.FS

// 与 Header 同源；Linux 打包仅产出裸二进制时需在此注入，窗口管理器才有任务栏/最小化图标。
// macOS/.app、Windows exe 另有 Wails 从同文件生成的 icns/ico。
//go:embed build/appicon.png
var appDesktopIconPNG []byte

func main() {
	if root, err := appruntime.BootstrapWorkingDirectory(); err != nil {
		fmt.Fprintf(os.Stderr, "bootstrap runtime root failed (%s): %v\n", root, err)
	}

	if err := proxy.EnsureConfigYAMLFromExample(); err != nil {
		fmt.Fprintf(os.Stderr, "init config.yaml: %v\n", err)
	}

	// Create an instance of the app structure
	app := NewApp()

	// Create application with options
	opts := &options.App{
		Title:  "leiAgent",
		Width:  2026,
		Height: 1024,
		// macOS：必须提供非 nil 的 Mac 选项，Darwin 前端才会把 zoomable 置为 1；
		// 否则左上角绿灯无法缩放窗口（Wails darwin/window.go 在 Mac==nil 时不设置 zoomable）。
		Mac: &mac.Options{
			DisableZoom: false,
		},
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		// 单实例：再次点击图标/启动 exe 时不新起进程，改为唤醒已有窗口（Windows 互斥体 / macOS 分布式通知 / Linux D-Bus）。
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId: "lei.leiAgent.desktop.single-instance",
			OnSecondInstanceLaunch: func(_ options.SecondInstanceData) {
				app.focusMainWindow()
			},
		},
		OnStartup:  app.startup,
		OnShutdown: app.shutdown,
		Bind: []interface{}{
			app,
		},
	}

	// Linux：运行时窗口/任务栏图标由下方 Icon 提供。
	// 说明：文件管理器里「leiAgent」二进制仍显示系统默认可执行文件图标（齿轮等）是正常现象；
	// 正确菜单图标需 freedesktop 资源，见 scripts/build-release-linux.sh 产出的 build/linux-dist 与 install-desktop-user.sh。
	if runtime.GOOS == "linux" && len(appDesktopIconPNG) > 0 {
		opts.Linux = &linux.Options{
			Icon:             appDesktopIconPNG,
			ProgramName:      "leiAgent",
			WebviewGpuPolicy: linux.WebviewGpuPolicyOnDemand,
			Messages:         linux.DefaultMessages(),
		}
	}

	err := wails.Run(opts)

	if err != nil {
		println("Error:", err.Error())
	}
}
