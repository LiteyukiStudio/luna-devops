package notification

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

type feishuInteractiveEnvelope struct {
	MsgType string                `json:"msg_type"`
	Card    feishuInteractiveCard `json:"card"`
}

type feishuInteractiveCard struct {
	Config   feishuCardConfig   `json:"config"`
	Header   feishuCardHeader   `json:"header"`
	Elements []feishuCardWidget `json:"elements"`
}

type feishuCardConfig struct {
	WideScreenMode bool `json:"wide_screen_mode"`
}

type feishuCardHeader struct {
	Template string         `json:"template"`
	Title    feishuCardText `json:"title"`
}

type feishuCardText struct {
	Tag     string `json:"tag"`
	Content string `json:"content"`
}

type feishuCardField struct {
	IsShort bool           `json:"is_short"`
	Text    feishuCardText `json:"text"`
}

type feishuCardButton struct {
	Tag  string         `json:"tag"`
	Text feishuCardText `json:"text"`
	Type string         `json:"type"`
	URL  string         `json:"url"`
}

type feishuCardWidget struct {
	Tag      string             `json:"tag"`
	Text     *feishuCardText    `json:"text,omitempty"`
	Fields   []feishuCardField  `json:"fields,omitempty"`
	Actions  []feishuCardButton `json:"actions,omitempty"`
	Elements []feishuCardText   `json:"elements,omitempty"`
}

func feishuHeaderTemplateColor(event Event) string {
	eventType := strings.ToLower(strings.TrimSpace(event.Type))
	switch {
	case strings.HasSuffix(eventType, ".recovered"),
		strings.HasSuffix(eventType, ".succeeded"),
		strings.HasSuffix(eventType, ".applied"),
		strings.HasSuffix(eventType, ".issued"),
		strings.HasSuffix(eventType, ".renewed"):
		return "green"
	}
	switch strings.ToLower(strings.TrimSpace(event.Severity)) {
	case SeverityError:
		return "red"
	case SeverityWarning:
		return "orange"
	default:
		return "blue"
	}
}

func cleanEntityLabel(ref EntityRef) string {
	if name := strings.TrimSpace(ref.Name); name != "" {
		return name
	}
	if identifier := strings.TrimSpace(ref.Identifier); identifier != "" {
		return identifier
	}
	return ""
}

func humanCardHeaderTitle(event Event, lang string) string {
	typeLabel := digestEventTypeLabel(event.Type, lang)
	scope := cleanEntityLabel(event.Application)
	if scope == "" {
		scope = cleanEntityLabel(event.Project)
	}
	target := cleanEntityLabel(event.DeploymentTarget)
	title := typeLabel
	if scope != "" && target != "" {
		title = fmt.Sprintf("%s · %s (%s)", typeLabel, scope, target)
	} else if scope != "" {
		title = fmt.Sprintf("%s · %s", typeLabel, scope)
	}
	return templateTruncate(title, 120)
}

func sanitizeLarkMarkdownValue(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "-"
	}
	trimmed = strings.ReplaceAll(trimmed, "```", "'''")
	return templateTruncate(trimmed, 600)
}

func renderFeishuInteractiveCardJSON(event Event, locale string, timeZone string) (string, error) {
	isZH := strings.HasPrefix(strings.ToLower(strings.TrimSpace(locale)), "zh")
	lang := "en"
	if isZH {
		lang = "zh-CN"
	}
	labels, _ := digestLabelsForLocale(lang)
	tz := NormalizeTimeZone(timeZone)
	occurredText := FormatTimeInZoneLayout(event.OccurredAt, tz, "2006-01-02 15:04:05 MST")
	if occurredText == "" {
		occurredText = "-"
	}

	fields := make([]feishuCardField, 0, 8)
	addField := func(label string, value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		fields = append(fields, feishuCardField{
			IsShort: true,
			Text: feishuCardText{
				Tag:     "lark_md",
				Content: fmt.Sprintf("**%s**\n%s", label, sanitizeLarkMarkdownValue(value)),
			},
		})
	}

	addField(labels.Details.Project, cleanEntityLabel(event.Project))
	addField(labels.Details.Application, cleanEntityLabel(event.Application))
	addField(labels.Details.Deployment, cleanEntityLabel(event.DeploymentTarget))
	addField(labels.Details.Time, occurredText)

	var codeSnippet string
	switch {
	case strings.HasPrefix(event.Type, "runtime."):
		podContainer := joinNonEmpty(" / ", event.Runtime.PodName, event.Runtime.ContainerName)
		if isZH {
			addField("异常实例 / 容器", podContainer)
		} else {
			addField("Pod / Container", podContainer)
		}
		if event.Runtime.DesiredReplicas > 0 || event.Runtime.ReadyReplicas > 0 {
			replicaSummary := fmt.Sprintf("%d / %d Ready", event.Runtime.ReadyReplicas, event.Runtime.DesiredReplicas)
			if isZH {
				addField("副本就绪", replicaSummary)
			} else {
				addField("Replicas", replicaSummary)
			}
		}
		if event.Runtime.RestartCount > 0 {
			if isZH {
				addField("重启次数", strconv.Itoa(int(event.Runtime.RestartCount)))
			} else {
				addField("Restarts", strconv.Itoa(int(event.Runtime.RestartCount)))
			}
		}
		if event.Runtime.ReminderCount > 0 {
			if isZH {
				addField("持续提醒", fmt.Sprintf("第 %d 次提醒", event.Runtime.ReminderCount))
			} else {
				addField("Reminder", fmt.Sprintf("#%d", event.Runtime.ReminderCount))
			}
		}
		if event.Runtime.DurationSeconds > 0 && event.Type == "runtime.recovered" {
			if isZH {
				addField("故障持续时长", fmt.Sprintf("%ds", event.Runtime.DurationSeconds))
			} else {
				addField("Incident Duration", fmt.Sprintf("%ds", event.Runtime.DurationSeconds))
			}
		}
		diagLines := make([]string, 0, 4)
		if reason := strings.TrimSpace(event.Runtime.Reason); reason != "" {
			diagLines = append(diagLines, "Reason: "+reason)
		}
		if event.Runtime.ExitCode != nil {
			diagLines = append(diagLines, fmt.Sprintf("ExitCode: %d", *event.Runtime.ExitCode))
		}
		if detail := strings.TrimSpace(event.Runtime.DiagnosticDetail); detail != "" {
			diagLines = append(diagLines, detail)
		} else if msg := strings.TrimSpace(event.Message); msg != "" {
			diagLines = append(diagLines, msg)
		}
		if event.Type != "runtime.recovered" && len(diagLines) > 0 {
			codeSnippet = strings.Join(diagLines, "\n")
		}
	case strings.HasPrefix(event.Type, "build."):
		gitSummary := event.Build.GitRef
		if len(event.Build.GitSHA) >= 8 {
			gitSummary = joinNonEmpty(" @ ", event.Build.GitRef, event.Build.GitSHA[:8])
		}
		addField(labels.Details.GitRef, gitSummary)
		addField(labels.Details.Image, event.Build.Image)
		if strings.TrimSpace(event.Build.Message) != "" && event.Severity != SeverityInfo {
			codeSnippet = event.Build.Message
		}
	case strings.HasPrefix(event.Type, "release."):
		if event.Release.Revision > 0 {
			addField(labels.Details.Revision, "#"+strconv.Itoa(event.Release.Revision))
		}
		addField(labels.Details.Image, event.Release.ImageRef)
		if strings.TrimSpace(event.Release.Message) != "" && event.Severity != SeverityInfo {
			codeSnippet = event.Release.Message
		}
	case strings.HasPrefix(event.Type, "certificate."):
		addField(labels.Host, firstNonEmptyString(event.Certificate.Host, event.Gateway.Domain))
		if event.Certificate.NotAfter != nil {
			addField(labels.ExpiresAt, FormatTimeInZoneLayout(*event.Certificate.NotAfter, tz, "2006-01-02 15:04 MST"))
		}
		addField(labels.Issuer, joinNonEmpty(" / ", event.Certificate.IssuerKind, event.Certificate.IssuerName))
		if strings.TrimSpace(event.Certificate.Message) != "" && event.Severity != SeverityInfo {
			codeSnippet = event.Certificate.Message
		}
	case strings.HasPrefix(event.Type, "gateway."):
		addField(labels.Details.Domain, joinNonEmpty("", event.Gateway.Domain, event.Gateway.Path))
		if strings.TrimSpace(event.Gateway.Message) != "" && event.Severity != SeverityInfo {
			codeSnippet = event.Gateway.Message
		}
	case strings.HasPrefix(event.Type, "hook."):
		addField(labels.Details.Name, event.Hook.Name)
		addField(labels.Details.Phase, event.Hook.Phase)
		if strings.TrimSpace(event.Hook.Message) != "" && event.Severity != SeverityInfo {
			codeSnippet = event.Hook.Message
		}
	}

	if actor := actorDigestValue(event.Actor); actor != "" {
		addField(labels.Details.Actor, actor)
	}

	elements := make([]feishuCardWidget, 0, 5)
	summary := strings.TrimSpace(event.Message)
	if summary != "" {
		elements = append(elements, feishuCardWidget{
			Tag: "div",
			Text: &feishuCardText{
				Tag:     "lark_md",
				Content: sanitizeLarkMarkdownValue(summary),
			},
		})
	}
	if len(fields) > 0 {
		elements = append(elements, feishuCardWidget{
			Tag:    "div",
			Fields: fields,
		})
	}
	if strings.TrimSpace(codeSnippet) != "" && strings.TrimSpace(codeSnippet) != summary {
		diagTitle := "Diagnostic Detail"
		if isZH {
			diagTitle = "异常诊断摘要"
		}
		elements = append(elements, feishuCardWidget{
			Tag: "div",
			Text: &feishuCardText{
				Tag:     "lark_md",
				Content: fmt.Sprintf("**%s**\n```text\n%s\n```", diagTitle, sanitizeLarkMarkdownValue(codeSnippet)),
			},
		})
	} else if event.Severity == SeverityError && strings.HasPrefix(event.Type, "runtime.") && strings.TrimSpace(codeSnippet) != "" {
		diagTitle := "Diagnostic Detail"
		if isZH {
			diagTitle = "异常诊断摘要"
		}
		elements = append(elements, feishuCardWidget{
			Tag: "div",
			Text: &feishuCardText{
				Tag:     "lark_md",
				Content: fmt.Sprintf("**%s**\n```text\n%s\n```", diagTitle, sanitizeLarkMarkdownValue(codeSnippet)),
			},
		})
	}

	if primaryURL := absoluteHTTPURL(templateLink(event.Links, "primary")); primaryURL != "" {
		ctaLabel := "View Details"
		if isZH {
			ctaLabel = "打开控制台排障"
			if event.Severity == SeverityInfo {
				ctaLabel = "查看详情"
			}
		}
		elements = append(elements,
			feishuCardWidget{Tag: "hr"},
			feishuCardWidget{
				Tag: "action",
				Actions: []feishuCardButton{
					{
						Tag:  "button",
						Type: "primary",
						URL:  primaryURL,
						Text: feishuCardText{
							Tag:     "plain_text",
							Content: ctaLabel,
						},
					},
				},
			},
		)
	}

	footerText := "Luna DevOps"
	if event.Type != "" {
		footerText = fmt.Sprintf("Luna DevOps · %s", event.Type)
	}
	elements = append(elements, feishuCardWidget{
		Tag: "note",
		Elements: []feishuCardText{
			{Tag: "plain_text", Content: footerText},
		},
	})

	envelope := feishuInteractiveEnvelope{
		MsgType: "interactive",
		Card: feishuInteractiveCard{
			Config:   feishuCardConfig{WideScreenMode: true},
			Header:   feishuCardHeader{Template: feishuHeaderTemplateColor(event), Title: feishuCardText{Tag: "plain_text", Content: humanCardHeaderTitle(event, lang)}},
			Elements: elements,
		},
	}
	encoded, err := json.MarshalIndent(envelope, "", "  ")
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}
