# 平台启动问题

这里处理 Docker Compose 启动时最常见的问题。应用构建问题见[常见问题](/start/faq)。

## 使用指定版本

```bash
DEVOPS_IMAGE_TAG=v0.1.0-rc.1 docker compose up -d
```

默认使用 `nightly`。正式环境请选择已发布的固定版本。

## 端口 `8088` 被占用

查看占用：

```bash
lsof -nP -iTCP:8088 -sTCP:LISTEN
```

停止占用进程，或把 `docker-compose.yaml` 中的端口映射改为其他空闲端口，例如 `8089:8080`，然后访问新端口。

## 页面能打开，但接口请求失败

```bash
docker compose ps
docker compose logs -f api
```

确认 API、PostgreSQL 和 Redis 均为健康状态。日志中出现认证或连接错误时，检查 `.env` 中的数据库和 Redis 配置。

## 任务一直不执行

```bash
docker compose logs -f worker
```

构建和发布需要 Worker 正常运行。修复日志中的连接或配置错误后，重新启动对应服务。

仍无法解决时，请记录使用的版本、`docker compose ps` 输出和相关服务日志，再到项目仓库提交问题。提交前请移除 Token、密码和连接串中的凭据。
