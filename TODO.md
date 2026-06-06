# TODO

## 1. 文档与原型收口

- [x] 更新产品原型为文档式多页面线框。
- [x] 在原型中覆盖创建应用、构建、部署、镜像站、自定义域名、Access Token、配额页面。
- [x] 检查文档中旧的 Actions 主路径表述，确保 Actions 只作为可选 BuildProvider。

## 2. 项目基础与前后端脚手架

- [x] 初始化 Go 服务目录。
- [x] 初始化 `cmd/api` 入口。
- [x] 初始化 `cmd/worker` 入口。
- [x] 接入 Gin。
- [x] 接入 PostgreSQL。
- [x] 接入 GORM。
- [x] 接入 golang-migrate。
- [x] 接入 Redis + Asynq。
- [x] 定义 API 与 Worker 的任务投递和状态回写约定。
- [x] 建立异步任务基础队列。
- [x] 定义 OpenAPI 基础结构。
- [x] 初始化 Vite + React + TypeScript。
- [x] 接入 Tailwind CSS。
- [x] 接入 shadcn/ui。
- [x] 接入 @antfu/eslint-config。
- [x] 接入 i18next。
- [x] 接入 Sonner toast。
- [x] 为自动关闭 toast 增加倒计时进度条。
- [x] 实现 light/dark/system 主题三态。
- [x] 建立前端基础布局、路由和 API client。
- [x] 将用户信息和主题切换控件移动到侧边栏底部。
- [x] 引入前端轻量动效，覆盖页面切换、列表、弹窗和基础控件。
- [x] 将侧边栏导航改为二级分组结构，按 DevOps、个人工作区、系统管理分栏展示。
- [x] 使用浏览器验收前端启动、主题切换和基础路由。
- [x] 开发环境使用 Vite proxy 反代后端 API。
- [x] 为 api、worker、web 编写 Dockerfile。
- [x] 提供完整 docker compose 运行编排。

## 3. 认证、权限与登录

- [x] 实现本地账号登录。
- [x] 实现管理员创建、邀请或导入本地账号。
- [x] 接入通用 OIDC。
- [x] 支持 Casdoor OIDC 配置。
- [x] 实现 AuthProvider。
- [x] 实现 AuthAdmissionPolicy。
- [x] 支持后台配置多个 OIDC Provider。
- [x] 实现 OIDC 外部身份绑定 ExternalIdentity。
- [x] 支持 OIDC 通过非空已验证邮箱绑定现有用户。
- [x] 支持登录态下绑定和解绑第三方登录。
- [x] 实现运行模式检测，开发模式支持开发账号快捷登录，生产模式禁用。
- [x] 收紧开发默认账号提示边界，仅 development 模式由后端下发并展示。
- [x] 实现生产模式首个平台管理员初始化流程。
- [x] 禁止开放自由注册。
- [x] 支持 OIDC 允许组白名单。
- [x] 支持可配置 OIDC group claim。
- [x] 支持邮箱域白名单和邀请邮箱白名单。
- [x] 准入失败记录 AuditLog。
- [x] 实现统一 AuthErrorPage。
- [x] 实现统一 ForbiddenPage。
- [x] 实现 OIDC state 错误、组白名单不匹配、账号未邀请、权限不足的友好错误展示。
- [x] 建立 User、Project、ProjectMember。
- [x] 实现 Owner/Admin/Developer/Viewer 角色。
- [x] 实现权限点校验。
- [x] 实现 Access Token 创建、hash 存储和撤销。
- [x] 实现 Access Token scope 校验。
- [x] 实现登录页。
- [x] 实现当前用户和基础权限状态管理。
- [x] 实现用户语言偏好保存和前端 i18n 同步。
- [x] 实现项目成员权限状态管理。
- [x] 将 Access Token 管理合并到账号安全页，作为账号安全的子内容块。
- [ ] 抽离统一分页组件，并将列表 API 改造为支持分页、排序、搜索和可选批量选择。
- [x] 使用浏览器验收本地登录、退出和 Access Token 创建/撤销流程。
- [x] 使用浏览器验收权限隐藏流程。

## 4. 项目、应用与前端主工作区

- [x] 实现 Project CRUD。
- [x] 修复 Project 软删除后 slug 唯一索引仍占用的问题，改为未删除项目唯一。
- [x] 实现 Project namespaceStrategy。
- [x] 实现 Application CRUD。
- [x] 支持 sourceType: repository。
- [x] 支持 sourceType: image。
- [x] 实现 .devops/app.yaml 读取和解析。
- [x] 实现项目页。
- [x] 实现应用页。
- [x] 实现创建应用向导。
- [x] 实现应用配置页。
- [x] 抽离可复用 PageHeader、EmptyState、ErrorState、StatusBadge。
- [x] 实现可复用 ConfirmDialog。
- [x] 实现公开站点配置 KV 读取接口。
- [x] 实现站点配置动态 KV 表单。
- [x] 支持自定义站点 title、logo、favicon、登录页副标题。
- [x] 使用浏览器验收站点设置保存、公开配置刷新和语言切换流程。
- [x] 使用浏览器验收项目页、应用页和 sourceType 切换流程。
- [x] 使用 PostgreSQL 集成环境验收项目创建和应用创建流程。

## 5. Git 集成

- [ ] 实现 GitProvider。
- [ ] 实现 GitAccount。
- [ ] 支持 Gitea OAuth。
- [ ] 支持 GitHub OAuth 或 GitHub App。
- [ ] 实现 RepositoryBinding。
- [ ] 创建 Git webhook。
- [ ] 校验 webhook 签名。
- [ ] 处理 push/tag webhook 事件。

## 6. 镜像站

- [ ] 实现 ArtifactRegistry。
- [ ] 支持 global/project/user scope。
- [ ] 实现 RegistryCredential 加密引用。
- [ ] 实现 registry 凭据测试。
- [ ] 实现默认镜像站选择优先级。
- [ ] 实现 ContainerImage 记录。

## 7. 平台构建

- [ ] 实现 BuildProvider 接口。
- [ ] 实现 BuildRun。
- [ ] 实现 BuildJob。
- [ ] 实现构建队列。
- [ ] 创建 build namespace。
- [ ] 实现 Kubernetes Job 构建。
- [ ] 使用 BuildKit rootless。
- [ ] 注入 Git 和 registry Secret。
- [ ] 实现 BuildNetworkPolicy。
- [ ] 实现 NetworkPolicyProvider 抽象。
- [ ] 为构建 Job 生成 restricted 网络出口策略。
- [ ] 支持公开 Git、公开 registry、公开包管理源访问。
- [ ] 支持内网 registry/镜像源 TCP 443 白名单访问。
- [ ] 禁止私有网段非 443 端口访问。
- [ ] 禁止元数据地址、Kubernetes API Server 和 Service CIDR 访问。
- [ ] 为构建网络拒绝事件记录审计日志。
- [ ] 记录构建日志。
- [ ] 记录 image tag 和 digest。
- [ ] 实现 Job 超时、重试和清理策略。
- [ ] 记录 CPU、内存和 credit 消耗。
- [ ] 预留 cache 配置字段。

## 8. 集群与部署

- [ ] 实现 RuntimeCluster。
- [ ] 支持设置默认集群。
- [ ] 测试 kubeconfig。
- [ ] 创建 Project namespace。
- [ ] 实现 Environment。
- [ ] 实现 Deployment/Service/ConfigMap/Secret apply。
- [ ] 实现 rollout 状态等待。
- [ ] 实现 Release 记录。
- [ ] 实现回滚到上一成功版本。

## 9. 网关与域名

- [ ] 实现 GatewayRoute。
- [ ] 默认域名 `{appSlug}-{projectSlug}.{stage}.{rootDomain}`。
- [ ] 检查域名冲突。
- [ ] 创建 Ingress。
- [ ] 支持自定义域名。
- [ ] 生成 CNAME 目标。
- [ ] 校验 DNS CNAME。
- [ ] 支持 HTTP Challenge 证书申请。
- [ ] 支持 HTTP-only 访问。

## 10. 前端联调验收

- [ ] 实现仓库绑定页，并与 Git 集成联调。
- [ ] 实现构建页，并与 BuildRun 状态和日志联调。
- [ ] 实现镜像站页，并与 ArtifactRegistry 联调。
- [ ] 实现部署环境页，并与 Environment/Release 联调。
- [ ] 实现网关域名页，并与 GatewayRoute/证书状态联调。
- [ ] 使用浏览器验收仓库绑定、构建、镜像站、部署、域名完整链路。
