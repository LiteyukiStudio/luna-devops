package registryprovider

import (
	"errors"
	"fmt"
)

// UpstreamError 是对镜像站上游 HTTP 错误的结构化封装。
type UpstreamError struct {
	StatusCode int
	Message    string
}

func (e *UpstreamError) Error() string {
	return fmt.Sprintf("registry upstream returned %d: %s", e.StatusCode, e.Message)
}

// AsUpstreamError 从 err 中提取 *UpstreamError；不是上游错误时返回 false。
func AsUpstreamError(err error) (*UpstreamError, bool) {
	if err == nil {
		return nil, false
	}
	var upstream *UpstreamError
	if errors.As(err, &upstream) {
		return upstream, true
	}
	return nil, false
}
