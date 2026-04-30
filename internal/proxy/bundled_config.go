package proxy

import _ "embed"

// bundledConfigYAML 发布版内置默认配置（与仓库 config/config.example.yaml 保持同步）。
//
//go:embed bundled_config.yaml
var bundledConfigYAML []byte

// bundledYAMLPathMarker 标记 readConfigRoot 的数据源为二进制内嵌，而非磁盘上的真实路径。
const bundledYAMLPathMarker = "<bundled-config.yaml>"

func isBundledYAMLPathMarker(p string) bool {
	return p != "" && p == bundledYAMLPathMarker
}
