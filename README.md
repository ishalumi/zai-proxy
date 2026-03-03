# zai-proxy

zai-proxy 是一个 Go 代理服务，把 z.ai 网页聊天接口转换成
OpenAI Chat Completions 兼容格式。

## 功能特性

- OpenAI API 兼容（`/v1/models`、`/v1/chat/completions`）
- 支持流式与非流式响应
- 支持模型标签：`-thinking`、`-search`（可组合）
- 支持多模态图片输入（URL / Base64），自动上传到 z.ai
- 支持匿名 Token（`Authorization: Bearer free`）
- 支持工具调用（`tools`、`tool_choice`、`tool_calls`）
- 自动生成签名并自动更新上游 FE 版本号

## 接口与默认行为

- `GET /v1/models`
- `POST /v1/chat/completions`

默认监听端口由 `PORT` 控制，未设置时为 `7990`。
如果请求体 `model` 为空，服务会使用默认模型 `GLM-4.6`。

## 环境变量配置

服务启动时会自动读取当前目录下的 `.env`（`godotenv.Load()`）。

| 变量名 | 默认值 | 说明 |
|---|---|---|
| `PORT` | `7990` | 服务监听端口 |
| `LOG_LEVEL` | `info` | 日志级别：`debug` / `info` / `warn` / `error` |
| `PROXY_URL` | 空 | 代理地址，配置后所有上游 HTTP 请求统一走代理 |

`PROXY_URL` 示例：

```env
PROXY_URL=http://127.0.0.1:7890
PROXY_URL=http://user:password@127.0.0.1:7890
```

代理会覆盖这些上游请求场景：

- 匿名 token 获取
- z.ai 聊天接口请求
- 图片下载与上传
- FE 版本号拉取

## 本地运行

```bash
git clone https://github.com/ishalumi/zai-proxy.git
cd zai-proxy
cp .env.example .env
go mod download
go run main.go
```

## Docker 部署（默认回环）

默认建议将端口绑定到本机回环地址，避免直接暴露公网：

```bash
docker run -d \
  --name zai-proxy \
  --restart unless-stopped \
  --env-file .env \
  -p 127.0.0.1:7990:7990 \
  ghcr.io/ishalumi/zai-proxy:latest
```

如果你把 `PORT` 改成了 `8001`，请同步改端口映射：`127.0.0.1:8001:8001`。

### docker-compose

项目内置 `docker-compose.yml`，已默认采用回环端口绑定：

```bash
docker compose up -d
```

## 获取 z.ai Token

### 方式一：匿名 Token（免登录）

直接使用 `free` 作为 API key：

```bash
curl http://127.0.0.1:7990/v1/chat/completions \
  -H "Authorization: Bearer free" \
  -H "Content-Type: application/json" \
  -d '{"model":"GLM-4.7","messages":[{"role":"user","content":"hello"}]}'
```

### 方式二：个人 Token

1. 登录 https://chat.z.ai
2. 打开开发者工具（F12）
3. 在 Cookies 中找到 `token`
4. 把该值放到 `Authorization: Bearer <token>`

## 支持模型

`/v1/models` 当前返回：

- `GLM-5`
- `GLM-5-thinking`
- `GLM-5-search`
- `GLM-5-thinking-search`
- `GLM-4.5`
- `GLM-4.6`
- `GLM-4.7`
- `GLM-4.7-thinking`
- `GLM-4.7-thinking-search`
- `GLM-4.5-V`
- `GLM-4.6-V`
- `GLM-4.6-V-thinking`
- `GLM-4.5-Air`

基础模型映射：

| OpenAI 模型名 | z.ai 上游模型 |
|---|---|
| `GLM-5` | `glm-5` |
| `GLM-4.5` | `0727-360B-API` |
| `GLM-4.6` | `GLM-4-6-API-V1` |
| `GLM-4.7` | `glm-4.7` |
| `GLM-4.5-V` | `glm-4.5v` |
| `GLM-4.6-V` | `glm-4.6v` |
| `GLM-4.5-Air` | `0727-106B-API` |

## 使用示例

### 流式请求

```bash
curl http://127.0.0.1:7990/v1/chat/completions \
  -H "Authorization: Bearer YOUR_ZAI_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "GLM-4.7-thinking-search",
    "messages": [{"role":"user","content":"hello"}],
    "stream": true
  }'
```

### 多模态请求（图片 URL / Base64）

```json
{
  "model": "GLM-4.6-V",
  "messages": [
    {
      "role": "user",
      "content": [
        {"type": "text", "text": "描述这张图片"},
        {"type": "image_url", "image_url": {"url": "https://example.com/image.jpg"}}
      ]
    }
  ]
}
```

## GitHub Releases 自动发布

仓库包含工作流：`.github/workflows/release-binaries.yml`

- 触发条件：推送 tag（如 `v1.2.3`）
- 执行内容：`go test ./... -timeout=60s`、多平台编译、打包、生成 SHA256
- 发布结果：自动上传到当前仓库的 GitHub Releases

示例：

```bash
git tag v1.2.3
git push fork v1.2.3
```
