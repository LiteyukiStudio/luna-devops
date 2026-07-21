# 平台启动问题

这里处理 Docker Compose 启动时最常见的几类问题。应用构建或 Kubernetes 运行异常，请继续看“使用”部分的排障文档。

## 使用指定版本的镜像

默认 `docker-compose.yaml` 使用 `nightly` 镜像。验证 RC 或稳定版本时，在启动命令前设置 `DEVOPS_IMAGE_TAG`：

```bash
DEVOPS_IMAGE_TAG=v0.1.0-rc.1 docker compose up -d
```

如果你要从当前源码构建镜像，而不是拉取 DockerHub 镜像，使用源码构建 compose：

```bash
docker compose -f docker-compose-build.yaml up -d --build
```

## 端口 `8088` 被占用

查看占用：

```bash
lsof -nP -iTCP:8088 -sTCP:LISTEN
```

你可以停止占用进程，或者修改 `docker-compose.yaml` 里的端口映射：

```yaml
ports:
  - "8089:8080"
```

然后访问 `http://localhost:8089`。

## 页面能打开，但接口请求失败

先查看 API 日志：

```bash
docker compose logs -f api
```

再确认数据库和 Redis 是健康状态：

```bash
docker compose ps
```

## worker 没有正常启动

查看 worker 日志：

```bash
docker compose logs -f worker
```

Worker 负责构建、部署和状态同步。只有 API 运行时可以浏览控制台，但要真正构建和发布应用，Worker 也必须保持正常。

## 使用平台构建模板

部署配置的“构建定义”支持两种方式：

- **仓库 Dockerfile**：使用仓库中已有的 Dockerfile，适合已经维护容器构建流程的项目。
- **平台构建模板**：平台根据少量参数生成本次构建使用的 Dockerfile，适合没有 Dockerfile，或希望统一构建方式的项目。

平台内置以下常用模板：

- Go 服务
- Node.js 服务、Node.js 静态站点、Next.js 服务和 Bun 服务
- Python + uv 服务
- Rust 服务
- Ruby 服务
- Java Maven 服务和 Java Gradle 服务
- .NET 服务
- 纯静态站点

模板选择器使用各技术项目提供的官方标志，资源随控制台静态文件一起提供，不会在使用时请求第三方图标服务。

选择模板后，可以调整依赖安装、构建、启动命令和服务端口等必要参数，并在保存前预览生成结果。Java 模板默认使用 JDK/JRE 21，.NET 模板默认使用 .NET 8；项目使用其他版本或产物名称时，请按项目实际情况调整模板参数。

平台模板不会修改代码仓库。Worker 会把生成的 Dockerfile 作为独立文件挂载到构建 Job，再让 BuildKit 使用它和原仓库构建上下文进行构建。即使仓库中已经存在 Dockerfile，只要部署配置选择了平台模板，本次构建也只使用平台生成的版本。

每次构建都会保存模板 ID、模板版本、参数、生成后的 Dockerfile 校验和及内部快照。后续修改部署配置不会改变历史构建记录。

### 怎么选择模板

1. 在应用的“部署”页面创建或编辑部署配置。
2. 选择代码仓库后，平台会读取仓库文件并把更匹配的模板排在前面。
3. 在“构建定义”中选择“平台构建模板”。
4. 选择模板、确认参数并预览 Dockerfile。
5. 保存部署配置并创建构建。

平台只根据 `package.json`、`bun.lock`、`pyproject.toml`、`go.mod`、`Cargo.toml`、`Gemfile`、`pom.xml`、`build.gradle`、`*.csproj`、`index.html` 等文件提供建议，不会替用户猜测项目特有的启动命令。实际命令仍应以项目文档为准。

### Next.js 服务模板

Next.js 服务模板遵循 Next.js 官方 Docker standalone 示例，使用依赖、构建和运行三个阶段。模板会根据仓库中的 `package-lock.json`、`pnpm-lock.yaml` 或 `yarn.lock` 选择对应包管理器，最终镜像只复制 standalone 运行文件、`public` 和 `.next/static`，并以非 root 用户启动 `server.js`。

使用前需要在 `next.config.js`、`next.config.mjs` 或 `next.config.ts` 中启用 standalone 输出：

```ts
const nextConfig = {
  output: 'standalone',
}

export default nextConfig
```

平台不会改写仓库配置。缺少 standalone 输出时，模板会在构建阶段明确报错，而不是生成无法启动的镜像。仓库存在 `next.config.js`、`next.config.mjs` 或 `next.config.ts` 时，平台会优先推荐 Next.js 服务模板。

生产环境仍由平台的 Gateway API 访问入口承担反向代理。运行多个 Next.js 副本时，需要额外评估 ISR/数据缓存共享、缓存标签同步、`NEXT_SERVER_ACTIONS_ENCRYPTION_KEY` 和版本偏差处理，避免不同 Pod 返回不一致内容。详细行为以 [Next.js 部署文档](https://nextjs.org/docs/app/getting-started/deploying)、[standalone 输出说明](https://nextjs.org/docs/app/api-reference/config/next-config-js/output)和[自托管指南](https://nextjs.org/docs/app/guides/self-hosting)为准。
