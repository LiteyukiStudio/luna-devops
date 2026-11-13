package notification

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/security"
)

const DefaultTimeZone = "Asia/Shanghai"

type egressPolicyContextKey struct{}
type testRecipientContextKey struct{}
type timeZoneContextKey struct{}

func WithEgressPolicy(ctx context.Context, policy security.EgressPolicy) context.Context {
	return context.WithValue(ctx, egressPolicyContextKey{}, policy)
}

func EgressPolicyFromContext(ctx context.Context) security.EgressPolicy {
	if ctx != nil {
		if policy, ok := ctx.Value(egressPolicyContextKey{}).(security.EgressPolicy); ok {
			return policy
		}
	}
	return security.PublicEgressPolicy()
}

func EgressPolicyForScope(scope string) security.EgressPolicy {
	if NormalizeChannelScope(scope) == "global" {
		return security.AdminEgressPolicy()
	}
	return security.PublicEgressPolicy()
}

func NormalizeChannelScope(scope string) string {
	switch strings.ToLower(strings.TrimSpace(scope)) {
	case "project":
		return "project"
	case "user":
		return "user"
	default:
		return "global"
	}
}

func WithTestRecipient(ctx context.Context, email string) context.Context {
	return context.WithValue(ctx, testRecipientContextKey{}, strings.TrimSpace(email))
}

func TestRecipientFromContext(ctx context.Context) string {
	if ctx != nil {
		if value, ok := ctx.Value(testRecipientContextKey{}).(string); ok {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func WithTimeZone(ctx context.Context, timeZone string) context.Context {
	return context.WithValue(ctx, timeZoneContextKey{}, NormalizeTimeZone(timeZone))
}

func TimeZoneFromContext(ctx context.Context) string {
	if ctx != nil {
		if value, ok := ctx.Value(timeZoneContextKey{}).(string); ok && strings.TrimSpace(value) != "" {
			return value
		}
	}
	return DefaultTimeZone
}

func NormalizeTimeZone(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return DefaultTimeZone
	}
	if _, err := time.LoadLocation(value); err != nil {
		return DefaultTimeZone
	}
	return value
}

func ValidateTimeZone(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return DefaultTimeZone, nil
	}
	if _, err := time.LoadLocation(value); err != nil {
		return "", fmt.Errorf("invalid IANA time zone %q", value)
	}
	return value, nil
}

func FormatTimeInZone(value time.Time, timeZone string) string {
	return FormatTimeInZoneLayout(value, timeZone, "2006-01-02 15:04:05 MST")
}

func FormatTimeInZoneLayout(value time.Time, timeZone string, layout string) string {
	if value.IsZero() {
		return ""
	}
	if strings.TrimSpace(layout) == "" {
		layout = "2006-01-02 15:04:05 MST"
	}
	loc, err := time.LoadLocation(NormalizeTimeZone(timeZone))
	if err != nil {
		loc = time.FixedZone("CST", 8*3600)
	}
	return value.In(loc).Format(layout)
}

type ChannelTargetOverride struct {
	To []string `json:"to,omitempty"`
	Cc []string `json:"cc,omitempty"`
}

func DecodeChannelTargets(raw []byte) (map[string]ChannelTargetOverride, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" || trimmed == "{}" {
		return map[string]ChannelTargetOverride{}, nil
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var targets map[string]ChannelTargetOverride
	if err := decoder.Decode(&targets); err != nil {
		return nil, fmt.Errorf("invalid channel targets: %w", err)
	}
	normalized := make(map[string]ChannelTargetOverride, len(targets))
	for channelID, override := range targets {
		channelID = strings.TrimSpace(channelID)
		if channelID == "" {
			continue
		}
		toList, err := normalizeEmailAddressList(override.To)
		if err != nil {
			return nil, fmt.Errorf("channel %s recipient: %w", channelID, err)
		}
		ccList, err := normalizeEmailAddressList(override.Cc)
		if err != nil {
			return nil, fmt.Errorf("channel %s cc: %w", channelID, err)
		}
		if len(toList) == 0 && len(ccList) == 0 {
			continue
		}
		normalized[channelID] = ChannelTargetOverride{To: toList, Cc: ccList}
	}
	return normalized, nil
}

func EncodeChannelTargets(targets map[string]ChannelTargetOverride) (string, error) {
	if len(targets) == 0 {
		return "{}", nil
	}
	data, err := json.Marshal(targets)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func DecodeChannelTargetOverride(raw string) ChannelTargetOverride {
	var override ChannelTargetOverride
	if strings.TrimSpace(raw) == "" {
		return override
	}
	_ = json.Unmarshal([]byte(raw), &override)
	override.To, _ = normalizeEmailAddressList(override.To)
	override.Cc, _ = normalizeEmailAddressList(override.Cc)
	return override
}

func EncodeChannelTargetOverride(override ChannelTargetOverride) string {
	override.To, _ = normalizeEmailAddressList(override.To)
	override.Cc, _ = normalizeEmailAddressList(override.Cc)
	if len(override.To) == 0 && len(override.Cc) == 0 {
		return "{}"
	}
	data, _ := json.Marshal(override)
	return string(data)
}

func MergeChannelTargetOverride(left, right ChannelTargetOverride) ChannelTargetOverride {
	mergedTo, _ := normalizeEmailAddressList(append(append([]string(nil), left.To...), right.To...))
	mergedCc, _ := normalizeEmailAddressList(append(append([]string(nil), left.Cc...), right.Cc...))
	return ChannelTargetOverride{To: mergedTo, Cc: mergedCc}
}

func ApplySMTPTargetOverride(configRaw json.RawMessage, override ChannelTargetOverride) (json.RawMessage, []string, error) {
	var cfg SMTPConfig
	if err := json.Unmarshal(configRaw, &cfg); err != nil {
		return nil, nil, err
	}
	if len(override.To) > 0 {
		cfg.To = override.To
	}
	if len(override.Cc) > 0 {
		cfg.Cc = override.Cc
	}
	recipients, err := normalizeEmailAddressList(append(append([]string(nil), cfg.To...), append(cfg.Cc, cfg.Bcc...)...))
	if err != nil {
		return nil, nil, err
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		return nil, nil, err
	}
	return data, recipients, nil
}

func normalizeEmailAddressList(values []string) ([]string, error) {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		parsed, err := mail.ParseAddress(value)
		if err != nil {
			return nil, fmt.Errorf("invalid email address %q: %w", value, err)
		}
		key := strings.ToLower(strings.TrimSpace(parsed.Address))
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	return out, nil
}
