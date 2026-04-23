# proxy-lb

Gin 写的独立大模型转发与负载均衡服务。

它对外暴露 OpenAI 风格接口：

- `GET /healthz`
- `GET /readyz`
- `POST /auth/register`
- `POST /auth/login`
- `GET /auth/me`
- `POST /auth/tokens/issue`
- `GET /v1/models`
- `POST /v1/chat/completions`

对内按配置把请求分发到不同上游，并做：

- 多后端轮询
- 单次请求失败自动故障转移
- OpenAI 兼容上游直通流式
- Gemini 上游请求转换与响应归一化
- 静态 `auth_token` 与动态签发 token 双鉴权
- 本地用户/令牌存储（默认 `data/auth.json`）

## 启动

最省事的方式：

```bash
cd proxy-lb
bash scripts/start.sh
```

第一次运行如果还没有 `config/config.yaml`，脚本会自动从示例配置生成一份；你把模型参数填好后，再执行一次同样的命令就会直接启动。

也可以手动启动：

```bash
cd proxy-lb
cp config/config.example.yaml config/config.yaml
go mod tidy
go run ./cmd/proxy-lb
```

也可以通过环境变量指定配置路径：

```bash
PROXY_LB_CONFIG=/path/to/config.yaml go run ./cmd/proxy-lb
```

如果想先编译再启动：

```bash
cd proxy-lb
make build
./bin/proxy-lb
```

## 配置说明

- `models[].name` 是对外暴露给客户端的逻辑模型名
- `models[].backends[].model` 是实际发给上游厂商的模型名
- `models[].backends[].name` 可选，不填会自动按 `model@base_url` 生成
- OpenAI 兼容上游的 `base_url` 可以写成根路径、`/v1` 或完整 `/v1/chat/completions`
- Gemini 需要填写完整的 `generateContent` 地址
- `server.auth_data_path` 不填时默认写到配置文件同目录下的 `data/auth.json`

## 鉴权流程

1. 也可以直接拿配置里的静态 `server.auth_token` 调用 `/v1/chat/completions`
2. 如果希望使用用户登录方式，可以先调用 `/auth/register` 或 `/auth/login`
3. 登录成功会返回一个 bearer token，后续可直接拿这个 token 调 `/v1/models` 和 `/v1/chat/completions`
4. 已登录用户还可以调用 `/auth/tokens/issue` 生成新的调用 token

## Docker

```bash
cd proxy-lb
docker build -t proxy-lb .
docker run --rm -p 8088:8088 -v "$PWD/config:/app/config" -v "$PWD/data:/app/data" proxy-lb
```

## 主项目接入

把主项目的 `config/config.yaml` 里的 LLM 指向这个服务：

```yaml
llm:
  api_key: "replace-with-your-service-token"
  base_url: "http://your-server:8088/v1/chat/completions"
  model: "qwen"
  provider: ""
  stream_mode: both
```
