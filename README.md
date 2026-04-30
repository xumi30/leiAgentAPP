# leiAgent

基于 **Wails v2** 的本地桌面 Agent 应用（Go 后端 + React 前端）：多会话对话、意图路由（对话 / 规划 / 工具）、LLM 与 YAML 配置、文库与备忘录等。

## 文档

- **[功能说明（项目总览，含备忘录）](docs/product-features.md)** — 维护产品/功能清单请改此文件。
- **[发布准备清单](docs/release-readiness.md)** — 公开分发前的安全、构建、签名和隐私检查。

## 开发与构建

```bash
go mod download
cd frontend && npm install && cd ..
wails dev    # 开发
wails build -clean    # 干净构建，会先生成 frontend/wailsjs bindings
bash scripts/build-release.sh  # macOS 发布构建
```

说明：

- `frontend/wailsjs/` 是 Wails 生成目录，不提交到 Git；干净克隆后不要在 Wails 生成 bindings 前单独执行 `frontend` 下的 `npm run build`。
- macOS 发布请优先使用 `scripts/build-release.sh`，不要直接用 `wails build`。
- 这个脚本会显式执行前端构建、Go 编译、`.app` 打包和 ad-hoc 签名。首次干净克隆如缺少 `frontend/wailsjs/`，请先执行一次 `wails build -clean` 或 `wails dev` 生成 bindings。
- Windows 下可继续使用 `scripts/build-release.ps1`。

## 独立 LLM 后端

当前主仓库只包含桌面端代理与配置逻辑；如果你单独部署了兼容 OpenAI Chat Completions 的 LLM 网关，可以把桌面端请求转发到该服务。

典型接法：

1. 在服务器上启动兼容 `/v1/chat/completions` 的 LLM 网关
2. 将主项目 `config/config.yaml` 中的 `llm.base_url` 改成 `http://your-server:8088/v1/chat/completions`
3. 将 `llm.api_key` 改成该网关的服务 token，或用 `LEIAGENT_LLM_API_KEY` 注入
4. 将 `llm.model` 改成该网关中配置的逻辑模型名，例如 `qwen`

## 发布前检查

```bash
npm run release:check
```

发布清单见 [docs/release-readiness.md](docs/release-readiness.md)。正式分发前请确认没有提交真实 API Key，并完成 macOS/Windows 签名与隐私说明。
