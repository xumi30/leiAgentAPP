# leiAgent

基于 **Wails v2** 的本地桌面 Agent 应用（Go 后端 + React 前端）：多会话对话、意图路由（对话 / 规划 / 工具）、LLM 与 YAML 配置、文库与备忘录等。

## 文档

- **[功能说明（项目总览，含备忘录）](docs/product-features.md)** — 维护产品/功能清单请改此文件。

## 开发与构建

```bash
go mod tidy
cd frontend && npm install && npm run build && cd ..
wails dev    # 开发
wails build  # 发布构建
```

具体依赖与输出以本仓库 `go.mod`、`wails.json` 为准。
