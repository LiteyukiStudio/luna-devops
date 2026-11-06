# 构建参数与构建变量

本页说明源码构建时向 Dockerfile 传值的两种方式、它们的格式和优先级，以及构建与部署钩子中可读取的平台环境变量。

| 方式 | 在哪里填写 | 适合 | 是否支持模板变量 |
| --- | --- | --- | --- |
| Build Args | 部署配置 → 构建 → Build Args | 版本号、开关等非敏感值 | 支持 |
| 构建变量与密钥 | 全局设置、项目空间“共享配置 → 构建变量”、应用、部署配置 | 多个应用共用的值；密钥 | 不支持 |

两种方式最终都以 BuildKit `build-arg` 传入。Dockerfile 中需要先声明 `ARG NAME` 才能读取对应值。

## Build Args 格式

每行一个 `KEY=value`：

```text
# 以 # 开头的行和空行会被忽略
EMBED_WEB=true
VERSION=${{ github.ref_name }}-{short_sha}
```

- 一行中没有 `=` 时，会把第一个 `:` 当作分隔符，例如 `MODE:prod`。
- 也可以填写 JSON 对象，例如 `{"EMBED_WEB":"true"}`；内容以 `{` 开头时按 JSON 解析。
- 名称和值首尾空白会被去掉；同名的后一行覆盖前一行。
- 名称只能使用字母、数字和下划线，不能以数字开头，最长 128 个字符。
- 单个值最长 4096 字节，整段内容最长 64 KiB。

格式不正确时保存会失败，错误代码为 `deployment.build_args_invalid`。

Build Args 会随部署配置快照保存到每条构建记录中，有权限的成员可以看到。**不要在这里填写密码或 Token**，敏感值请使用构建密钥。

### 值中的模板变量

Build Args 的值在构建时渲染，可用变量与镜像 Tag 相同：

| 变量 | 渲染结果 |
| --- | --- |
| `${{ github.sha }}` | 实际检出的完整提交 SHA |
| `{short_sha}` | 提交 SHA 前 12 位 |
| `${{ github.ref_name }}` | 构建 Git 标签时取标签名，否则取分支名 |
| `${{ github.ref_type }}` | `tag` 或 `branch` |
| `${{ github.ref }}` | `refs/heads/<分支>` 或 `refs/tags/<标签>` |
| `{commit}` / `{shortSha}` | 同 `${{ github.sha }}` / `{short_sha}` |
| `{branch}` / `{tag}` | 分支名 / Git 标签名，未构建对应引用时为空 |
| `{branchSlug}` / `{tagSlug}` | 分支名 / 标签名转为小写字母、数字和 `-` |

与镜像 Tag 不同，Build Args 的值不做字符规范化，例如 `${{ github.ref }}` 保持 `refs/heads/main`。表外的写法不会被替换。各变量的详细说明见[镜像仓库路径与 Tag 模板](./image-templates#tag-变量)。

## 构建变量与密钥

构建变量按以下顺序合并，后者覆盖前者的同名值：

1. 全局构建变量。
2. 构建变量集：先按创建顺序合并绑定到该项目空间的项目空间变量集，再按选择顺序合并部署配置中额外选择的变量集。
3. 应用级构建变量。
4. 部署配置级构建变量。

规则：

- 名称规则与 Build Args 相同；单个值最长 4096 字节。
- 同一名称在胜出的那一层只能是普通变量或密钥之一。
- 名称以 `LITEYUKI_` 开头的变量为平台保留，会被忽略。
- 与 Build Args 同名时，Build Args 的值生效。
- 构建变量的值不渲染模板变量，`${{ github.sha }}` 会原样传入。
- 普通构建变量会进入构建记录；密钥值只在构建时解析，不回显，并在构建日志中被遮盖。

> 密钥同样以 `build-arg` 传入。被 `RUN` 指令使用的 build-arg 可能出现在镜像构建历史中；不要在最终运行阶段引用密钥，优先在多阶段构建的中间阶段使用。

## 钩子中的环境变量

钩子脚本不做模板渲染，请直接读取环境变量。

**构建钩子**（仓库拉取前后、构建前后、镜像推送前后）运行在构建容器中，可读取：

| 变量 | 含义 |
| --- | --- |
| `LITEYUKI_PROJECT_ID` | 项目空间 ID |
| `LITEYUKI_APPLICATION_ID` | 应用 ID |
| `LITEYUKI_DEPLOYMENT_TARGET_ID` | 部署配置 ID |
| `LITEYUKI_BUILD_RUN_ID` | 构建记录 ID |
| `LITEYUKI_BUILD_JOB_ID` | 构建任务 ID |
| `LITEYUKI_HOOK_RUN_ID` | 本次钩子运行 ID |
| `LITEYUKI_HOOK_PHASE` | 钩子阶段，例如 `preBuild` |
| `LITEYUKI_GIT_BRANCH` | 分支名，构建标签时可能为空 |
| `LITEYUKI_GIT_TAG` | Git 标签名，构建分支时为空 |
| `LITEYUKI_GIT_REF_NAME` | 标签名或分支名 |
| `LITEYUKI_GIT_REF_TYPE` | `tag` 或 `branch` |
| `LITEYUKI_GIT_REF` | `refs/heads/...` 或 `refs/tags/...` |
| `LITEYUKI_GIT_SHA` | 提交 SHA；仓库拉取前为请求中的提交，可能为空 |
| `LITEYUKI_GIT_SHORT_SHA` | 提交 SHA 前 12 位 |
| `LITEYUKI_IMAGE_REF` | 目标镜像地址；构建前及之后的阶段为最终推送地址 |

构建变量、密钥和 Build Args 也以同名环境变量提供给构建钩子。

**部署钩子**（部署前、部署后）以独立 Kubernetes Job 运行，使用本次发布的镜像，并加载与应用容器相同的普通变量和加密变量。除 `LITEYUKI_BUILD_JOB_ID` 外，可读取上表同名变量，另外提供：

| 变量 | 含义 |
| --- | --- |
| `LITEYUKI_RELEASE_ID` | 本次发布 ID |
| `LUNA_DEPLOYMENT_STAGE` | 部署阶段 |

部署钩子中的 `LITEYUKI_IMAGE_REF` 是本次发布的镜像地址；Git 相关变量来自生成该镜像的构建，直接发布已有镜像时可能为空。
