# leiAgent

基于 **Wails v2** 的本地桌面 Agent 应用（Go 后端 + React 前端）：多会话对话、意图路由（对话 / 规划 / 工具）、LLM 与 YAML 配置、文库与备忘录等。

## 文档

- **[产品介绍与使用说明（单页网页）](docs/leiagent-intro.html)** — 配置 LLM、能力与交互说明、Release 下载入口。
- **[功能说明（项目总览，含备忘录）](docs/product-features.md)** — 维护产品/功能清单请改此文件。

## 开发与构建

```bash
go mod download
cd frontend && npm install && npm run build && cd ..
wails dev    # 开发
bash scripts/build-release.sh  # macOS 发布构建
```

说明：

- macOS 发布请优先使用 `scripts/build-release.sh`，不要直接用 `wails build`。
- 这个脚本会显式执行前端构建、Go 编译、`.app` 打包和 ad-hoc 签名，避开当前环境下 Wails CLI 在 `node_modules` 扫描、`UTType` 链接和 GUI 启动工作目录上的不稳定点。
- Windows 下可继续使用 `scripts/build-release.ps1`。

## 独立 LLM 后端

仓库内置了一个可单独部署的 Gin 服务 [`proxy-lb/`](/Users/lei/codes/leiAgentAPP/proxy-lb/README.md)，用于把桌面端 `proxy` 之后的模型实际请求下沉到服务器端执行。

典型接法：

1. 在服务器上启动 `proxy-lb`
2. 将主项目 `config/config.yaml` 中的 `llm.base_url` 改成 `http://your-server:8088/v1/chat/completions`
3. 将 `llm.api_key` 改成 `proxy-lb` 的服务 token
4. 将 `llm.model` 改成 `proxy-lb` 中配置的逻辑模型名，例如 `qwen`

## 发布前检查

```bash
npm run release:check
bash scripts/build-release.sh
```

发布清单见 [docs/release-readiness.md](docs/release-readiness.md)。正式分发前请确认没有提交真实 API Key，并完成 macOS/Windows 签名与隐私说明。
