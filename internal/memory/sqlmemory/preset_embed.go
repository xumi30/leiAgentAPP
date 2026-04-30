package sqlmemory

import "embed"

// 与 frontend/src/assets/ 下同名文件保持一致；发布版从 embed 读取，开发期优先读磁盘以便改图无需同步副本。
//
//go:embed presets
var embeddedPresets embed.FS
