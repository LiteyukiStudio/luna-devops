# 运行环境变量与引用语法

本页说明部署配置和共享运行配置中环境变量的命名规则、合并优先级、`${NAME}` 引用语法、随机密钥和配置文件路径规则。操作步骤见[配置与密钥](../use/workflow/configuration-and-secrets)。

## 名称与取值限制

| 项目 | 规则 |
| --- | --- |
| 变量名称 | 字母、数字和下划线，不能以数字开头，最长 128 个字符，例如 `DATABASE_URL`、`_TOKEN` |
| 变量值 | 最长 8192 个字符 |
| 数量 | 每个部署配置或共享运行配置最多 128 个变量 |
| 唯一性 | 同一部署配置或共享运行配置内，普通变量与加密变量不能同名 |

不符合规则时保存失败，常见错误代码：`deployment.runtime_environment_key_invalid`（名称无效）、`deployment.runtime_environment_key_duplicate`（名称重复）、`deployment.runtime_environment_key_conflict`（普通与加密同名）、`deployment.runtime_environment_value_too_long`（值过长）。

## 合并优先级

发布时按以下顺序合并，后者覆盖前者的同名值：

1. 部署配置引用的共享运行配置，按引用顺序。
2. 部署配置自己的环境变量。

任一层的加密变量都优先于其他层的同名普通变量。合并后再加入服务关系注入的变量；服务关系变量与已有变量同名时发布失败，需要修改其中一方的名称。

## `${NAME}` 引用

普通变量的值可以用 `${NAME}` 引用同一部署最终生效的其他变量，发布时展开：

```text
DB_HOST=postgres
DB_PORT=5432
DB_NAME=app
DATABASE_URL=postgresql://app@${DB_HOST}:${DB_PORT}/${DB_NAME}
```

发布后容器中的 `DATABASE_URL` 为 `postgresql://app@postgres:5432/app`。

规则：

- 只支持 `${NAME}` 写法，`NAME` 需符合变量名称规则；`$NAME`、`$(NAME)` 不会被平台展开。
- 可引用的范围：合并后的普通变量、加密变量和服务关系注入的变量，例如 `${POSTGRES_HOST}`。
- 支持链式引用（A 引用 B，B 再引用 C）。
- 引用不存在的变量或引用自身时，保持原样，例如 `${MISSING}` 仍是 `${MISSING}`。
- 被引用的变量值为空时，替换为空字符串。
- 只有普通变量的值会被展开；加密变量的值、配置文件和密钥文件内容不会展开。

> **注意**：普通变量引用加密变量时，展开后的完整值会写入普通变量所在的 ConfigMap，有权限的客户端可以读到其中的密钥。包含密码的连接串请整体保存为加密变量，不要用普通变量拼接。

## 随机生成的加密值

在环境变量表中对加密变量选择“重新生成”，保存时平台生成 32 个字符的随机值，字符集为 Base64（`A-Z a-z 0-9 + /`），值不会回显。

通过 API 生成时可以指定：

| 参数 | 允许值 | 默认 |
| --- | --- | --- |
| 长度 | 8–256 | 32 |
| 编码 | `base64`、`hex`、`alphanumeric`、`numeric` | `base64` |

需要放进 URL 的随机值建议使用 `alphanumeric` 或 `hex`，避免 `+` 和 `/` 需要转义。参数无效时返回 `deployment.secret_generation_invalid`。

## 配置文件与密钥文件路径

- 路径必须是容器内绝对路径，例如 `/app/config/settings.yaml`；不能是 `/`。
- 路径会被规范化，`/app/./config/../settings.yaml` 等价于 `/app/settings.yaml`；规范化后不能重复。
- 文件以只读方式挂载；不能与数据卷挂载路径冲突。
- 共享运行配置与部署配置中的同路径文件，以部署配置为准。
- 文件内容原样挂载，不渲染变量。

路径无效时返回 `deployment.runtime_config_path_invalid`；与数据卷冲突时返回 `deployment.runtime_path_invalid`。

## 生效时机

修改环境变量、共享运行配置或文件后，需要重新发布才会影响运行中的 Pod。以“跟随引用”方式引用的共享运行配置更新后，平台会提示重新部署；“使用快照”只复制引用当时的内容，后续更新不影响该部署配置。
