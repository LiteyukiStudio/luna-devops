package notification

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"text/template"
	"time"
)

const maxRenderedTemplateBytes = 64 * 1024

func renderTemplate(name string, source string, data any) (string, error) {
	return renderTemplateWithTimeZone(name, source, data, "")
}

func renderTemplateWithTimeZone(name string, source string, data any, timeZone string) (string, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		return "", nil
	}
	tpl, err := template.New(name).Funcs(template.FuncMap{
		"default":      templateDefault,
		"details":      templateDetails,
		"detailsTitle": templateDetailsTitle,
		"feishuCard": func(event Event, locale string) (string, error) {
			return renderFeishuInteractiveCardJSON(event, locale, timeZone)
		},
		"json": templateJSON,
		"link": templateLink,
		"time": func(value any, layout string) string {
			return templateTimeInZone(value, layout, timeZone)
		},
		"truncate": templateTruncate,
	}).Option("missingkey=error").Parse(source)
	if err != nil {
		return "", err
	}
	var out bytes.Buffer
	if err := tpl.Execute(&out, data); err != nil {
		return "", err
	}
	if out.Len() > maxRenderedTemplateBytes {
		return "", fmt.Errorf("rendered template exceeds %d bytes", maxRenderedTemplateBytes)
	}
	return out.String(), nil
}

func renderMessage(event Event, tpl Template, secrets map[string]string) (RenderedMessage, error) {
	return renderMessageWithTimeZone(event, tpl, secrets, "")
}

func renderMessageWithTimeZone(event Event, tpl Template, secrets map[string]string, timeZone string) (RenderedMessage, error) {
	data := templateData{Event: event, Secrets: secrets}
	subject, err := renderTemplateWithTimeZone("subject", tpl.Subject, data, timeZone)
	if err != nil {
		return RenderedMessage{}, fmt.Errorf("render subject: %w", err)
	}
	body, err := renderTemplateWithTimeZone("body", tpl.Body, data, timeZone)
	if err != nil {
		return RenderedMessage{}, fmt.Errorf("render body: %w", err)
	}
	jsonBody, err := renderTemplateWithTimeZone("json", tpl.JSON, data, timeZone)
	if err != nil {
		return RenderedMessage{}, fmt.Errorf("render json: %w", err)
	}
	message := RenderedMessage{Subject: subject, Body: body}
	if strings.TrimSpace(jsonBody) != "" {
		var normalized any
		if err := json.Unmarshal([]byte(jsonBody), &normalized); err != nil {
			return RenderedMessage{}, fmt.Errorf("rendered body is not valid json: %w", err)
		}
		data, err := json.Marshal(normalized)
		if err != nil {
			return RenderedMessage{}, err
		}
		message.JSON = data
	}
	return message, nil
}

type templateData struct {
	Event   Event
	Secrets map[string]string
}

func templateDefault(fallback string, value any) string {
	text := strings.TrimSpace(fmt.Sprint(value))
	if text == "" || text == "<nil>" {
		return fallback
	}
	return text
}

func templateLink(links map[string]string, key string) string {
	if links == nil {
		return ""
	}
	return strings.TrimSpace(links[key])
}

func templateDetailsTitle(event Event) string {
	eventType := strings.TrimSpace(event.Type)
	if eventType == "" {
		eventType = "notification"
	}
	severity := strings.TrimSpace(event.Severity)
	if severity == "" {
		return eventType
	}
	return fmt.Sprintf("[%s] %s", severity, eventType)
}

func templateDetails(event Event, locale string) string {
	labels := detailLabelsForLocale(locale)
	lines := []string{
		fmt.Sprintf("%s: %s", labels.Message, fallbackText(event.Message)),
		fmt.Sprintf("%s: %s", labels.EventType, fallbackText(event.Type)),
		fmt.Sprintf("%s: %s", labels.Severity, fallbackText(event.Severity)),
		fmt.Sprintf("%s: %s", labels.Project, entityDetail(event.Project)),
		fmt.Sprintf("%s: %s", labels.Application, entityDetail(event.Application)),
		fmt.Sprintf("%s: %s", labels.Deployment, entityDetail(event.DeploymentTarget)),
		fmt.Sprintf("%s: %s", labels.Time, templateTime(event.OccurredAt, "2006-01-02 15:04:05 MST")),
	}
	if actor := actorDetail(event.Actor); actor != "" {
		lines = append(lines, fmt.Sprintf("%s: %s", labels.Actor, actor))
	}
	if event.Build.ID != "" || event.Build.Status != "" || event.Build.Image != "" || event.Build.Message != "" {
		lines = append(lines,
			"",
			labels.BuildSection,
			fmt.Sprintf("- %s: %s", labels.ID, fallbackText(event.Build.ID)),
			fmt.Sprintf("- %s: %s", labels.Status, fallbackText(event.Build.Status)),
			fmt.Sprintf("- %s: %s", labels.Image, fallbackText(event.Build.Image)),
			fmt.Sprintf("- %s: %s", labels.GitRef, fallbackText(event.Build.GitRef)),
			fmt.Sprintf("- %s: %s", labels.GitSHA, fallbackText(event.Build.GitSHA)),
			fmt.Sprintf("- %s: %s", labels.DetailMessage, fallbackText(event.Build.Message)),
		)
	}
	if event.Release.ID != "" || event.Release.Status != "" || event.Release.ImageRef != "" || event.Release.Message != "" {
		lines = append(lines,
			"",
			labels.ReleaseSection,
			fmt.Sprintf("- %s: %s", labels.ID, fallbackText(event.Release.ID)),
			fmt.Sprintf("- %s: %s", labels.Status, fallbackText(event.Release.Status)),
			fmt.Sprintf("- %s: %d", labels.Revision, event.Release.Revision),
			fmt.Sprintf("- %s: %s", labels.Image, fallbackText(event.Release.ImageRef)),
			fmt.Sprintf("- %s: %s", labels.DetailMessage, fallbackText(event.Release.Message)),
		)
	}
	if event.Hook.ID != "" || event.Hook.Name != "" || event.Hook.Status != "" || event.Hook.Message != "" {
		lines = append(lines,
			"",
			labels.HookSection,
			fmt.Sprintf("- %s: %s", labels.ID, fallbackText(event.Hook.ID)),
			fmt.Sprintf("- %s: %s", labels.Name, fallbackText(event.Hook.Name)),
			fmt.Sprintf("- %s: %s", labels.Phase, fallbackText(event.Hook.Phase)),
			fmt.Sprintf("- %s: %s", labels.Status, fallbackText(event.Hook.Status)),
			fmt.Sprintf("- %s: %s", labels.DetailMessage, fallbackText(event.Hook.Message)),
		)
	}
	if event.Gateway.ID != "" || event.Gateway.Domain != "" || event.Gateway.Status != "" || event.Gateway.Message != "" {
		lines = append(lines,
			"",
			labels.GatewaySection,
			fmt.Sprintf("- %s: %s", labels.ID, fallbackText(event.Gateway.ID)),
			fmt.Sprintf("- %s: %s", labels.Domain, fallbackText(event.Gateway.Domain)),
			fmt.Sprintf("- %s: %s", labels.Path, fallbackText(event.Gateway.Path)),
			fmt.Sprintf("- %s: %s", labels.Status, fallbackText(event.Gateway.Status)),
			fmt.Sprintf("- %s: %s", labels.DetailMessage, fallbackText(event.Gateway.Message)),
		)
	}
	if link := templateLink(event.Links, "primary"); link != "" {
		lines = append(lines, "", fmt.Sprintf("%s: %s", labels.DetailLink, link))
	}
	return strings.Join(lines, "\n")
}

type detailLabels struct {
	Message        string
	EventType      string
	Severity       string
	Project        string
	Application    string
	Deployment     string
	Time           string
	Actor          string
	BuildSection   string
	ReleaseSection string
	HookSection    string
	GatewaySection string
	ID             string
	Name           string
	Status         string
	Revision       string
	Image          string
	GitRef         string
	GitSHA         string
	Phase          string
	Domain         string
	Path           string
	DetailMessage  string
	DetailLink     string
}

func detailLabelsForLocale(locale string) detailLabels {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(locale)), "zh") {
		return detailLabels{
			Message:        "消息",
			EventType:      "事件类型",
			Severity:       "严重级别",
			Project:        "项目空间",
			Application:    "应用",
			Deployment:     "部署配置",
			Time:           "时间",
			Actor:          "操作人",
			BuildSection:   "构建详情",
			ReleaseSection: "发布详情",
			HookSection:    "Hook 详情",
			GatewaySection: "访问入口详情",
			ID:             "ID",
			Name:           "名称",
			Status:         "状态",
			Revision:       "版本",
			Image:          "镜像",
			GitRef:         "Git Ref",
			GitSHA:         "Git SHA",
			Phase:          "阶段",
			Domain:         "域名",
			Path:           "路径",
			DetailMessage:  "详情",
			DetailLink:     "详情链接",
		}
	}
	return detailLabels{
		Message:        "Message",
		EventType:      "Event type",
		Severity:       "Severity",
		Project:        "Project space",
		Application:    "Application",
		Deployment:     "Deployment target",
		Time:           "Time",
		Actor:          "Actor",
		BuildSection:   "Build details",
		ReleaseSection: "Release details",
		HookSection:    "Hook details",
		GatewaySection: "Gateway route details",
		ID:             "ID",
		Name:           "Name",
		Status:         "Status",
		Revision:       "Revision",
		Image:          "Image",
		GitRef:         "Git ref",
		GitSHA:         "Git SHA",
		Phase:          "Phase",
		Domain:         "Domain",
		Path:           "Path",
		DetailMessage:  "Detail",
		DetailLink:     "Detail link",
	}
}

func entityDetail(ref EntityRef) string {
	parts := make([]string, 0, 3)
	for _, part := range []string{ref.Name, ref.Identifier, ref.ID} {
		part = strings.TrimSpace(part)
		if part != "" {
			parts = append(parts, part)
		}
	}
	if len(parts) == 0 {
		return "-"
	}
	return strings.Join(parts, " / ")
}

func actorDetail(actor ActorContext) string {
	parts := make([]string, 0, 3)
	for _, part := range []string{actor.Name, actor.Email, actor.ID} {
		part = strings.TrimSpace(part)
		if part != "" {
			parts = append(parts, part)
		}
	}
	return strings.Join(parts, " / ")
}

func fallbackText(value any) string {
	text := strings.TrimSpace(fmt.Sprint(value))
	if text == "" || text == "<nil>" {
		return "-"
	}
	return text
}

func templateJSON(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func templateTime(value any, layout string) string {
	return templateTimeInZone(value, layout, "")
}

func templateTimeInZone(value any, layout string, timeZone string) string {
	if layout == "" {
		layout = time.RFC3339
	}
	var ts time.Time
	switch item := value.(type) {
	case time.Time:
		ts = item
	case *time.Time:
		if item != nil {
			ts = *item
		}
	default:
		return ""
	}
	if ts.IsZero() {
		return ""
	}
	if strings.TrimSpace(timeZone) != "" {
		return FormatTimeInZoneLayout(ts, timeZone, layout)
	}
	return ts.Format(layout)
}

func templateTruncate(value any, max int) string {
	text := fmt.Sprint(value)
	if max <= 0 || len([]rune(text)) <= max {
		return text
	}
	runes := []rune(text)
	return string(runes[:max])
}
