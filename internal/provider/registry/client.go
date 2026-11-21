package registryprovider

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/security"
	"github.com/LiteyukiStudio/devops/internal/telemetry"
)

type Credential struct {
	Username string
	Secret   string
}

type PingResult struct {
	Success    bool
	StatusCode int
	Message    string
	Endpoint   string
}

func Ping(parent context.Context, endpointText string, policy security.EgressPolicy, credential Credential) PingResult {
	endpoint, err := PingURL(endpointText)
	if err != nil {
		return PingResult{Success: false, Message: "镜像站地址无效"}
	}
	if _, err := policy.ValidateURL(endpoint.String()); err != nil {
		return PingResult{Success: false, Message: "镜像站目标不符合访问策略", Endpoint: endpoint.String()}
	}

	resp, err := doRegistryGET(parent, endpoint.String(), "", policy, credential, 5*time.Second)
	if err != nil {
		telemetry.LogWarn(parent, "Registry request failed", "registry.ping.failed",
			"registry.ping", "provider.request.failed", err,
			slog.String("server.address", endpoint.Hostname()))
		return PingResult{Success: false, Message: "镜像站连接失败，请检查地址、网络或凭据", Endpoint: endpoint.String()}
	}
	defer drainAndCloseBody(resp.Body)

	success := resp.StatusCode >= 200 && resp.StatusCode < 400
	if resp.StatusCode == http.StatusUnauthorized {
		return PingResult{Success: false, StatusCode: resp.StatusCode, Message: "镜像站可访问，但凭据未通过认证", Endpoint: endpoint.String()}
	}
	return PingResult{Success: success, StatusCode: resp.StatusCode, Message: testMessage(success, resp.StatusCode), Endpoint: endpoint.String()}
}

func ParseEndpoint(endpoint string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, errors.New("invalid registry endpoint")
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return nil, errors.New("registry endpoint must use http or https")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, errors.New("registry endpoint must not contain credentials, query, or fragment")
	}
	return parsed, nil
}

func PingURL(endpoint string) (*url.URL, error) {
	parsed, err := normalizeOCIEndpoint(endpoint)
	if err != nil {
		return nil, err
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/v2/"
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed, nil
}

func testMessage(success bool, statusCode int) string {
	if success {
		return "镜像站连接成功"
	}
	return fmt.Sprintf("镜像站返回 HTTP %d", statusCode)
}
