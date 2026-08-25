package agentobservability

import (
	"math"
	"strconv"
	"strings"
)

// TokenUsage is a typed aggregate of provider-reported model usage. Cache and
// reasoning fields remain nil when any included usage omitted or invalidated
// the corresponding breakdown.
type TokenUsage struct {
	InputTokens           int64    `json:"inputTokens"`
	OutputTokens          int64    `json:"outputTokens"`
	CacheReadInputTokens  *int64   `json:"cacheReadInputTokens"`
	CacheWriteInputTokens *int64   `json:"cacheWriteInputTokens"`
	ReasoningOutputTokens *int64   `json:"reasoningOutputTokens"`
	CacheHitRate          *float64 `json:"cacheHitRate"`
}

func cacheHitRate(inputTokens int64, cacheReadInputTokens *int64) *float64 {
	if inputTokens <= 0 || cacheReadInputTokens == nil || *cacheReadInputTokens < 0 || *cacheReadInputTokens > inputTokens {
		return nil
	}
	rate := float64(*cacheReadInputTokens) * 100 / float64(inputTokens)
	return &rate
}

func aggregateTraceTokenUsage(spans []TraceSpan) *TokenUsage {
	usage := &TokenUsage{}
	reported := 0
	cacheReadTotal, cacheWriteTotal, reasoningTotal := int64(0), int64(0), int64(0)
	cacheReadComplete, cacheWriteComplete, reasoningComplete := true, true, true

	for _, span := range spans {
		if !isTraceModelSpan(span.Attributes) {
			continue
		}
		status := strings.TrimSpace(span.Attributes["luna.gen_ai.usage.status"])
		if status == "unavailable" {
			continue
		}
		if status != "" && status != "reported" {
			return nil
		}
		inputTokens, inputState := traceTokenAttribute(span.Attributes, "gen_ai.usage.input_tokens")
		outputTokens, outputState := traceTokenAttribute(span.Attributes, "gen_ai.usage.output_tokens")
		if inputState != traceTokenValid || outputState != traceTokenValid {
			return nil
		}
		var ok bool
		if usage.InputTokens, ok = addTraceTokens(usage.InputTokens, inputTokens); !ok {
			return nil
		}
		if usage.OutputTokens, ok = addTraceTokens(usage.OutputTokens, outputTokens); !ok {
			return nil
		}
		reported++

		cacheRead, cacheReadState := traceTokenAttribute(span.Attributes, "gen_ai.usage.cache_read.input_tokens")
		cacheWrite, cacheWriteState := traceTokenAttribute(span.Attributes, "gen_ai.usage.cache_write.input_tokens")
		reasoning, reasoningState := traceTokenAttribute(span.Attributes, "gen_ai.usage.reasoning.output_tokens")
		if cacheReadState == traceTokenValid && cacheRead > inputTokens {
			cacheReadState = traceTokenInvalid
		}
		if cacheWriteState == traceTokenValid && cacheWrite > inputTokens {
			cacheWriteState = traceTokenInvalid
		}
		if cacheReadState == traceTokenValid && cacheWriteState == traceTokenValid && cacheRead > inputTokens-cacheWrite {
			cacheReadState, cacheWriteState = traceTokenInvalid, traceTokenInvalid
		}
		if reasoningState == traceTokenValid && reasoning > outputTokens {
			reasoningState = traceTokenInvalid
		}

		cacheReadTotal, cacheReadComplete = aggregateTraceBreakdown(cacheReadTotal, cacheReadComplete, cacheRead, cacheReadState)
		cacheWriteTotal, cacheWriteComplete = aggregateTraceBreakdown(cacheWriteTotal, cacheWriteComplete, cacheWrite, cacheWriteState)
		reasoningTotal, reasoningComplete = aggregateTraceBreakdown(reasoningTotal, reasoningComplete, reasoning, reasoningState)
	}

	if reported == 0 {
		return nil
	}
	if cacheReadComplete {
		usage.CacheReadInputTokens = &cacheReadTotal
	}
	if cacheWriteComplete {
		usage.CacheWriteInputTokens = &cacheWriteTotal
	}
	if reasoningComplete {
		usage.ReasoningOutputTokens = &reasoningTotal
	}
	usage.CacheHitRate = cacheHitRate(usage.InputTokens, usage.CacheReadInputTokens)
	return usage
}

func isTraceModelSpan(attributes map[string]string) bool {
	switch strings.TrimSpace(attributes["gen_ai.operation.name"]) {
	case "chat", "generate_content", "text_completion":
		return true
	default:
		return false
	}
}

type traceTokenState uint8

const (
	traceTokenMissing traceTokenState = iota
	traceTokenValid
	traceTokenInvalid
)

func traceTokenAttribute(attributes map[string]string, key string) (int64, traceTokenState) {
	text, exists := attributes[key]
	if !exists {
		return 0, traceTokenMissing
	}
	value, err := strconv.ParseInt(strings.TrimSpace(text), 10, 64)
	if err != nil || value < 0 {
		return 0, traceTokenInvalid
	}
	return value, traceTokenValid
}

func addTraceTokens(total, value int64) (int64, bool) {
	if value < 0 || total > math.MaxInt64-value {
		return 0, false
	}
	return total + value, true
}

func aggregateTraceBreakdown(total int64, complete bool, value int64, state traceTokenState) (int64, bool) {
	if !complete || state != traceTokenValid {
		return total, false
	}
	next, ok := addTraceTokens(total, value)
	return next, ok
}
