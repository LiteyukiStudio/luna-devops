# 镜像仓库路径与 Tag 模板

源码构建推送的镜像地址由三部分组成：`<镜像站主机>/<仓库路径>:<tag>`。本页说明仓库路径模板、Tag 模板可用的变量、渲染结果和限制。

## 在哪里填写

| 位置 | 字段 | 作用 |
| --- | --- | --- |
| 镜像站 → 推送凭据 | 镜像仓库模板 | 新建部署配置时生成默认仓库路径。 |
| 镜像站 → 推送凭据 | 镜像 Tag 模板 | 新建部署配置时生成默认 Tag，默认值 `latest`。 |
| 部署配置 → 基本信息与来源 | 目标镜像 | 每次构建实际使用的 `仓库路径:tag`，可直接修改。 |
| 应用 → 构建 → 手动构建 | 目标镜像 | 只影响本次构建。 |

凭据模板只在部署配置的目标镜像为空或仍是平台默认值时生效；手动改过的目标镜像会原样保留。修改凭据模板不会改写已有部署配置。

同一镜像站上的一个仓库路径只能属于一个项目空间。其它项目空间的部署配置或构建已经用过该仓库时，保存部署配置返回 `deployment_target.image_repository_claimed`，触发构建返回 `build.image_repository_claimed`；同一项目空间内的多个应用和部署配置可以共用仓库。

## 仓库路径模板变量

默认模板：`{registryNamespace}/{projectIdentifier}-{appIdentifier}`。变量可以写成 `{name}` 或 `${name}`，两种写法等价。

| 变量 | 含义 | 取值来源 |
| --- | --- | --- |
| `{registryNamespace}` | 镜像站命名空间 | 镜像站未设置命名空间时（控制台创建的镜像站都如此）取项目空间标识。 |
| `{projectIdentifier}` | 项目空间标识 | 项目空间标识；为空时取项目空间名称。 |
| `{project}` | 同 `{projectIdentifier}` | 别名。 |
| `{appIdentifier}` | 应用标识 | 应用标识；为空时取应用名称。 |
| `{app}` | 同 `{appIdentifier}` | 别名。 |
| `{applicationIdentifier}` | 同 `{appIdentifier}` | 别名。 |
| `{application}` | 同 `{appIdentifier}` | 别名。 |
| `{stage}` | 部署阶段 | `dev`、`test`、`staging` 或 `prod`。 |
| `{target}` | 部署配置名称 | 部署配置显示名称；为空时取阶段。 |

除 `{registryNamespace}` 外，变量值都会规范化：转为小写，连续的非 `a-z0-9` 字符替换为一个 `-`，去掉首尾 `-`；结果为空时使用 `app`。渲染后会去掉首尾 `/` 并合并重复的 `/`。

**示例**：项目空间标识 `neo-blog`、应用标识 `frontend`、阶段 `prod`、部署配置名称 `Prod Web`：

| 模板 | 渲染结果 |
| --- | --- |
| `{registryNamespace}/{projectIdentifier}-{appIdentifier}` | `neo-blog/neo-blog-frontend` |
| `team/{project}-{app}-{stage}` | `team/neo-blog-frontend-prod` |
| `team/{app}/{target}` | `team/frontend/prod-web` |

### 镜像站主机前缀

- 仓库路径第一段包含 `.` 或 `:`，或等于 `localhost` 时，视为已经带主机，原样使用，例如 `harbor.example.com/team/app`。
- DockerHub 镜像站不加主机前缀，路径第一段必须是你的 DockerHub 用户名或组织名。
- 其他镜像站会自动加上镜像站地址中的主机，例如 `https://harbor.example.com` 加上 `team/app` 得到 `harbor.example.com/team/app`。

## Tag 变量

部署配置或手动构建“目标镜像”的 Tag 部分在构建时渲染，可用以下变量：

| 变量 | 含义 | 示例值 |
| --- | --- | --- |
| `${{ github.sha }}` | 实际检出的完整提交 SHA | `1234567890abcdef1234567890abcdef12345678` |
| `{short_sha}` | 提交 SHA 前 12 位 | `1234567890ab` |
| `${{ github.ref_name }}` | 构建 Git 标签时取标签名，否则取分支名 | `main`、`v1.2.0` |
| `${{ github.ref_type }}` | `tag` 或 `branch` | `branch` |
| `${{ github.ref }}` | 完整引用 | `refs/heads/main`、`refs/tags/v1.2.0` |
| `{commit}` | 同 `${{ github.sha }}` | `1234567890abcdef1234567890abcdef12345678` |
| `{shortSha}` | 同 `{short_sha}` | `1234567890ab` |
| `{branch}` | 分支名；构建 Git 标签时为空 | `feature/login` |
| `{branchSlug}` | 分支名转为小写字母、数字和 `-` | `feature-login` |
| `{tag}` | Git 标签名；构建分支时为空 | `v1.2.0` |
| `{tagSlug}` | 标签名转为小写字母、数字和 `-` | `v1-2-0` |

这些写法区分大小写，空格也必须与上表一致；表外的写法（例如 `{sha}`、`{ref_name}`）不会被替换。

渲染后 Tag 会被规范化：`A-Z a-z 0-9 _ . -` 以外的连续字符替换为一个 `-`，去掉首尾的 `.` 和 `-`，最长 128 个字符；结果为空时使用 `latest`。

`buildcache` 是平台构建缓存的保留 Tag：渲染结果为 `buildcache` 的构建会在推送前失败，镜像站中的 `buildcache` Tag 也不会出现在发布可选镜像中。

**示例**：分支 `feature/login`，提交 `1234567890abcdef…`：

| 目标镜像 | 实际推送 |
| --- | --- |
| `team/api:${{ github.ref_name }}-{short_sha}` | `team/api:feature-login-1234567890ab` |
| `team/api:${{ github.ref }}` | `team/api:refs-heads-feature-login` |
| `team/api:{short_sha}` | `team/api:1234567890ab` |
| `team/api:{branchSlug}-{shortSha}` | `team/api:feature-login-1234567890ab` |

### 推送凭据中的 Tag 模板

凭据的“镜像 Tag 模板”用于生成新部署配置的默认 Tag。创建部署配置时先替换[仓库路径模板变量](#仓库路径模板变量)，上表的构建时变量原样保留，在每次构建时渲染。例如模板 `{stage}-{shortSha}` 生成的默认 Tag 为 `prod-{shortSha}`，构建后推送 `prod-1234567890ab`。模板中有其他未知占位符时，默认 Tag 回落为 `latest`。

## 常见错误

- **推送的 Tag 变成了 `sha`、`ref-name` 这类字面量**：使用了表外的写法，构建时不会被替换，只会被规范化成字面量。请改用上表中的写法。
- **仓库路径里出现 `{...}`**：仓库路径模板中拼错或使用了不存在的变量，未知变量会原样保留，推送会失败。核对变量名后保存凭据，再在部署配置中恢复默认目标镜像。
- **DockerHub 推送被拒绝**：仓库路径第一段不是你有权推送的用户名或组织名。把模板改为 `你的用户名/{projectIdentifier}-{appIdentifier}`。
- **`{target}` 渲染成 `app`**：部署配置名称只包含中文或符号，规范化后为空。改用 `{stage}`，或在名称中包含字母数字。
- **保存部署配置提示仓库已被其它项目空间使用**：换一个仓库路径，例如在路径中包含本项目空间标识；需要共用镜像时，请在原项目空间内构建。
- **构建日志提示 `image tag "buildcache" is reserved`**：Tag 渲染结果与缓存保留 Tag 相同，例如以 `{branch}` 为 Tag 构建名为 `buildcache` 的分支。修改 Tag 模板后重试。
- **构建过程中显示的镜像地址与最终不同**：构建完成前显示的是平台预估值，构建成功后以实际推送的镜像地址为准。
