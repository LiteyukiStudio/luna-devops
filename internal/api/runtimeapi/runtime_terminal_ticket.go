package runtimeapi

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	runtimeTerminalTicketTTL       = 45 * time.Second
	runtimeTerminalTicketKeyPrefix = "runtime_terminal:ticket:"
)

type runtimeTerminalTicketResponse struct {
	Ticket    string    `json:"ticket"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type runtimeTerminalTicketValue struct {
	UserID              string                              `json:"userId"`
	Authorization       runtimeTerminalAuthorizationBinding `json:"authorization"`
	ResourceKind        string                              `json:"resourceKind"`
	ResourceFingerprint string                              `json:"resourceFingerprint"`
	ExpiresAt           time.Time                           `json:"expiresAt"`
}

var runtimeTerminalMemoryTickets sync.Map

func (h *Handlers) issueRuntimeTerminalTicket(
	ctx context.Context,
	authorization runtimeTerminalAuthorizationBinding,
	resourceKind string,
	resource any,
) (string, time.Time, error) {
	resourceFingerprint := runtimeTerminalResourceFingerprint(resourceKind, resource)
	if resourceFingerprint == "" {
		return "", time.Time{}, errors.New("runtime terminal ticket resource is invalid")
	}
	expiresAt := time.Now().Add(runtimeTerminalTicketTTL)
	value := runtimeTerminalTicketValue{
		UserID:              authorization.UserID,
		Authorization:       authorization,
		ResourceKind:        resourceKind,
		ResourceFingerprint: resourceFingerprint,
		ExpiresAt:           expiresAt,
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return "", time.Time{}, err
	}
	if h.ticketRedis != nil {
		ticket := "rtt_r_" + randomHex(32)
		redisCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
		err = h.ticketRedis.Set(redisCtx, runtimeTerminalTicketKeyPrefix+hashToken(ticket), payload, runtimeTerminalTicketTTL).Err()
		cancel()
		if err == nil {
			return ticket, expiresAt, nil
		}
		if h.mode == "production" {
			return "", time.Time{}, err
		}
	}
	if h.mode == "production" {
		return "", time.Time{}, errors.New("Redis is required for production runtime terminal tickets")
	}
	ticket := "rtt_m_" + randomHex(32)
	ticketHash := hashToken(ticket)
	runtimeTerminalMemoryTickets.Store(ticketHash, value)
	time.AfterFunc(runtimeTerminalTicketTTL, func() {
		runtimeTerminalMemoryTickets.CompareAndDelete(ticketHash, value)
	})
	return ticket, expiresAt, nil
}

func (h *Handlers) consumeRuntimeTerminalTicket(ctx context.Context, ticket string) (runtimeTerminalTicketValue, bool, error) {
	var value runtimeTerminalTicketValue
	switch {
	case strings.HasPrefix(ticket, "rtt_r_"):
		if h.ticketRedis == nil {
			return value, false, errors.New("Redis runtime terminal ticket store is unavailable")
		}
		redisCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
		raw, err := h.ticketRedis.GetDel(redisCtx, runtimeTerminalTicketKeyPrefix+hashToken(ticket)).Bytes()
		cancel()
		if errors.Is(err, redis.Nil) {
			return value, false, nil
		}
		if err != nil {
			return value, false, err
		}
		if err := json.Unmarshal(raw, &value); err != nil {
			return value, false, err
		}
	case strings.HasPrefix(ticket, "rtt_m_") && h.mode != "production":
		raw, found := runtimeTerminalMemoryTickets.LoadAndDelete(hashToken(ticket))
		if !found {
			return value, false, nil
		}
		var ok bool
		value, ok = raw.(runtimeTerminalTicketValue)
		if !ok {
			return value, false, errors.New("invalid in-memory runtime terminal ticket")
		}
	default:
		return value, false, nil
	}
	if !value.ExpiresAt.After(time.Now()) || value.UserID == "" || value.Authorization.UserID != value.UserID {
		return runtimeTerminalTicketValue{}, false, nil
	}
	return value, true, nil
}

func (value runtimeTerminalTicketValue) matches(resourceKind string, resource any) bool {
	return value.ResourceKind == resourceKind &&
		value.ResourceFingerprint == runtimeTerminalResourceFingerprint(resourceKind, resource)
}

func runtimeTerminalResourceFingerprint(resourceKind string, resource any) string {
	payload, err := json.Marshal(resource)
	if err != nil {
		return ""
	}
	return hashToken(resourceKind + "\n" + string(payload))
}
