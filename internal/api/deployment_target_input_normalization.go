package api

import (
	"fmt"
	"net/http"
	"strings"
	"unicode"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
	"k8s.io/apimachinery/pkg/api/resource"
)

const (
	defaultBuildCPURequest     = "2"
	defaultBuildMemoryRequest  = "4Gi"
	defaultBuildTimeoutSeconds = 1800
	minBuildTimeoutSeconds     = 60
	maxBuildTimeoutSeconds     = 24 * 60 * 60
)

func normalizeDeploymentSourceType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "image":
		return "image"
	default:
		return "repository"
	}
}

func normalizeBuildTimeoutSeconds(ctx *gin.Context, value int) (int, bool) {
	normalized := normalizeBuildTimeoutSecondsValue(value)
	if normalized < minBuildTimeoutSeconds || normalized > maxBuildTimeoutSeconds {
		writeError(ctx, http.StatusBadRequest, "构建超时时间必须在 1 分钟到 24 小时之间")
		return 0, false
	}
	return normalized, true
}

func normalizeBuildTimeoutSecondsValue(value int) int {
	if value <= 0 {
		return defaultBuildTimeoutSeconds
	}
	return value
}

func normalizeDeploymentServicePorts(ctx *gin.Context, input []model.DeploymentServicePort, fallbackPort int) ([]model.DeploymentServicePort, bool) {
	if len(input) == 0 {
		input = []model.DeploymentServicePort{{Name: "http", Port: fallbackInt(fallbackPort, 8080)}}
	}
	if len(input) > 16 {
		writeError(ctx, http.StatusBadRequest, "服务端口最多配置 16 个")
		return nil, false
	}
	seenNames := map[string]bool{}
	seenPorts := map[int]bool{}
	ports := make([]model.DeploymentServicePort, 0, len(input))
	for index, item := range input {
		port := item.Port
		if port <= 0 || port > 65535 {
			writeError(ctx, http.StatusBadRequest, "服务端口必须在 1 到 65535 之间")
			return nil, false
		}
		if seenPorts[port] {
			writeError(ctx, http.StatusBadRequest, "服务端口不能重复")
			return nil, false
		}
		name := normalizeDeploymentServicePortName(item.Name, port, index)
		if seenNames[name] {
			writeError(ctx, http.StatusBadRequest, "服务端口名称不能重复")
			return nil, false
		}
		seenPorts[port] = true
		seenNames[name] = true
		ports = append(ports, model.DeploymentServicePort{Name: name, Port: port, AppProtocol: normalizeAppProtocol(item.AppProtocol)})
	}
	return ports, true
}

func normalizeAppProtocol(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 253 {
		return value[:253]
	}
	return value
}

func normalizeDeploymentServicePortName(value string, port int, index int) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var builder strings.Builder
	for _, char := range value {
		if unicode.IsLetter(char) || unicode.IsDigit(char) || char == '-' {
			builder.WriteRune(char)
		} else if char == '_' || unicode.IsSpace(char) {
			builder.WriteRune('-')
		}
	}
	name := strings.Trim(builder.String(), "-")
	if name == "" {
		if index == 0 {
			name = "http"
		} else {
			name = fmt.Sprintf("port-%d", port)
		}
	}
	if len(name) > 63 {
		name = strings.Trim(name[:63], "-")
	}
	return name
}

func normalizeBuildResourceQuantity(ctx *gin.Context, value string, fallbackValue string, label string) (string, bool) {
	normalized, err := normalizeBuildResourceQuantityValue(value, fallbackValue, label)
	if err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return "", false
	}
	return normalized, true
}

func normalizeBuildResourceQuantityValue(value string, fallbackValue string, label string) (string, error) {
	normalized := fallback(strings.TrimSpace(value), fallbackValue)
	quantity, err := resource.ParseQuantity(normalized)
	if err != nil || quantity.Sign() <= 0 {
		return "", fmt.Errorf("%s必须是有效的正数资源规格", label)
	}
	return normalized, nil
}

func normalizeOptionalResourceQuantity(ctx *gin.Context, value string, label string) (string, bool) {
	normalized := strings.TrimSpace(value)
	if normalized == "" {
		return "", true
	}
	quantity, err := resource.ParseQuantity(normalized)
	if err != nil || quantity.Sign() <= 0 {
		writeError(ctx, http.StatusBadRequest, label+"必须是有效的正数资源规格")
		return "", false
	}
	return normalized, true
}
