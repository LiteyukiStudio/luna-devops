# 二进制部署

只有在调试、离线排障或特殊环境验证时，才建议直接运行二进制。正式使用请优先选择 [Kubernetes (Helm) 部署](/start/install/kubernetes) 或 [Docker Compose 部署](/start/install/docker-compose)。

直接运行二进制意味着 PostgreSQL、Redis、进程守护、日志和升级都要自行维护；容器化部署已经处理了其中大部分重复工作。

## 开始前准备

你需要：

- PostgreSQL。
- Redis。
- API 和 worker 二进制文件。
- API 使用的环境变量。
- 一个进程管理器，例如 systemd、supervisor 或你自己的启动脚本。

## 构建二进制

在仓库根目录执行：

```bash
go build -o bin/luna-devops-api ./cmd/api
go build -o bin/luna-devops-worker ./cmd/worker
```

API 会同时提供控制台页面，不需要另行启动前端服务。

## 准备配置

至少需要准备 PostgreSQL、Redis、公开访问地址、密钥加密 key 和首个管理员配置：

```bash
export APP_ENV=production
export DATABASE_URL='postgres://devops:password@127.0.0.1:5432/devops?sslmode=disable'
export REDIS_ADDR='redis://default:password@127.0.0.1:6379/0'
export PUBLIC_BASE_URL='https://devops.example.com'
export SECRET_ENCRYPTION_KEY='replace-with-a-stable-random-value'
export INITIAL_ADMIN_EMAIL='admin@example.com'
export INITIAL_ADMIN_PASSWORD='replace-with-a-strong-8-to-72-byte-password'
export INITIAL_ADMIN_NAME='Platform Admin'
export INITIAL_ADMIN_LANGUAGE='zh-CN'
```

`SECRET_ENCRYPTION_KEY` 用于加密 Git、镜像站和 OIDC 密钥。部署后不要随意修改，否则旧密钥可能无法解密。

`INITIAL_ADMIN_*` 只供 API 在全新数据库中创建首个管理员。已有有效管理员时不会覆盖账号或密码；数据库已有用户但没有有效管理员时，API 会拒绝启动并要求先恢复管理员。Worker 不需要、也不应接收管理员密码。

## 启动 API

```bash
./bin/luna-devops-api
```

默认监听端口以项目配置为准。确认健康检查：

```bash
curl http://127.0.0.1:8080/healthz
```

如果需要公网访问，建议在 API 前面放 Caddy、Nginx 或其他反向代理，再把 `PUBLIC_BASE_URL` 设置成用户实际访问地址。

## 启动 worker

另开一个进程启动 worker：

```bash
./bin/luna-devops-worker
```

构建和发布需要 worker 正常运行；只启动 API 无法完成交付流程。

## 建议的 systemd 形态

下面是一份起点配置，不应原样用于所有服务器。请按实际安装路径和环境变量调整：

```ini
[Unit]
Description=Luna DevOps API
After=network.target postgresql.service redis.service

[Service]
WorkingDirectory=/opt/luna-devops
Environment=APP_ENV=production
Environment=DATABASE_URL=postgres://devops:password@127.0.0.1:5432/devops?sslmode=disable
Environment=REDIS_ADDR=redis://default:password@127.0.0.1:6379/0
Environment=PUBLIC_BASE_URL=https://devops.example.com
Environment=SECRET_ENCRYPTION_KEY=replace-with-a-stable-random-value
Environment=INITIAL_ADMIN_EMAIL=admin@example.com
Environment=INITIAL_ADMIN_PASSWORD=replace-with-a-strong-8-to-72-byte-password
Environment=INITIAL_ADMIN_NAME=Platform Admin
Environment=INITIAL_ADMIN_LANGUAGE=zh-CN
ExecStart=/opt/luna-devops/bin/luna-devops-api
Restart=always

[Install]
WantedBy=multi-user.target
```

worker 复用数据库、Redis、公开地址和加密密钥配置，但不要下发 `INITIAL_ADMIN_*`。将 `ExecStart` 改成：

```ini
ExecStart=/opt/luna-devops/bin/luna-devops-worker
```

## 升级

1. 停止 API 和 worker。
2. 替换二进制文件。
3. 确认环境变量没有丢失。
4. 启动 API 和 worker。
5. 打开控制台并检查 worker 日志。

## 什么时候不要用

- 你不想手动维护 PostgreSQL 和 Redis。
- 你希望升级、回滚和日志收集更简单。
- 你还不确定平台配置。
- 你只是想先体验产品。

这些场景更适合容器化部署。
