# 部署上线一个 Web 项目

这是一条可以从头照着做的部署流程。我们用 GitHub 仓库 [`snowykami/neo-blog`](https://github.com/snowykami/neo-blog) 做例子，把博客服务从源码构建成镜像，发布到集群，再为前端创建访问入口。

`neo-blog` 是一个前后端分离项目：

| 服务 | 目录 | Dockerfile | 端口 | 说明 |
| --- | --- | --- | ---: | --- |
| 前端 | `web/` | `web/Dockerfile` | `3000` | Next.js，对外访问入口指向它。 |
| 后端 | 仓库根目录 | `Dockerfile` | `8888` | Go/Hertz，只给前端和集群内访问。 |
| 数据 | 后端数据目录 | 后端数据卷 | - | 按项目要求配置持久化存储。 |

开始前先确认三件事，缺一项都可能让后面的步骤卡住：

- 平台 API 和 worker 都在运行。
- 已经配置可用的运行集群。
- 已经配置镜像站，并且构建任务有权限推送镜像。

## 1. 创建项目空间

进入“项目空间”，创建一个新的空间。这个空间用来放博客的前端、后端、数据配置和访问入口。

![创建项目空间](/guide/deploy-web-project/01-project-space.svg)

建议这样填：

| 字段 | 示例 | 说明 |
| --- | --- | --- |
| 名称 | `Neo Blog` | 页面展示用，写人能看懂的名字。 |
| 标识 | `neo-blog` | 用英文、小写和短横线。 |
| 成员 | 先保留自己 | 跑通后再邀请团队成员。 |

创建完成后，进入这个项目空间继续操作。

## 2. 创建应用

在项目空间里创建两个应用：

![创建应用](/guide/deploy-web-project/02-applications.svg)

| 应用 | 标识 | 用途 |
| --- | --- | --- |
| `Neo Blog Frontend` | `neo-blog-frontend` | 对外访问的前端服务。 |
| `Neo Blog Backend` | `neo-blog-backend` | 提供 API 和后台逻辑。 |

数据服务和持久化方式按项目自身要求配置，不需要为每个数据卷创建应用。

## 3. 绑定 GitHub 仓库

进入前端或后端应用的仓库设置，选择 GitHub，绑定：

```text
snowykami/neo-blog
```

![绑定 GitHub 仓库](/guide/deploy-web-project/03-repository.svg)

建议先确认：

| 项 | 建议 |
| --- | --- |
| 默认分支 | `main` |
| Webhook | 开启，后续 push 可以自动触发构建。 |
| 仓库权限 | 至少能读取源码；如果要自动配置 Webhook，需要对应写权限。 |

同一个仓库只需要绑定一次。前端和后端部署配置都可以引用这个仓库，只是 Dockerfile 和构建上下文不同。

## 4. 创建后端部署配置

先创建后端，因为前端需要知道后端地址。

| 字段 | 示例 |
| --- | --- |
| 应用 | `neo-blog-backend` |
| 来源 | 从仓库构建 |
| 仓库 | `snowykami/neo-blog` |
| Dockerfile | `Dockerfile` |
| 构建上下文 | `.` |
| Dockerfile Build Args | 可选，例如 `EMBED_WEB=true` |
| 服务端口 | `8888` |
| 镜像仓库 | 选择你的推送镜像站 |
| 镜像标签 | `latest` 或 `${GIT_SHA}` |
| 副本数 | `1` |
| CPU / 内存 | 第一次可用 `1C / 1Gi` |

按项目说明填写运行模式、端口、公开地址和持久化目录。密码、Token 和签名密钥必须使用 Secret，并在上线后保持稳定。

## 5. 创建前端部署配置

前端使用仓库里的 `web/Dockerfile`。

![创建部署配置](/guide/deploy-web-project/04-deployment-targets.svg)

| 字段 | 示例 |
| --- | --- |
| 应用 | `neo-blog-frontend` |
| 来源 | 从仓库构建 |
| 仓库 | `snowykami/neo-blog` |
| Dockerfile | `web/Dockerfile` |
| 构建上下文 | `web` |
| Dockerfile Build Args | 可选，例如 `VERSION=${{ github.sha }}` |
| 服务端口 | `3000` |
| 镜像仓库 | 选择你的推送镜像站 |
| 镜像标签 | `latest` 或 `${GIT_SHA}` |
| 副本数 | `1` |

构建参数只用于非敏感值；密码、Token 和私有包凭据应放在项目空间的密钥构建变量中。

前端环境变量建议设置：

| 变量 | 示例 | 说明 |
| --- | --- | --- |
| `BACKEND_URL` | `http://neo-blog-backend:8888` | 前端服务端访问后端用。实际服务名以平台下发的 Service 为准。 |
| `NODE_ENV` | `production` | 前端生产模式。 |

如果平台展示的后端服务名不是 `neo-blog-backend`，用部署后的 Service 名替换 `BACKEND_URL`。

## 6. 触发构建并发布

先构建后端，再构建前端。

![触发构建并发布](/guide/deploy-web-project/05-build-release.svg)

推荐顺序：

1. 在后端部署配置里点击“触发构建”。
2. 等构建状态变成成功。
3. 创建后端 Release，等待运行状态正常。
4. 在前端部署配置里点击“触发构建”。
5. 等前端构建成功后创建 Release。
6. 打开应用部署列表，确认前端和后端都处于正常状态。

如果构建失败，先查看构建日志末尾，再参考[常见问题](/start/faq)。

## 7. 创建访问入口

只给前端创建访问入口。后端保持集群内访问即可。

![创建访问入口](/guide/deploy-web-project/06-gateway.svg)

| 字段 | 示例 |
| --- | --- |
| 应用 | `neo-blog-frontend` |
| 部署配置 | 前端部署配置 |
| 域名 | `blog.example.com` |
| 路径 | `/` |
| 服务端口 | `3000` |
| TLS | 按你的网关和证书策略选择 |

创建后，等访问入口状态正常，再打开：

```text
https://blog.example.com
```

如果页面能打开，但登录、文章或接口请求异常，优先检查前端的 `BACKEND_URL` 和后端的 `BASE_URL`。

## 8. 确认跑通后再打开自动化

第一次上线建议手动构建、手动发布。确认没有问题后，再逐步打开：

- 仓库 Webhook 自动触发构建。
- 构建成功后自动发布。
- 分支匹配，例如只让 `main` 自动发布生产环境。
- 标签匹配，例如 `v*` 发布稳定版本。

逐项开启后，每一步出了问题都容易定位，回滚也更清楚。
