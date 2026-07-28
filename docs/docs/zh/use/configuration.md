# 配置与连接

这一页说明平台如何连接 Git、镜像站和运行集群，以及哪些设置会影响安全和控制台展示。配置大致分为两类：可以公开给用户看的站点信息，以及只在后端使用的外部系统连接。

## 公开配置

公开配置决定控制台显示什么，例如：

- 站点标题。
- Logo 和 Favicon。
- 登录页副标题。
- 主题和语言偏好。

这些内容可以放心展示给前端，但不要放 Token、密码或内部地址。

## 注册与邮件

“站点设置 → 注册与邮件”提供两个彼此独立的注册开关。邮箱注册默认关闭；启用后，新用户需要先接收并填写一次性邮箱验证码。OIDC 注册默认开启；关闭后只禁止未绑定身份的用户首次进入平台，不影响已有用户继续通过 OIDC 登录。

邮箱注册依赖 SMTP 主机、端口、传输安全、账号、发件地址和发件名称。SMTP 密码是只写字段：保存后仅显示“已设置”，真实值写入 Secret Store，不会落入普通配置响应。修改其他邮件配置时留空密码即可保留原值。

“允许无密码账号设置本地密码”默认关闭。开启后，通过 OIDC 创建且尚无密码的用户可以在“账号 → 安全设置”补设密码。已有密码的用户修改密码时必须验证当前密码；任何成功的密码设置或修改都会撤销该账号现有会话。

全局构建环境位于单独的“构建”标签页，不属于品牌信息。注册、邮件和认证策略位于“注册与邮件”，MFA 等敏感操作策略继续位于“安全策略”。

### 品牌主题色

平台管理员可以在“站点设置 → 品牌信息”的第一项选择全站默认配色主题。用户可以在“账号 → 个人设置”中继续跟随平台，或者选择自己的主题。个人设置优先于站点默认；用户选择“跟随平台”时，账号中保存的是空偏好而不是当前主题快照，因此管理员以后修改默认主题会自动影响这些用户。多色主题组合主色、辅助色、支持色和强调色，单色主题则让这些角色保持同一色相；成功、警告、失败等语义状态色始终独立。亮色、暗色或跟随系统的明暗模式也集中在“账号 → 个人设置”中选择，不需要分别维护两套配色主题。

平台提供 Aurora、Harbor、Sunset、Botanical、Meadow、Citrus 六套精选多色主题。主题选择器另外保留 Gold、Orange、Red、Pink、Violet、Blue、Cyan、Teal、Green、Lime 十套来自 [Radix Colors 3.0.0 官方品牌色阶](https://www.radix-ui.com/colors/docs/palette-composition/composing-a-palette) 的单色代表主题，每个主要色相只保留一套；旧账号已经保存的其他官方单色预设仍可继续使用和修改，但不再作为推荐项铺满选择器。平台不接受任意 HEX 或自定义 CSS。

每个色阶的组件背景、悬停、边框、焦点、实心控件和文本分别使用 Radix 定义的对应步骤。多色主题还把辅助色、支持色和强调色用于导航选中面以及列表结构线，大面积工作区背景保持低饱和单色平面；状态色保持独立。官方 CSS 同时提供 sRGB 和 Display-P3 值，浏览器会按屏幕能力选择。生产环境返回控制台首页时，API 会把站点默认预设写入 HTML；首屏脚本在 React 渲染前读取当前账号缓存的个人偏好，并按“个人偏好 → 站点默认 → Blue”应用主题。登录后再以后端用户记录校准缓存，退出账号后恢复站点默认，因此刷新和正常登录态下不会先显示错误颜色再切换。

### 简约模式

平台管理员可以在“站点设置 → 品牌信息”中设置“默认启用简约模式”。标准模式保留低饱和品牌色画布；简约模式将页面背景、透明侧边栏及导航弱表面恢复为中性白/灰色，主按钮、链接、焦点、选中指示和成功/警告/失败状态色不受影响。

用户可以在“账号 → 个人设置”选择“跟随平台”“标准模式”或“简约模式”。跟随平台会保存为空偏好，因此平台管理员以后修改默认开关时会自动生效；显式选择的个人模式始终优先于平台默认。明暗主题与界面风格相互独立，简约模式同样支持浅色、深色和跟随系统。

## 安全策略

生产模式下，API 会为控制台响应增加基础安全响应头，包括 `Content-Security-Policy`、`X-Content-Type-Options`、`X-Frame-Options`、`Referrer-Policy` 和 `Permissions-Policy`。CSP 默认只允许同源脚本、manifest 和连接，禁止插件对象，允许 Tailwind/shadcn 所需的 inline style、`data:` 字体和图片，以及 HTTPS 图片资源。

`Strict-Transport-Security` 只建议在生产 HTTPS 环境开启。平台默认在 `APP_ENV=production` 时启用，也可以通过 `APP_ENABLE_HSTS=true` 显式开启，或用 `APP_ENABLE_HSTS=false` 关闭。不要在仍需 HTTP 访问的本地或测试域名上开启 HSTS。

敏感操作二次验证由站点配置 `security.stepUpMfa.enabled` 控制，默认关闭。安全配置在每次判断时从共享 PostgreSQL 读取，因此多个 API 副本会使用同一策略；数据库读取失败时按“启用 MFA + 更短超时”处理，不会回退成免验证。启用前，至少一名可用的平台管理员必须先在“账号安全”绑定离线 TOTP 验证器；全局策略开启后，最后一名已绑定 MFA 的平台管理员不能解绑、被禁用或降级。策略修改、管理员解绑/重置及管理员账号状态变更会争用同一个 PostgreSQL 事务锁，并在锁内通过当前事务重新读取策略和复核 actor、session、Step-up assertion 与可用管理员，避免等待锁的旧请求继续执行，也避免并发请求留下“策略已开启但无人可验证”的状态。`security.stepUpMfa.idleTimeoutMinutes` 控制没有继续执行敏感操作时的空闲有效期，默认 10 分钟；`security.stepUpMfa.absoluteTimeoutMinutes` 控制一次验证无论是否持续活动都不能超过的绝对有效期，默认 60 分钟。

站点设置只提交实际修改过的字段。后端也会比较安全策略的当前值，只有 `security.stepUpMfa.*` 真正变化时才触发 `security_settings_update` 二次验证；修改品牌、运营面板等普通配置不会因为表单中携带未变化的安全字段而被误判。

用户开始绑定前必须再次证明主身份：本地账号输入当前密码，OIDC 账号必须在 5 分钟内完成过主认证并且不是管理员模拟登录；remember token 恢复只创建新 session，不会刷新这项主认证时间。通过后页面会显示二维码、完整 `otpauth` URI 和手动密钥。只有输入有效的 6 位 TOTP 后才会正式启用；校验接受当前 30 秒窗口及前后各一个窗口，但同一时间步及更早的验证码不能重复使用。邮箱注册和 MFA 使用同一套六位分格验证码输入，支持整串粘贴、移动端数字键盘和系统或密码管理器的一次性验证码填充。确认时平台生成 10 个一次性恢复码，明文只展示一次，后端只保存 bcrypt hash。每个恢复码只能成功使用一次，重新生成会立即作废全部旧码。TOTP secret 进入平台加密 Secret 存储，不落业务表明文，也不会提供给管理员查看。

开启全局策略后，Web Console、运行命令、数据导出、登录密码修改、个人访问令牌创建或撤销、密钥和镜像站凭据写入、kubeconfig 更新、认证源更新、平台管理员账号变更及安全策略修改会检查当前浏览器会话与具体 purpose 的 Step-up assertion。个人访问令牌有效期由后端强制限制为 7、15、30、90 天或无限有效，不能通过绕过前端提交其他值。权限边界明确的全局管理接口通过“认证、平台管理员授权、Step-up MFA”路由中间件声明保护；依赖项目角色、资源归属或请求内容的操作仍在完成业务授权后调用同一 Step-up guard，避免二次验证替代资源权限判断。未通过时 API 返回 `mfa_required`，控制台弹出统一验证码/恢复码 Dialog，并在验证成功后自动重试原操作。断言按用户、session 和 purpose 存在共享数据库中；成功执行同类敏感操作会刷新空闲时间，但不会延长绝对有效期。个人令牌不能完成 MFA，也不能替代这些交互式会话检查。

Git、OIDC、Registry 和通知 Webhook 等后端出站 HTTP 请求在连接阶段解析目标域名，先校验全部解析 IP，再直接连接同一批已验证 IP；不会在安全校验后用原始域名再次解析，因此 DNS 重绑定不能把已经通过校验的公网域名切换到回环、云元数据或集群私网地址。域名白名单仍表示管理员明确允许该域名访问其解析地址。

MFA 绑定、确认和验证按用户与来源 IP 限流。绑定最多连续尝试 10 次/小时，确认和敏感操作验证最多连续尝试 20 次/5 分钟；成功后会立即清空当前用户对应操作的计数。来源 IP 使用独立的高阈值，避免同一办公室或网关出口下的正常用户互相影响。验证码和恢复码不会写入日志。使用恢复码、重新生成恢复码、绑定、解绑、策略更新、管理员重置以及验证成功或失败都会写入审计日志。平台管理员重置他人 MFA 前也必须完成 `user_admin_update` 二次验证；不能通过该入口重置自己，也不能在全局策略开启时移除最后一名已绑定 MFA 的可用管理员。账号密码、角色或禁用状态变化会撤销现有会话、remember token 和 Step-up assertion；解绑或重置 MFA 也会删除 TOTP secret、恢复码及当前断言。

## Git Provider

Git Provider 用来连接 GitHub 或 Gitea。配置完成后，用户可以绑定仓库、接收 Webhook，并按分支或标签触发构建。

Git 凭据的“作用域”是平台内的使用边界：个人级仅创建者可用，项目级可供所选项目空间成员在平台任务中使用，全局级可供所有项目使用。“Scopes”则是 GitHub、Gitea 或 GitLab 授予 Token 的上游 API 权限，例如 `repo`、`read:user`，不会改变凭据在 Luna DevOps 内的归属范围。创建长期凭据时，两层权限都应遵循最小权限原则。

Git 凭据创建后仍可修改作用域和项目空间绑定，但使用范围不能超过所属 Git Provider：个人 Provider 只能创建个人凭据，项目 Provider 只能创建个人或其绑定项目范围内的凭据，全局 Provider 才能创建全局凭据。编辑时 Access Token 和 Refresh Token 留空会保留原值。

删除 Git Provider 时，平台会同步删除属于该 Provider 的 Git 凭据。删除前请确认相关仓库绑定和构建链路不再依赖这些凭据。

如果只是想先验证部署链路，可以暂时跳过 Git Provider，直接使用已有镜像。等应用正常运行后再接仓库，问题会更容易定位。

## 镜像站

镜像站负责保存构建产物，也为运行集群提供要拉取的镜像。常见选择包括 Harbor、Gitea Registry、DockerHub 和通用 OCI / Docker Registry。

通用 OCI 镜像站走标准 Docker Registry HTTP API V2：平台会用 `/v2/` 测试连通性，用 `_catalog` 搜索仓库，用 `tags/list` 读取 tag。部分镜像站会关闭 catalog 列表权限，这时搜索可能不可用，但仍可以手动填写仓库路径和 tag。

删除镜像站时，平台会同步删除属于该镜像站的凭据。删除前请确认部署配置、构建任务或运行集群拉取镜像不再依赖这些凭据。

镜像站凭据使用两组独立设置：`usage` 决定用于拉取、推送或两者兼具；`scope` 决定谁能在平台任务中使用，支持个人、多个项目空间或全局。凭据范围不能超过所属镜像站，密钥不会因共享而回显给其他用户。

需要自动构建时，部署配置会使用镜像站推送凭据；只部署已有镜像时，重点确认运行集群能拉取目标镜像。

创建发布时，平台会优先按部署配置里的目标镜像站和仓库实时读取 tag；如果镜像站接口不可用、凭据权限不足或仓库关闭 tag 列表能力，发布弹窗会回退到平台保存的成功构建记录。平台保存的构建记录只说明“曾经构建并推送成功”，不保证上游镜像站仍保留该 tag；如果 registry 开启自动清理，发布或回滚前应确认已发布版本对应的镜像未被删除。

部署配置支持 Dockerfile Build Args。用户可以按 `KEY=value` 每行填写一个 Dockerfile `ARG`，平台会在触发构建时把当前配置保存到 BuildRun，并传给 BuildKit。Build Args 支持和镜像 Tag 一致的构建时模板：`${{ github.sha }}`、`${{ github.ref_name }}`、`${{ github.ref_type }}`、`${{ github.ref }}` 和 `{short_sha}`。Build Args 会进入构建参数和构建记录，不适合保存密钥；敏感值请使用项目空间构建变量中的密钥。

构建环境变量和密钥支持四级配置，构建时按 `全局 -> 项目空间 -> 应用 -> 部署配置` 合并，同名键由更靠后的级别覆盖：

- 全局级由平台管理员在“站点设置”中维护，适合统一的镜像源或构建工具默认值。
- 项目空间级继续使用项目工作台中的“构建变量”，可以组合多个变量集。
- 应用级在应用“构建”页面维护，适合同一应用所有部署配置共享的值。
- 部署级在部署配置的构建区域维护，适合环境或部署配置独有的值。

变量和密钥会在创建 BuildRun 时保存快照，重试同一构建时继续使用原快照，之后修改配置不会改变已有构建。普通变量随构建记录保存；密钥只保存加密存储引用，API 只返回“已设置”状态，Worker 执行构建时才解析明文。

镜像站凭据可以配置“镜像仓库模板”和“镜像 Tag 模板”。它们只用于创建部署配置时填充默认推送位置；部署配置保存后会把仓库和 Tag 保存为快照，不会继续跟随凭据模板变化。常用写法例如仓库模板 `devopsns/{projectIdentifier}-{appIdentifier}-{stage}`，Tag 模板 `{appIdentifier}-{stage}`，新建部署配置时会得到类似 `devopsns/blog-api-prod:api-prod` 的默认镜像引用。

仓库模板和 Tag 模板都只渲染创建部署配置时已知的静态变量：`{registryNamespace}`、`{project}`、`{projectIdentifier}`、`{app}`、`{appIdentifier}`、`{application}`、`{applicationIdentifier}`、`{stage}`、`{target}`。`{project}` 与 `{app}` 是对应不可变标识的简写。如果 Tag 模板使用 `{commit}`、`{branch}` 等构建时变量，创建部署配置时会回落为 `latest`，避免后续构建被凭据模板隐式改写。

## 运行集群

运行集群是发布目标。平台会把 Release 转换成 Kubernetes 资源，并把状态、日志和诊断信息展示回来。

项目空间、应用和部署阶段都有创建后不可修改的标识。项目空间和应用标识长度为 2–22 个字符，阶段标识长度为 2–12 个字符；只能使用小写英文字母、数字和短横线，并且必须以字母或数字开头和结尾。显示名称仍然可以修改，也可以使用中文。

新创建的数据使用可读且稳定的 ID 与 Kubernetes 名称：

| 对象 | 示例 |
| --- | --- |
| 项目空间 `payments` | ID `prj_payments`，Namespace `luna-payments` |
| 项目中的应用 `api` | ID `app_payments_api` |
| 应用的生产部署 `prod` | ID `dplt_payments_api_prod` |
| Deployment / StatefulSet / Service / HPA 基名 | `luna-api-prod` |
| ConfigMap / Secret | `luna-api-prod-config` / `luna-api-prod-secret` |

平台把 Kubernetes 名称单独保存，不会在显示名称变化时重命名资源。升级前已经存在的项目空间和部署配置继续使用原来的 `ns-*`、`dplt-*` 名称，避免升级时重建工作负载或中断访问。

升级时，平台会把历史镜像凭据模板中的 `{projectSlug}`、`{appSlug}`、`{applicationSlug}` 和 `{targetSlug}` 自动迁移为对应的不可变标识变量。已经生成并保存到部署配置、构建记录和镜像记录中的实际镜像地址不会被改写。

运行集群也维护访问入口的可用域名后缀、外层访问协议、外层访问端口和 Gateway API 默认值。一个集群可以配置多个域名后缀；创建访问入口时，用户从部署配置所属集群的后缀中选择一个。访问入口按所选后缀生成默认域名、补全短域名前缀，并返回控制台访问链接；因此多个集群可以分别接入不同 GatewayClass、共享 Gateway 或不同根域名，同一集群也可以同时提供公网、内网或不同业务线域名。

网关配置分为“对外展示”和“集群内部”两层：

- 外层访问协议和外层访问端口只影响控制台生成的访问 URL。HTTP `80` 和 HTTPS `443` 不会显示在访问地址里；如果配置为非标端口，访问地址会显示 `:端口`。它们不会修改 Kubernetes Gateway listener，也不会触发证书申请。
- Gateway listener 名称和端口是集群内部 Gateway/Controller 承接流量的配置，默认 `web:8080` 和 `websecure:8443`。普通项目用户不需要选择端口或 listener。

访问入口默认绑定哪个 listener 由外部 TLS 模式决定：选择“Gateway 终止 TLS”时绑定 HTTPS listener，例如 `websecure`；选择“上游代理已终止 TLS”时绑定 HTTP listener，例如 `web`，因为进入 Gateway 的流量已经是明文 HTTP。HTTPS listener 始终按 HTTPS/TLS 入口生成，用于匹配 Traefik `websecure` 这类启用 TLS 的 entryPoint。

需要由集群内 Gateway 终止 HTTPS 时，管理员可以在运行集群上配置已有 Kubernetes TLS Secret 的名称和命名空间。平台会把该 Secret 作为 HTTPS listener 的 `tls.certificateRefs` 写入共享 Gateway；Secret 内容需要由管理员或外部证书同步系统提前创建，平台不会在这个手动模式里保存证书私钥或自动签发证书。TLS Secret 命名空间留空时默认引用 Gateway 所在命名空间；跨命名空间引用可能还需要按 Gateway API 要求配置 `ReferenceGrant`。

如果访问入口选择“HTTP Challenge 证书”，平台会按运行集群的 cert-manager 配置创建 `Certificate`，并把证书 Secret 追加到共享 Gateway 的 HTTPS listener。运行集群可配置 Issuer 类型、Issuer 名称和证书命名空间；留空 Issuer 名称时使用 Worker 默认 `CERT_MANAGER_CLUSTER_ISSUER`。当前阶段只负责创建 Certificate、读取 Ready 状态并引用 Secret，HTTP-01 是否可达仍取决于集群里的 cert-manager solver、Gateway HTTP listener 和公网 80 入口配置。

没有公网 `80` 入口或希望统一使用通配符证书时，可以在运行集群启用 DNS-01 通配符证书。平台会创建包含根域名和 `*.根域名` 的 cert-manager `Certificate`，并把输出 Secret 引用到 Gateway HTTPS listener。DNS API 凭据、DNS-01 solver 和 ACME 账号仍由所选 Issuer / ClusterIssuer 维护，平台不会保存 DNS Provider 凭据。

如果外层 Nginx/CDN/负载均衡器已经占用宿主机 `80/443`，可以让它转发到集群 Gateway 的内部端口：上游已终止 TLS 时转到 HTTP listener，例如 `8080`；需要 Gateway 终止 TLS 时转到 HTTPS listener，例如 `8443`。平台生成的访问地址仍按运行集群的外层访问协议和外层访问端口展示。

集群资源页会分页展示平台管理的命名空间、工作负载、服务、配置、密钥和存储资源；分页总数只统计当前用户有权查看的资源。工作负载页以 Deployment 为主行，展开后展示该 Deployment 下的 Pod，Pod 子行不参与分页计数。

平台管理员可以在工作负载页的 Pod 子行打开 Web Console，直接进入该 Pod 的交互式终端。前端会先通过普通 HTTP API 调用 Pod terminal authorize 预检；如果返回 `mfa_required`，统一 MFA Dialog 可以完成 `runtime_terminal` 验证并自动重试。预检通过只表示可以继续尝试连接，随后 WebSocket 端点会在升级前重新校验，并在连接期间每 3 秒复核会话、平台管理员角色、MFA assertion、Pod 身份和平台资源归属；任一条件失效都会主动结束 shell。只有真实终端输入会节流刷新空闲期限，窗口缩放、ping 和后台轮询不会保持会话活跃，绝对期限始终不会延长。终端操作会写入审计日志。

如果 API 或 worker 在容器里运行，kubeconfig 里的地址必须能从容器访问，不要直接使用宿主机专用的 `127.0.0.1`。平台只接受 HTTPS API Server 和内联的 CA、客户端证书/私钥或 Token；会拒绝 `exec` credential plugin、`auth-provider`、`tokenFile`、`proxy-url` 以及本机证书文件路径。请先运行 `kubectl config view --raw --minify --flatten` 再保存，避免平台进程执行外部命令或读取宿主机文件。

运行集群也承担 Kubernetes 构建 Job。小团队默认每个运行集群最多同时运行 4 个构建 Job；项目空间默认最多同时运行 2 个构建。超过额度时，新构建会保持排队并自动重试，不会立刻标记为失败。

## 个人令牌

个人令牌用于脚本、CI 或外部自动化调用平台 API。Token 明文只会在创建后展示一次，后端只保存 hash；撤销后会立即失效并从列表中隐藏。

创建令牌时可以勾选多个权限范围。权限目录由后端统一下发，前端会定期同步，因此后续新增或调整权限粒度时不需要在页面中维护硬编码列表。普通用户只能创建读类权限和明确的自动化触发权限，例如读取项目空间、读取部署、触发构建和创建发布；平台管理员可以创建更高风险的写入、删除、Web Console、密钥值查看、用户管理和站点配置权限。

建议按最小权限创建令牌：只触发构建的 CI 使用 `build:trigger`，只创建发布的自动化使用 `deployment:release`，需要读取构建日志时再额外勾选 `build:read` 或 `deployment:read`。不要给长期有效 Token 勾选不需要的写入或管理权限。

## 密钥

Secret、Token 和 Registry Credential 不会明文回显。编辑时留空表示“不修改已有值”，需要替换时输入新值并保存。

控制台会用浅色的 `****** (已设置)` 占位符提示某个 write-only 字段已有内容。这个文本不是已保存密钥，也不会作为表单值提交；输入新内容后，占位符会被正常输入取代。
