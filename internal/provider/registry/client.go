package registryprovider

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/security"
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

	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return PingResult{Success: false, Message: "镜像站请求创建失败", Endpoint: endpoint.String()}
	}
	if credential.Secret != "" {
		req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(credential.Username+":"+credential.Secret)))
	}

	resp, err := security.NewHTTPClient(policy, 5*time.Second).Do(req)
	if err != nil {
		slog.WarnContext(ctx, "registry.ping.failed",
			"error.type", fmt.Sprintf("%T", err),
			"registry.host", endpoint.Hostname(),
		)
		return PingResult{Success: false, Message: "镜像站连接失败，请检查地址、网络或凭据", Endpoint: endpoint.String()}
	}
	defer resp.Body.Close()

	success := resp.StatusCode >= 200 && resp.StatusCode < 400
	if resp.StatusCode == http.StatusUnauthorized {
		return PingResult{Success: false, StatusCode: resp.StatusCode, Message: "镜像站可访问，但凭据未通过认证", Endpoint: endpoint.String()}
	}
	return PingResult{Success: success, StatusCode: resp.StatusCode, Message: testMessage(success, resp.StatusCode), Endpoint: endpoint.String()}
}

func ParseEndpoint(endpoint string) (*url.URL, error) {
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, errors.New("invalid registry endpoint")
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return nil, errors.New("registry endpoint must use http or https")
	}
	return parsed, nil
}

func PingURL(endpoint string) (*url.URL, error) {
	parsed, err := ParseEndpoint(endpoint)
	if err != nil {
		return nil, err
	}
	path := strings.TrimRight(parsed.Path, "/")
	if strings.HasSuffix(path, "/v2") {
		parsed.Path = path + "/"
	} else {
		parsed.Path = path + "/v2/"
	}
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
