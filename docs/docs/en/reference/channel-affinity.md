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
2. Keep **Channel affinity** enabled. It is enabled by default on new installations.
3. Save the configuration.

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

Do not add `X-Luna-Affinity-Key` to headers forwarded to the upstream model. New API needs it only for channel selection, and keeping it at the gateway avoids exposing a stable identifier to the model provider.

## Verify the result

1. Start a conversation in Luna DevOps and send two consecutive turns.
2. In New API channel-affinity observability or logs, confirm that both requests match `luna-chat-affinity` and successful requests select the same channel.
3. Compare cached-input Tokens reported by the Provider on later turns. If the Provider does not report cache usage, time to first token alone does not prove a cache hit.
4. Temporarily disable the selected channel and send another turn. Confirm that New API selects another healthy channel and the Luna conversation continues.

If the rule does not match, check the request path, rule enablement, header name, and shared Redis configuration for New API replicas. Do not log the complete affinity key; log only the rule name, match outcome, and channel-selection outcome.

See the [New API channel management guide](https://docs.newapi.ai/docs/guide/console/channel-management) and [New API channel-affinity implementation](https://github.com/QuantumNous/new-api/blob/main/service/channel_affinity.go).
