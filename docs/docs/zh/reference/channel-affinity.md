# 渠道亲和性

渠道亲和性让同一 AI 会话尽量复用模型网关中的同一上游渠道。对于依赖前缀缓存的长对话，这通常能减少跨账号或跨渠道后缓存失效的概率；它不是永久绑定，渠道不可用时网关仍可切换。

Luna DevOps 默认启用渠道亲和性。启用后，Agent 只为关联到会话的模型请求发送 `X-Luna-Affinity-Key`。其值是从原始会话 ID 派生的 64 字符十六进制散列，不包含会话正文，也不会写入日志或指标。关闭后不会发送该请求头。

## 前置条件

- 已在 **全局设置 → AI 助手** 配置 OpenAI `chat/completions` 兼容地址和可用模型。
- New API 版本支持自定义渠道亲和规则和 `request_header` 键来源。
- 多副本 New API 共用 Redis，否则亲和映射不能跨实例复用。
- 对缓存命中敏感的流量，建议一个 New API 渠道只配置一个上游账号或 API Key；渠道内部再次随机选择 Key 时，渠道亲和性不能保证命中同一上游缓存。

## 在 Luna DevOps 中启用

1. 进入 **全局设置 → AI 助手**。
2. 保持 **渠道亲和性** 开启。新安装默认开启。
3. 保存配置。

关闭该开关只会停止发送亲和请求头，不会清除 New API 中已经存在的映射；映射会按 New API 的 TTL 自动过期。

## 配置 New API

先在 New API 系统设置的 **Channel Affinity** 区域启用渠道亲和性。推荐开启“成功后切换”，关闭“渠道禁用后仍保留”，并确保失败时允许重试其他渠道。

新增以下规则：

```json
{
  "name": "luna-chat-affinity",
  "model_regex": [".*"],
  "path_regex": ["^/v1/chat/completions$"],
  "key_sources": [
    {
      "type": "request_header",
      "key": "X-Luna-Affinity-Key"
    }
  ],
  "value_regex": "^[a-f0-9]{64}$",
  "ttl_seconds": 10800,
  "skip_retry_on_failure": false,
  "include_using_group": true,
  "include_model_name": true,
  "include_rule_name": true
}
```

这条规则只匹配 Luna 当前使用的 `/v1/chat/completions`。`ttl_seconds: 10800` 表示一次成功选择在三小时内可复用；可以按模型服务的缓存窗口调短或调长。`include_model_name` 和 `include_using_group` 可避免同一会话在不同模型或分组之间误用映射。

不需要把 `X-Luna-Affinity-Key` 加入转发到上游模型的请求头列表。它只供 New API 选择渠道使用，避免向模型供应商暴露稳定标识。

## 验证结果

1. 在 Luna DevOps 中新建会话并连续提问两轮。
2. 在 New API 的渠道亲和观测或日志中确认两次请求命中 `luna-chat-affinity`，且成功请求选择了同一渠道。
3. 从 Provider 官方用量中比较后续轮次的缓存输入 Token；如果 Provider 不报告缓存用量，只比较首字时间不能证明缓存命中。
4. 临时停用被选渠道并再次提问，确认 New API 能选择其他健康渠道，Luna 会话仍可继续。

如果规则没有命中，依次检查请求路径、规则是否启用、Header 键名和 New API 多副本的 Redis 配置。不要在日志中输出完整亲和键；排障时只记录规则名、是否命中和渠道选择结果。

相关参考：[New API 渠道管理文档](https://docs.newapi.ai/zh/docs/guide/console/channel-management) 与 [New API 渠道亲和实现](https://github.com/QuantumNous/new-api/blob/main/service/channel_affinity.go)。
