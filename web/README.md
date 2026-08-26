# Luna DevOps 前端

`web/` 是 Luna DevOps 管理控制台，负责项目空间、应用、构建、部署、运行集群、网关、计费、平台设置和 AI 助手等用户界面。开发环境由 Vite 提供热更新并代理本地 API；生产环境构建为静态资源，由 Go API 以嵌入式 SPA 的形式交付。

## 技术栈

| 领域 | 方案 |
| --- | --- |
| 应用框架 | React 19、React Router 7、TypeScript 6、Vite 8 |
| 样式与组件 | Tailwind CSS 4、shadcn/ui、Radix UI、Lucide |
| 服务端状态 | TanStack Query 5 |
| 表单与校验 | React Hook Form、Zod |
| 国际化与反馈 | i18next、react-i18next、Sonner |
| 测试与质量 | Vitest、Testing Library、ESLint |

详细开发规则见 [前端开发规范](AGENTS.md)，组件选型见 [shadcn/ui 组件清单](SHADCN_COMPONENTS.md)。

## 环境要求

- 推荐使用与 CI 一致的 Node.js 24。
- 使用 `web/package.json` 声明的 pnpm 11.1.0；不要使用 npm 或 Yarn 改写锁文件。
- 本地联调需要 Luna DevOps API 监听 `http://localhost:8080`。

可通过 Corepack 启用项目声明的 pnpm：

```bash
corepack enable
corepack prepare pnpm@11.1.0 --activate
```

## 本地开发

以下命令均从仓库根目录执行。

先启动 PostgreSQL 和 Redis：

```bash
docker compose -f docker-compose-dev-db.yaml up -d
```

按照根目录 [README](../README.md) 准备 `.env`，然后启动 API：

```bash
go run ./cmd/api
```

安装锁定依赖并启动前端：

```bash
pnpm --dir web install --frozen-lockfile
pnpm --dir web dev
```

Vite 默认在 `http://localhost:5173` 提供页面，并将 `/api`、`/healthz` 代理到 `http://localhost:8080`；`/api` 同时支持 WebSocket 代理。默认 API 基址为 `/api/v1`，通常无需额外配置。

## 构建环境变量

前端当前只使用以下 `VITE_*` 变量。它们会在构建时写入浏览器产物，因此不得存放 Token、密码、Secret 或其他敏感信息。

| 变量 | 默认值 | 用途 |
| --- | --- | --- |
| `VITE_API_BASE_URL` | `/api/v1` | 覆盖浏览器请求 Luna API 的基址。 |
| `VITE_DOCS_BASE_URL` | `https://luna-devops.liteyuki.org` | 覆盖控制台跳转到文档站的基址。 |
| `VITE_APP_COMMIT_SHA` | 未设置（Docker 构建默认 `dev`） | 标识构建版本；未设置时更新检查停用，浏览器遥测使用 `dev` 标识。 |

本地临时覆盖可以写入未提交的 `web/.env.local`。修改后需要重启 Vite 开发服务器。

## 常用命令

| 命令 | 作用 |
| --- | --- |
| `pnpm --dir web dev` | 启动 Vite 开发服务器。 |
| `pnpm --dir web test` | 运行全部 Vitest 测试。 |
| `pnpm --dir web lint` | 运行 ESLint，并同时执行 i18n 完整性检查。 |
| `pnpm --dir web check:i18n` | 单独检查五种语言的 key、插值、bundle 和动态 key 白名单。 |
| `pnpm --dir web check:singletons` | 检查 React、React DOM、CodeMirror 等运行时单例依赖。 |
| `pnpm --dir web build` | 依次执行 TypeScript 构建检查、Vite 生产构建和静态资源压缩优化。 |
| `pnpm --dir web preview` | 本地预览 `dist/` 生产产物。 |

完整前端门禁与 CI 的 Web quality job 一致：

```bash
pnpm --dir web test
pnpm --dir web lint
pnpm --dir web check:singletons
pnpm --dir web build
```

## 目录结构

```text
web/
├── public/                 静态资源
├── scripts/                i18n、单例依赖和产物优化脚本
├── src/
│   ├── api/                API 公共请求层、类型和领域客户端
│   │   └── domains/        按业务域拆分的 API 方法
│   ├── app/                Session、主题、公开配置和应用级 Provider
│   ├── components/
│   │   ├── ui/             shadcn/ui 基础组件
│   │   └── common/         跨页面复用的业务组合组件
│   ├── i18n/               语言配置、核心翻译和按功能懒加载的翻译包
│   ├── layouts/            应用级布局
│   ├── lib/                与页面无关的公共逻辑
│   ├── pages/              按业务模块组织的页面与页面私有代码
│   ├── styles/             设计 token、品牌和主题样式
│   └── test/               全局测试环境
├── components.json         shadcn/ui 生成配置
├── vite.config.ts          Vite、Vitest、代理和构建配置
└── package.json            依赖与脚本
```

## 开发入口

开始修改前：

1. 阅读根目录 [`AGENTS.md`](../AGENTS.md) 和本目录 [`AGENTS.md`](AGENTS.md)。
2. 在 `src/components/ui`、`src/components/common` 和相邻页面中搜索可复用模式。
3. 对照 [OpenAPI](../openapi/openapi.yaml) 与现有领域客户端确认数据契约。
4. 新增路由时同步配置页面懒加载和对应翻译 bundle。
5. 行为变更时补充就近的 `*.test.ts` 或 `*.test.tsx`，并按改动风险执行质量门禁。

关键约定：

- 页面使用 `src/pages/<module>/` 组织；页面私有组件、Hook、模型和测试与页面共置。
- 共享模块使用 `@/` 根路径导入；相对导入只用于同一页面或组件目录内的私有文件。
- 页面不得直接请求外部平台或自行编写 `fetch`；统一使用 `@/api`，权限仍由后端最终判断。
- 基础 UI 优先复用 shadcn/ui，列表、页面结构、状态和表单优先复用已有公共组件。
- 所有用户可见文案及可访问性文案都必须进入 i18n。
- `dist/`、`node_modules/` 和 `*.local` 是本地产物，不提交到仓库。

## 生产交付

根目录 `Dockerfile` 会在独立阶段安装锁定依赖、运行 `pnpm --dir web build`，再把 `web/dist` 复制到 Go 的嵌入资源目录，并使用 `embed_web` 构建标签生成包含 SPA 的 API 镜像。不要手工提交 `dist/`，也不要依赖开发代理作为生产运行条件。

## 相关文档

- [仓库总览](../README.md)
- [前端开发规范](AGENTS.md)
- [shadcn/ui 组件清单与使用准则](SHADCN_COMPONENTS.md)
- [贡献指南](../CONTRIBUTING.md)
- [公开文档站源码](../docs/)
