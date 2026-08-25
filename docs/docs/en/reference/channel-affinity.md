# Channel affinity

Channel affinity encourages requests from the same AI conversation to reuse one upstream channel in a model gateway. For long conversations that benefit from prefix caching, this reduces cache misses caused by switching accounts or channels. It is not a permanent binding: the gateway can still fail over when a channel is unavailable.

Luna DevOps enables channel affinity by default. Agent adds `X-Luna-Affinity-Key` only to model requests associated with a conversation. The value is a 64-character hexadecimal hash of the original conversation ID; it contains no conversation content and is never written to logs or Metrics. No affinity header is sent when the setting is disabled.

## Prerequisites

- Configure an OpenAI `chat/completions`-compatible endpoint and enabled models under **Global Settings → AI Assistant**.
- Use a New API version that supports custom channel-affinity rules and the `request_header` key source.
- Configure all New API replicas to share Redis so that mappings survive cross-instance routing.
- For cache-sensitive traffic, prefer one upstream account or API key per New API channel. If a channel randomly selects among several keys, channel affinity cannot guarantee reuse of the upstream cache.

## Enable it in Luna DevOps

1. Open **Global Settings → AI Assistant**.
2. **Provider compatibility** uses automatic detection by default. It detects a direct official DeepSeek endpoint, but when New API or another gateway hides the leaf hostname, explicitly select **OpenAI-compatible** or **DeepSeek-compatible** so Agent uses the correct request and usage adapter.
3. Keep **Channel affinity** enabled. It is enabled by default on new installations.
4. Save the configuration.

Disabling the setting stops Luna from sending the header. It does not delete mappings already stored by New API; those mappings expire according to their New API TTL.

## Configure New API

First enable channel affinity in the **Channel Affinity** section of New API system settings. Enable switching after a successful retry, disable retaining mappings for disabled channels, and allow a failed affinity channel to retry another channel.

Add this rule:

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

The rule matches Luna's current `/v1/chat/completions` path only. `ttl_seconds: 10800` reuses a successful selection for up to three hours; adjust it to the model service's cache window. `include_model_name` and `include_using_group` prevent a conversation from reusing mappings across different models or groups.

For a single New API layer, do not add `X-Luna-Affinity-Key` to the leaf channel's forwarded headers. New API needs it only for channel selection, and keeping it at the gateway avoids exposing a stable identifier to the model Provider.

### Two New API layers

For Luna → New API 1 → New API 2 → Provider, Luna can configure only the Header sent by Agent to the first layer; it cannot modify either external New API instance for you. On the first-layer channel that points to the second layer, explicitly copy the incoming Header with a Header Override such as:

```json
{
  "X-Luna-Affinity-Key": "{client_header:X-Luna-Affinity-Key}"
}
```

The second layer consumes that Header with the `request_header` affinity rule above. Do not configure the same Override on its leaf Provider channels, so the stable Header terminates at layer two. If layer one retries another channel, affinity survives only when that channel also targets the controlled second layer and has the same Override.

This placeholder behavior was checked against New API `main` at [`2d8e50b`](https://github.com/QuantumNous/new-api/blob/2d8e50bf36e94200b809dfb39e73624ec48b1e23/relay/channel/api_request.go#L153-L195). Fields and UI can differ by version. After saving, inspect the actual outbound Header from layer one and confirm that the leaf Provider never receives it.

## Configure the prompt cache key

`prompt_cache_key` is a model-request body capability, not the same field as the gateway-only `X-Luna-Affinity-Key`. Agent derives a separate 64-character hexadecimal hash from the conversation ID with a distinct domain separator. It is used only for conversation-bound assistant requests; health probes, titles, and summaries do not carry it.

Under **Global Settings → AI Assistant → Prompt cache key**, choose:

- **Automatic**: send only to `api.openai.com`; unknown compatible gateways receive no new field by default.
- **Enable for a confirmed endpoint**: use only after confirming that the gateway and every relevant downstream request path support `prompt_cache_key`. The body field can cross two New API layers and reach a capable leaf Provider.
- **Disabled**: never send it.

DeepSeek's official cache does not require this request field, and the DeepSeek adapter never sends it even if the mode is enabled. When DeepSeek is behind New API, set **Provider compatibility** to **DeepSeek-compatible** and verify cache reads from official Provider usage. Current New API Chat Completions DTOs include `prompt_cache_key`, but protocol conversion, channel type, and version can still affect final forwarding. Inspect redacted outbound field names before enabling it, and never log the complete key.

## Verify the result

1. Start a conversation in Luna DevOps and send two consecutive turns.
2. In New API channel-affinity observability or logs, confirm that both requests match `luna-chat-affinity` and successful requests select the same channel. With two layers, also confirm that layer two receives the Header and the leaf Provider does not.
3. If the prompt cache key is enabled, confirm that a compatible leaf Provider receives the body field while logs and Traces omit the complete value.
4. Compare cached-input Tokens reported by the Provider on later turns. If the Provider does not report cache usage, time to first token alone does not prove a cache hit.
5. Temporarily disable the selected channel and send another turn. Confirm that New API selects another healthy channel and the Luna conversation continues.

If the rule does not match, check the request path, rule enablement, header name, and shared Redis configuration for New API replicas. Do not log the complete affinity key; log only the rule name, match outcome, and channel-selection outcome.

See the [New API channel management guide](https://docs.newapi.ai/docs/guide/console/channel-management) and [New API channel-affinity implementation](https://github.com/QuantumNous/new-api/blob/main/service/channel_affinity.go).
