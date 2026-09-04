# TODO

这里只记录尚未完成、具有明确入口和验收条件的工作。完成记录由 Git 历史保存；没有近期消费者或验收方式的构想不放入本文件。

## 登录后界面验收

- [ ] 使用有效本地账号，在桌面端、390px 移动端、暗色模式和高饱和主题下完成应用内页面视觉回归；覆盖部署配置保存与权威回读、应用市场安装、公共组件、AI 助手键盘/横竖屏/返回和模型价格提示。
- [ ] 使用平台管理员账号验收 Agent 观测 overview → turns/tools → tool-call/trace，以及工具搜索、分页、展开和 Trace 下钻。

## 应用与模板

- [ ] 在项目空间提供“从模板安装”入口；表单只展示模板 Schema 必填项，默认短名按 `{templateSlug}-{随机字符}` 生成，密码可自动生成并复制。
- [ ] 安装完成后按模板 outputs 展示内网域名、端口和建议环境变量；敏感值默认隐藏，复制操作受权限控制且不写入遥测。

## 构建、部署与集群

- [ ] 在发布日志和集群工作负载详情展示 Pod Events、重新同步入口及镜像架构不匹配提示，并使用稳定错误码映射用户文案。
- [ ] 为平台自有 executor 镜像提供可复现构建，内置 git、ca-certificates、buildctl、shell、jq 和基础诊断工具；平台构建 smoke 不再依赖第三方临时镜像内容。
- [ ] 将构建出口网络拒绝事件接入受权限控制的审计或日志视图，并验证拒绝原因不会泄露 Secret。
- [ ] 集群资源页提供集群、命名空间、项目空间、应用筛选和手动刷新；详情展示 labels/annotations 摘要、关联业务对象和 Events，始终不展示 Secret data。
- [ ] 周期发现带平台 managed labels、但权威业务对象已不存在的 Kubernetes 资源，先展示残留提示和清理建议；自动删除需另行定义审计与幂等契约。
- [ ] 使用可销毁 PostgreSQL、Kubernetes、浏览器和临时 OTel 栈完成数据卷直接传输、运行计费、构建、发布与资源页的成功/失败/取消/撤权 E2E。

## 网关与证书

- [ ] 在 HTTPS 诊断中展示 DNS-01、Certificate、Secret 和 Gateway `certificateRefs` 状态，并为不可用上游返回稳定 `observationCode`。
- [ ] 将 cert-manager HTTP-01 作为显式高级选项；启用前验证公网 port 80 listener 可达，条件不满足时拒绝保存并给出可操作提示。
- [ ] 将 Gateway `allowedRoutes.namespaces.from=All` 收紧为项目 Namespace label selector，并覆盖跨项目拒绝与同项目成功链路。

## 数据清理门禁

- [ ] 完成备份校验、历史对账和回滚演练后，通过 Contract DROP 物理删除 `retained_volumes` 及部署目标旧存储列。

## CLI 发布

- [ ] 确认 npm `@liteyuki` 组织权限，以 2FA 发布首个 `@liteyuki/luna-cli` 预发布包，再配置 Trusted Publisher 和 GitHub `npm` Environment 完成 OIDC 发布验收。
- [ ] 接入 Apple Developer ID、公证和制品验证；macOS 构建完成签名与公证后再进入稳定发布矩阵。

## Agent 安全回归

- [ ] 建立跨用户、跨项目、权限变化、提示注入、批准重放、Secret 脱敏、计费拒绝和 Agent 故障恢复的端到端测试门禁。
