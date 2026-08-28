package notification

import (
	"bytes"
	"errors"
	"fmt"
	"html/template"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	maxRenderedEmailDigestBytes = 512 * 1024
	maxEmailDigestTextRunes     = 2000
	maxEmailDigestTitleRunes    = 160
	maxEmailDigestSubjectRunes  = 240
	maxEmailDigestURLRunes      = 4096
)

type emailDigestView struct {
	Lang       string
	Title      string
	Intro      string
	RangeLabel string
	Range      string
	CTA        string
	URLLabel   string
	Events     []emailDigestEventView
	Footer     string
}

type emailDigestEventView struct {
	Number        int
	Title         string
	Severity      string
	SeverityLabel string
	Message       string
	Fields        []emailDigestField
	Details       []emailDigestField
	PrimaryURL    string
}

type emailDigestField struct {
	Label string
	Value string
}

type emailDigestLabels struct {
	Details         detailLabels
	DigestTitle     func(int) string
	Intro           string
	Range           string
	EventID         string
	CorrelationID   string
	Host            string
	ExpiresAt       string
	Issuer          string
	CTA             string
	URL             string
	Footer          string
	SeverityInfo    string
	SeverityWarning string
	SeverityError   string
}

// RenderPersonalEmailDigest renders one personal email containing one or more
// platform events. The HTML part uses html/template so event-provided content
// is escaped for the email document context.
func RenderPersonalEmailDigest(events []Event, locale string) (RenderedMessage, error) {
	if len(events) == 0 {
		return RenderedMessage{}, errors.New("personal email digest requires at least one event")
	}

	labels, lang := digestLabelsForLocale(locale)
	ordered := append([]Event(nil), events...)
	sort.SliceStable(ordered, func(left, right int) bool {
		return ordered[left].OccurredAt.Before(ordered[right].OccurredAt)
	})

	view := emailDigestView{
		Lang:       lang,
		Title:      labels.DigestTitle(len(ordered)),
		Intro:      labels.Intro,
		RangeLabel: labels.Range,
		Range:      digestTimeRange(ordered),
		CTA:        labels.CTA,
		URLLabel:   labels.URL,
		Events:     make([]emailDigestEventView, 0, len(ordered)),
		Footer:     labels.Footer,
	}
	for index, event := range ordered {
		view.Events = append(view.Events, digestEventView(index+1, event, labels, lang))
	}

	var html bytes.Buffer
	if err := personalEmailDigestTemplate.Execute(&html, view); err != nil {
		return RenderedMessage{}, fmt.Errorf("render personal email digest html: %w", err)
	}
	body := renderPersonalEmailDigestText(view)
	if len(body) > maxRenderedEmailDigestBytes || html.Len() > maxRenderedEmailDigestBytes {
		return RenderedMessage{}, fmt.Errorf("rendered email digest exceeds %d bytes", maxRenderedEmailDigestBytes)
	}

	return RenderedMessage{
		Subject:  digestSubject(ordered, labels, lang),
		Body:     body,
		HTMLBody: html.String(),
	}, nil
}

func digestEventView(number int, event Event, labels emailDigestLabels, lang string) emailDigestEventView {
	occurredAt := digestTime(event.OccurredAt)
	view := emailDigestEventView{
		Number:        number,
		Title:         digestEventTypeLabel(event.Type, lang),
		Severity:      normalizedDigestSeverity(event.Severity),
		SeverityLabel: digestSeverityLabel(event.Severity, labels),
		Message:       templateTruncate(fallbackText(event.Message), maxEmailDigestTextRunes),
		PrimaryURL:    absoluteHTTPURL(event.Links["primary"]),
	}
	view.Fields = appendDigestField(view.Fields, labels.Details.Project, entityDigestValue(event.Project))
	view.Fields = appendDigestField(view.Fields, labels.Details.Application, entityDigestValue(event.Application))
	view.Fields = appendDigestField(view.Fields, labels.Details.Deployment, entityDigestValue(event.DeploymentTarget))
	view.Fields = appendDigestField(view.Fields, labels.Details.Actor, actorDigestValue(event.Actor))
	view.Fields = appendDigestField(view.Fields, labels.Details.Time, occurredAt)
	view.Fields = appendDigestField(view.Fields, labels.CorrelationID, event.CorrelationID)
	view.Fields = appendDigestField(view.Fields, labels.EventID, event.ID)
	view.Details = digestEventDetails(event, labels)
	return view
}

func digestEventDetails(event Event, labels emailDigestLabels) []emailDigestField {
	details := make([]emailDigestField, 0, 8)
	switch {
	case event.Type == "build.failed":
		details = appendDigestField(details, labels.Details.ID, event.Build.ID)
		details = appendDigestField(details, labels.Details.Status, event.Build.Status)
		details = appendDigestField(details, labels.Details.Image, event.Build.Image)
		details = appendDigestField(details, labels.Details.GitRef, event.Build.GitRef)
		details = appendDigestField(details, labels.Details.GitSHA, event.Build.GitSHA)
		details = appendDistinctDetail(details, labels.Details.DetailMessage, event.Build.Message, event.Message)
	case event.Type == "release.failed":
		details = appendDigestField(details, labels.Details.ID, event.Release.ID)
		details = appendDigestField(details, labels.Details.Status, event.Release.Status)
		if event.Release.Revision > 0 {
			details = appendDigestField(details, labels.Details.Revision, strconv.Itoa(event.Release.Revision))
		}
		details = appendDigestField(details, labels.Details.Image, event.Release.ImageRef)
		details = appendDistinctDetail(details, labels.Details.DetailMessage, event.Release.Message, event.Message)
	case event.Type == "hook.failed":
		details = appendDigestField(details, labels.Details.ID, event.Hook.ID)
		details = appendDigestField(details, labels.Details.Name, event.Hook.Name)
		details = appendDigestField(details, labels.Details.Phase, event.Hook.Phase)
		details = appendDigestField(details, labels.Details.Status, event.Hook.Status)
		details = appendDistinctDetail(details, labels.Details.DetailMessage, event.Hook.Message, event.Message)
	case event.Type == "certificate.failed" || event.Type == "certificate.expired":
		details = appendDigestField(details, labels.Details.ID, event.Certificate.RouteID)
		details = appendDigestField(details, labels.Host, event.Certificate.Host)
		details = appendDigestField(details, labels.Details.Status, event.Certificate.Status)
		if event.Certificate.NotAfter != nil {
			details = appendDigestField(details, labels.ExpiresAt, digestTime(*event.Certificate.NotAfter))
		}
		details = appendDigestField(details, labels.Issuer, joinNonEmpty(" / ", event.Certificate.IssuerKind, event.Certificate.IssuerName))
		details = appendDistinctDetail(details, labels.Details.DetailMessage, event.Certificate.Message, event.Message)
	case event.Type == "gateway.apply_failed":
		details = appendDigestField(details, labels.Details.ID, event.Gateway.ID)
		details = appendDigestField(details, labels.Details.Domain, event.Gateway.Domain)
		details = appendDigestField(details, labels.Details.Path, event.Gateway.Path)
		details = appendDigestField(details, labels.Details.Status, event.Gateway.Status)
		details = appendDistinctDetail(details, labels.Details.DetailMessage, event.Gateway.Message, event.Message)
	}
	return details
}

func appendDigestField(fields []emailDigestField, label string, value string) []emailDigestField {
	value = strings.TrimSpace(value)
	if value == "" {
		return fields
	}
	return append(fields, emailDigestField{Label: label, Value: templateTruncate(value, maxEmailDigestTextRunes)})
}

func appendDistinctDetail(fields []emailDigestField, label string, value string, message string) []emailDigestField {
	if strings.TrimSpace(value) == strings.TrimSpace(message) {
		return fields
	}
	return appendDigestField(fields, label, value)
}

func renderPersonalEmailDigestText(view emailDigestView) string {
	var out strings.Builder
	fmt.Fprintf(&out, "Luna DevOps — %s\n", view.Title)
	if view.Range != "" {
		fmt.Fprintf(&out, "%s: %s\n", view.RangeLabel, view.Range)
	}
	for _, event := range view.Events {
		fmt.Fprintf(&out, "\n%d. [%s] %s\n", event.Number, event.SeverityLabel, event.Title)
		fmt.Fprintln(&out, event.Message)
		for _, field := range event.Fields {
			fmt.Fprintf(&out, "%s: %s\n", field.Label, field.Value)
		}
		for _, field := range event.Details {
			fmt.Fprintf(&out, "%s: %s\n", field.Label, field.Value)
		}
		if event.PrimaryURL != "" {
			fmt.Fprintf(&out, "%s: %s\n", view.URLLabel, event.PrimaryURL)
		}
	}
	fmt.Fprintf(&out, "\n%s\n", view.Footer)
	return out.String()
}

func digestSubject(events []Event, labels emailDigestLabels, lang string) string {
	title := labels.DigestTitle(len(events))
	if len(events) == 1 {
		title = digestEventTypeLabel(events[0].Type, lang)
	}
	scope := sharedDigestScope(events)
	if scope == "" {
		return templateTruncate("Luna DevOps · "+title, maxEmailDigestSubjectRunes)
	}
	return templateTruncate("Luna DevOps · "+title+" · "+scope, maxEmailDigestSubjectRunes)
}

func sharedDigestScope(events []Event) string {
	var scope string
	for _, event := range events {
		current := strings.TrimSpace(event.Application.Name)
		if current == "" {
			current = strings.TrimSpace(event.Project.Name)
		}
		if current == "" || scope != "" && current != scope {
			return ""
		}
		scope = current
	}
	return templateTruncate(scope, maxEmailDigestTitleRunes)
}

func digestTimeRange(events []Event) string {
	var first time.Time
	var last time.Time
	for _, event := range events {
		if event.OccurredAt.IsZero() {
			continue
		}
		if first.IsZero() || event.OccurredAt.Before(first) {
			first = event.OccurredAt
		}
		if last.IsZero() || event.OccurredAt.After(last) {
			last = event.OccurredAt
		}
	}
	if first.IsZero() {
		return ""
	}
	if first.Equal(last) {
		return digestTime(first)
	}
	return digestTime(first) + " — " + digestTime(last)
}

func digestTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format("2006-01-02 15:04:05 UTC")
}

func entityDigestValue(ref EntityRef) string {
	name := strings.TrimSpace(ref.Name)
	identifier := strings.TrimSpace(ref.Identifier)
	switch {
	case name != "" && identifier != "":
		return name + " (" + identifier + ")"
	case name != "":
		return name
	case identifier != "":
		return identifier
	default:
		return strings.TrimSpace(ref.ID)
	}
}

func actorDigestValue(actor ActorContext) string {
	return joinNonEmpty(" / ", actor.Name, actor.Email)
}

func joinNonEmpty(separator string, values ...string) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			parts = append(parts, value)
		}
	}
	return strings.Join(parts, separator)
}

func absoluteHTTPURL(value string) string {
	value = strings.TrimSpace(value)
	if len([]rune(value)) > maxEmailDigestURLRunes {
		return ""
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || parsed.User != nil {
		return ""
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return ""
	}
	return parsed.String()
}

func normalizedDigestSeverity(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case SeverityInfo:
		return SeverityInfo
	case SeverityWarning:
		return SeverityWarning
	default:
		return SeverityError
	}
}

func digestSeverityLabel(value string, labels emailDigestLabels) string {
	switch normalizedDigestSeverity(value) {
	case SeverityInfo:
		return labels.SeverityInfo
	case SeverityWarning:
		return labels.SeverityWarning
	default:
		return labels.SeverityError
	}
}

func digestLabelsForLocale(locale string) (emailDigestLabels, string) {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(locale)), "zh") {
		return emailDigestLabels{
			Details:         detailLabelsForLocale("zh"),
			DigestTitle:     func(count int) string { return fmt.Sprintf("%d 条通知", count) },
			Intro:           "以下是本次汇总的 Luna DevOps 通知。",
			Range:           "通知时间范围",
			EventID:         "事件 ID",
			CorrelationID:   "关联 ID",
			Host:            "主机",
			ExpiresAt:       "到期时间",
			Issuer:          "签发者",
			CTA:             "查看详情",
			URL:             "可访问地址",
			Footer:          "这是一封由 Luna DevOps 自动发送的通知邮件。",
			SeverityInfo:    "信息",
			SeverityWarning: "警告",
			SeverityError:   "错误",
		}, "zh-CN"
	}
	return emailDigestLabels{
		Details:         detailLabelsForLocale("en"),
		DigestTitle:     func(count int) string { return fmt.Sprintf("%d notifications", count) },
		Intro:           "Here is your Luna DevOps notification digest.",
		Range:           "Notification time range",
		EventID:         "Event ID",
		CorrelationID:   "Correlation ID",
		Host:            "Host",
		ExpiresAt:       "Expires at",
		Issuer:          "Issuer",
		CTA:             "View details",
		URL:             "Accessible URL",
		Footer:          "This is an automated notification from Luna DevOps.",
		SeverityInfo:    "Info",
		SeverityWarning: "Warning",
		SeverityError:   "Error",
	}, "en"
}

func digestEventTypeLabel(eventType string, lang string) string {
	labels := eventTypeLabelsEN
	if lang == "zh-CN" {
		labels = eventTypeLabelsZH
	}
	if label := labels[strings.TrimSpace(eventType)]; label != "" {
		return label
	}
	return templateTruncate(fallbackText(eventType), maxEmailDigestTitleRunes)
}

var eventTypeLabelsZH = map[string]string{
	"build.failed":         "构建失败",
	"release.failed":       "发布失败",
	"hook.failed":          "Hook 失败",
	"gateway.apply_failed": "访问入口应用失败",
	"certificate.failed":   "证书签发失败",
	"certificate.expired":  "证书已过期",
}

var eventTypeLabelsEN = map[string]string{
	"build.failed":         "Build failed",
	"release.failed":       "Release failed",
	"hook.failed":          "Hook failed",
	"gateway.apply_failed": "Gateway route failed",
	"certificate.failed":   "Certificate failed",
	"certificate.expired":  "Certificate expired",
}

var personalEmailDigestTemplate = template.Must(template.New("personal-email-digest").Parse(`<!doctype html>
<html lang="{{.Lang}}">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Luna DevOps — {{.Title}}</title>
</head>
<body style="margin:0;padding:0;background:#f5f7fb;color:#172033;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Arial,sans-serif;">
  <table role="presentation" width="100%" cellspacing="0" cellpadding="0" border="0" style="width:100%;background:#f5f7fb;">
    <tr>
      <td align="center" style="padding:24px 12px;">
        <table role="presentation" width="640" cellspacing="0" cellpadding="0" border="0" style="width:100%;max-width:640px;background:#ffffff;border-radius:12px;">
          <tr>
            <td style="padding:28px 32px 20px 32px;border-bottom:1px solid #e5e7eb;">
              <p style="margin:0 0 8px 0;color:#2563eb;font-size:14px;font-weight:700;letter-spacing:.02em;">Luna DevOps</p>
              <h1 style="margin:0;color:#111827;font-size:24px;line-height:1.35;font-weight:700;">{{.Title}}</h1>
              <p style="margin:10px 0 0 0;color:#4b5563;font-size:14px;line-height:1.6;">{{.Intro}}</p>
              {{if .Range}}<p style="margin:6px 0 0 0;color:#6b7280;font-size:13px;line-height:1.5;">{{.RangeLabel}}：{{.Range}}</p>{{end}}
            </td>
          </tr>
          {{range .Events}}
          <tr>
            <td style="padding:24px 32px;border-bottom:1px solid #e5e7eb;">
              <table role="presentation" width="100%" cellspacing="0" cellpadding="0" border="0">
                <tr>
                  <td style="vertical-align:top;padding-right:12px;">
                    <h2 style="margin:0;color:#111827;font-size:18px;line-height:1.45;font-weight:700;">{{.Number}}. {{.Title}}</h2>
                  </td>
                  <td align="right" style="vertical-align:top;white-space:nowrap;">
                    {{if eq .Severity "info"}}<span style="display:inline-block;padding:4px 9px;border-radius:999px;background:#dbeafe;color:#1d4ed8;font-size:12px;font-weight:700;">{{.SeverityLabel}}</span>
                    {{else if eq .Severity "warning"}}<span style="display:inline-block;padding:4px 9px;border-radius:999px;background:#fef3c7;color:#92400e;font-size:12px;font-weight:700;">{{.SeverityLabel}}</span>
                    {{else}}<span style="display:inline-block;padding:4px 9px;border-radius:999px;background:#fee2e2;color:#b91c1c;font-size:12px;font-weight:700;">{{.SeverityLabel}}</span>{{end}}
                  </td>
                </tr>
              </table>
              <p style="margin:12px 0 16px 0;color:#1f2937;font-size:15px;line-height:1.65;word-break:break-word;">{{.Message}}</p>
              <table role="presentation" width="100%" cellspacing="0" cellpadding="0" border="0" style="font-size:13px;line-height:1.55;">
                {{range .Fields}}<tr><td style="width:132px;padding:4px 12px 4px 0;color:#6b7280;vertical-align:top;">{{.Label}}</td><td style="padding:4px 0;color:#1f2937;word-break:break-word;">{{.Value}}</td></tr>{{end}}
                {{range .Details}}<tr><td style="width:132px;padding:4px 12px 4px 0;color:#6b7280;vertical-align:top;">{{.Label}}</td><td style="padding:4px 0;color:#1f2937;word-break:break-word;">{{.Value}}</td></tr>{{end}}
              </table>
              {{if .PrimaryURL}}
              <table role="presentation" cellspacing="0" cellpadding="0" border="0" style="margin-top:18px;"><tr><td style="border-radius:7px;background:#2563eb;"><a href="{{.PrimaryURL}}" style="display:inline-block;padding:10px 16px;color:#ffffff;text-decoration:none;font-size:14px;font-weight:700;">{{$.CTA}}</a></td></tr></table>
              <p style="margin:12px 0 0 0;color:#6b7280;font-size:12px;line-height:1.5;word-break:break-all;">{{$.URLLabel}}：<a href="{{.PrimaryURL}}" style="color:#2563eb;text-decoration:underline;">{{.PrimaryURL}}</a></p>
              {{end}}
            </td>
          </tr>
          {{end}}
          <tr><td style="padding:18px 32px;color:#6b7280;font-size:12px;line-height:1.5;">{{.Footer}}</td></tr>
        </table>
      </td>
    </tr>
  </table>
</body>
</html>`))
