package notification

import (
	"context"
	"encoding/json"
)

func resolveSecretMap(ctx context.Context, raw json.RawMessage, resolver SecretResolver) map[string]string {
	refs := map[string]string{}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &refs)
	}
	resolved := map[string]string{}
	for key, ref := range refs {
		if resolver == nil {
			resolved[key] = ""
			continue
		}
		resolved[key] = resolver.ResolveContext(ctx, ref)
	}
	return resolved
}
