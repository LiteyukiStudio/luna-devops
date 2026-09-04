# Luna DevOps 前端

`web/` 是 Luna DevOps 管理控制台。开发环境由 Vite 提供热更新并代理本地 API；生产构建作为静态
资源嵌入 Go API 镜像。工程规则见 [前端增量规范](AGENTS.md)。

## 环境要求

- Node.js 24
- `web/package.json` 声明的 pnpm 版本
- 本地 Luna API：`http://localhost:8080`

可通过 Corepack 启用 pnpm：

```bash
corepack enable
corepack prepare pnpm@11.1.0 --activate
```

## 本地运行

以下命令均从仓库根目录执行。

```bash
docker compose -f docker-compose-dev-db.yaml up -d
go run ./cmd/api
pnpm --dir web install --frozen-lockfile
pnpm --dir web dev
```

Vite 默认监听 `http://localhost:5173`，并把 `/api` 和 `/healthz` 代理到
`http://localhost:8080`；`/api` 同时支持 WebSocket。默认 API 基址为 `/api/v1`。

## 构建环境变量

`VITE_*` 会写入浏览器产物，不能存放 Token、密码、Secret 或私钥。本地临时值写入不提交的
`web/.env.local`，修改后重启 Vite。

| 变量 | 默认值 | 用途 |
| --- | --- | --- |
| `VITE_API_BASE_URL` | `/api/v1` | 覆盖 Luna API 基址。 |
| `VITE_DOCS_BASE_URL` | `https://luna-devops.liteyuki.org` | 覆盖文档站地址。 |
| `VITE_APP_COMMIT_SHA` | 未设置 | 标识构建版本；未设置时停用更新检查。 |

## 常用命令

```bash
pnpm --dir web test
pnpm --dir web lint
pnpm --dir web check:i18n
pnpm --dir web check:singletons
pnpm --dir web generate:api-types
pnpm --dir web build
pnpm --dir web preview
```

`lint` 已包含 i18n 检查。test、lint、dev 和 build 会先从根 OpenAPI 生成不入库的传输类型；
手工运行 `generate:api-types` 可刷新编辑器类型。完整前端门禁依次执行 test、lint、singletons
和 build；具体脚本以 `web/package.json` 为准。

## 生产交付

根 `Dockerfile` 安装锁定依赖、构建 `web/dist`，再用 `embed_web` 构建标签将 SPA 嵌入 API。
不要提交 `dist/`、`node_modules/` 或 `*.local`，也不要把 Vite 代理当作生产运行条件。

## 相关入口

- [仓库总览](../README.md)
- [前端增量规范](AGENTS.md)
- [贡献指南](../CONTRIBUTING.md)
- [OpenAPI](../openapi/openapi.yaml)
- [公开文档站源码](../docs/)
