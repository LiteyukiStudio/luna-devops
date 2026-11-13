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
	return RenderPersonalEmailDigestWithZone(events, locale, "UTC")
}

// RenderPersonalEmailDigestWithZone renders a batched personal email digest
// formatting timestamps in the requested IANA timeZone (defaulting to Asia/Shanghai when empty).
func RenderPersonalEmailDigestWithZone(events []Event, locale string, timeZone string) (RenderedMessage, error) {
	if len(events) == 0 {
		return RenderedMessage{}, errors.New("personal email digest requires at least one event")
	}
	tz := NormalizeTimeZone(timeZone)
	if strings.EqualFold(strings.TrimSpace(timeZone), "UTC") {
		tz = "UTC"
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
		Range:      digestTimeRangeInZone(ordered, tz),
		CTA:        labels.CTA,
		URLLabel:   labels.URL,
		Events:     make([]emailDigestEventView, 0, len(ordered)),
		Footer:     labels.Footer,
	}
	for index, event := range ordered {
		view.Events = append(view.Events, digestEventViewInZone(index+1, event, labels, lang, tz))
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

type singleEventEmailView struct {
	Lang          string
	AccentColor   string
	BadgeBg       string
	BadgeText     string
	Title         string
	ScopeSubtitle string
	SeverityLabel string
	Message       string
	Fields        []emailDigestField
	Details       []emailDigestField
	DiagTitle     string
	DiagSnippet   string
	PrimaryURL    string
	CTA           string
	URLLabel      string
	Footer        string
	EventType     string
}

// RenderEventEmail renders a single notification event as a responsive HTML +
// plain-text multipart/alternative email using the configured IANA timezone.
func RenderEventEmail(event Event, locale string, timeZone string) (RenderedMessage, error) {
	labels, lang := digestLabelsForLocale(locale)
	isZH := lang == "zh-CN"
	tz := NormalizeTimeZone(timeZone)
	accentColor, badgeBg, badgeText := emailToneColors(event)

	title := digestEventTypeLabel(event.Type, lang)
	scopeParts := make([]string, 0, 3)
	if p := cleanEntityLabel(event.Project); p != "" {
		scopeParts = append(scopeParts, p)
	}
	if a := cleanEntityLabel(event.Application); a != "" {
		scopeParts = append(scopeParts, a)
	}
	if d := cleanEntityLabel(event.DeploymentTarget); d != "" {
		scopeParts = append(scopeParts, d)
	}

	itemView := digestEventViewInZone(1, event, labels, lang, tz)
	// Filter out internal EventID/CorrelationID rows from clean single-alert fields unless needed
	cleanFields := make([]emailDigestField, 0, len(itemView.Fields))
	for _, f := range itemView.Fields {
		if f.Label == labels.EventID || f.Label == labels.CorrelationID {
			continue
		}
		cleanFields = append(cleanFields, f)
	}

	diagTitle := "Diagnostic Detail"
	if isZH {
		diagTitle = "异常诊断摘要"
	}
	diagSnippet := singleEventDiagnosticSnippet(event)

	cta := labels.CTA
	if isZH && event.Severity == SeverityError {
		cta = "打开控制台排障"
	}

	view := singleEventEmailView{
		Lang:          lang,
		AccentColor:   accentColor,
		BadgeBg:       badgeBg,
		BadgeText:     badgeText,
		Title:         title,
		ScopeSubtitle: strings.Join(scopeParts, " / "),
		SeverityLabel: itemView.SeverityLabel,
		Message:       itemView.Message,
		Fields:        cleanFields,
		Details:       itemView.Details,
		DiagTitle:     diagTitle,
		DiagSnippet:   diagSnippet,
		PrimaryURL:    itemView.PrimaryURL,
		CTA:           cta,
		URLLabel:      labels.URL,
		Footer:        labels.Footer,
		EventType:     strings.TrimSpace(event.Type),
	}

	var html bytes.Buffer
	if err := singleEventEmailTemplate.Execute(&html, view); err != nil {
		return RenderedMessage{}, fmt.Errorf("render single event email html: %w", err)
	}
	body := renderSingleEventEmailText(view)
	if len(body) > maxRenderedEmailDigestBytes || html.Len() > maxRenderedEmailDigestBytes {
		return RenderedMessage{}, fmt.Errorf("rendered event email exceeds %d bytes", maxRenderedEmailDigestBytes)
	}

	subject := singleEventEmailSubject(event, title)
	return RenderedMessage{
		Subject:  subject,
		Body:     body,
		HTMLBody: html.String(),
	}, nil
}

func singleEventEmailSubject(event Event, title string) string {
	app := cleanEntityLabel(event.Application)
	if app == "" {
		app = cleanEntityLabel(event.Project)
	}
	target := cleanEntityLabel(event.DeploymentTarget)
	parts := []string{"Luna DevOps", title}
	if app != "" && target != "" {
		parts = append(parts, fmt.Sprintf("%s (%s)", app, target))
	} else if app != "" {
		parts = append(parts, app)
	}
	return templateTruncate(strings.Join(parts, " · "), maxEmailDigestSubjectRunes)
}

func emailToneColors(event Event) (accent string, badgeBg string, badgeText string) {
	switch feishuHeaderTemplateColor(event) {
	case "green":
		return "#16a34a", "#dcfce7", "#15803d"
	case "orange":
		return "#d97706", "#fef3c7", "#92400e"
	case "red":
		return "#dc2626", "#fee2e2", "#b91c1c"
	default:
		return "#2563eb", "#dbeafe", "#1d4ed8"
	}
}

func singleEventDiagnosticSnippet(event Event) string {
	switch {
	case strings.HasPrefix(event.Type, "runtime.") && event.Type != "runtime.recovered":
		lines := make([]string, 0, 4)
		if reason := strings.TrimSpace(event.Runtime.Reason); reason != "" {
			lines = append(lines, "Reason: "+reason)
		}
		if event.Runtime.ExitCode != nil {
			lines = append(lines, fmt.Sprintf("ExitCode: %d", *event.Runtime.ExitCode))
		}
		if detail := strings.TrimSpace(event.Runtime.DiagnosticDetail); detail != "" {
			lines = append(lines, detail)
		}
		return templateTruncate(strings.Join(lines, "\n"), maxEmailDigestTextRunes)
	case strings.HasPrefix(event.Type, "build.") && event.Severity != SeverityInfo:
		if strings.TrimSpace(event.Build.Message) != "" && strings.TrimSpace(event.Build.Message) != strings.TrimSpace(event.Message) {
			return templateTruncate(strings.TrimSpace(event.Build.Message), maxEmailDigestTextRunes)
		}
	case strings.HasPrefix(event.Type, "release.") && event.Severity != SeverityInfo:
		if strings.TrimSpace(event.Release.Message) != "" && strings.TrimSpace(event.Release.Message) != strings.TrimSpace(event.Message) {
			return templateTruncate(strings.TrimSpace(event.Release.Message), maxEmailDigestTextRunes)
		}
	case strings.HasPrefix(event.Type, "certificate.") && event.Severity != SeverityInfo:
		if strings.TrimSpace(event.Certificate.Message) != "" && strings.TrimSpace(event.Certificate.Message) != strings.TrimSpace(event.Message) {
			return templateTruncate(strings.TrimSpace(event.Certificate.Message), maxEmailDigestTextRunes)
		}
	}
	return ""
}

func renderSingleEventEmailText(view singleEventEmailView) string {
	var out strings.Builder
	fmt.Fprintf(&out, "Luna DevOps — [%s] %s\n", view.SeverityLabel, view.Title)
	if view.ScopeSubtitle != "" {
		fmt.Fprintf(&out, "%s\n", view.ScopeSubtitle)
	}
	fmt.Fprintf(&out, "\n%s\n\n", view.Message)
	for _, field := range view.Fields {
		fmt.Fprintf(&out, "%s: %s\n", field.Label, field.Value)
	}
	for _, field := range view.Details {
		fmt.Fprintf(&out, "%s: %s\n", field.Label, field.Value)
	}
	if view.DiagSnippet != "" {
		fmt.Fprintf(&out, "\n%s:\n%s\n", view.DiagTitle, view.DiagSnippet)
	}
	if view.PrimaryURL != "" {
		fmt.Fprintf(&out, "\n%s: %s\n", view.URLLabel, view.PrimaryURL)
	}
	fmt.Fprintf(&out, "\n%s\n", view.Footer)
	return out.String()
}

func digestEventView(number int, event Event, labels emailDigestLabels, lang string) emailDigestEventView {
	return digestEventViewInZone(number, event, labels, lang, "UTC")
}

func digestEventViewInZone(number int, event Event, labels emailDigestLabels, lang string, timeZone string) emailDigestEventView {
	occurredAt := digestTimeInZone(event.OccurredAt, timeZone)
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
	view.Details = digestEventDetailsInZone(event, labels, lang, timeZone)
	return view
}

func digestEventDetails(event Event, labels emailDigestLabels) []emailDigestField {
	return digestEventDetailsInZone(event, labels, "en", "UTC")
}

func digestEventDetailsInZone(event Event, labels emailDigestLabels, lang string, timeZone string) []emailDigestField {
	isZH := lang == "zh-CN"
	details := make([]emailDigestField, 0, 8)
	switch {
	case strings.HasPrefix(event.Type, "runtime."):
		podContainer := joinNonEmpty(" / ", event.Runtime.PodName, event.Runtime.ContainerName)
		if isZH {
			details = appendDigestField(details, "异常实例 / 容器", podContainer)
		} else {
			details = appendDigestField(details, "Pod / Container", podContainer)
		}
		if event.Runtime.DesiredReplicas > 0 || event.Runtime.ReadyReplicas > 0 {
			replicas := fmt.Sprintf("%d / %d Ready", event.Runtime.ReadyReplicas, event.Runtime.DesiredReplicas)
			if isZH {
				details = appendDigestField(details, "副本就绪", replicas)
			} else {
				details = appendDigestField(details, "Replicas", replicas)
			}
		}
		if event.Runtime.RestartCount > 0 {
			if isZH {
				details = appendDigestField(details, "重启次数", strconv.Itoa(int(event.Runtime.RestartCount)))
			} else {
				details = appendDigestField(details, "Restarts", strconv.Itoa(int(event.Runtime.RestartCount)))
			}
		}
		if event.Runtime.Reason != "" {
			if isZH {
				details = appendDigestField(details, "退出/等待原因", event.Runtime.Reason)
			} else {
				details = appendDigestField(details, "Reason", event.Runtime.Reason)
			}
		}
		if event.Runtime.ExitCode != nil {
			if isZH {
				details = appendDigestField(details, "退出码", strconv.Itoa(int(*event.Runtime.ExitCode)))
			} else {
				details = appendDigestField(details, "Exit code", strconv.Itoa(int(*event.Runtime.ExitCode)))
			}
		}
		if event.Runtime.ReminderCount > 0 {
			if isZH {
				details = appendDigestField(details, "持续告警提醒", fmt.Sprintf("第 %d 次提醒", event.Runtime.ReminderCount))
			} else {
				details = appendDigestField(details, "Reminder", fmt.Sprintf("#%d", event.Runtime.ReminderCount))
			}
		}
		if event.Runtime.DurationSeconds > 0 && event.Type == "runtime.recovered" {
			if isZH {
				details = appendDigestField(details, "故障持续时长", fmt.Sprintf("%ds", event.Runtime.DurationSeconds))
			} else {
				details = appendDigestField(details, "Incident duration", fmt.Sprintf("%ds", event.Runtime.DurationSeconds))
			}
		}
	case strings.HasPrefix(event.Type, "build."):
		details = appendDigestField(details, labels.Details.ID, event.Build.ID)
		details = appendDigestField(details, labels.Details.Status, event.Build.Status)
		details = appendDigestField(details, labels.Details.Image, event.Build.Image)
		details = appendDigestField(details, labels.Details.GitRef, event.Build.GitRef)
		details = appendDigestField(details, labels.Details.GitSHA, event.Build.GitSHA)
		details = appendDistinctDetail(details, labels.Details.DetailMessage, event.Build.Message, event.Message)
	case strings.HasPrefix(event.Type, "release."):
		details = appendDigestField(details, labels.Details.ID, event.Release.ID)
		details = appendDigestField(details, labels.Details.Status, event.Release.Status)
		if event.Release.Revision > 0 {
			details = appendDigestField(details, labels.Details.Revision, strconv.Itoa(event.Release.Revision))
		}
		details = appendDigestField(details, labels.Details.Image, event.Release.ImageRef)
		details = appendDistinctDetail(details, labels.Details.DetailMessage, event.Release.Message, event.Message)
	case strings.HasPrefix(event.Type, "hook."):
		details = appendDigestField(details, labels.Details.ID, event.Hook.ID)
		details = appendDigestField(details, labels.Details.Name, event.Hook.Name)
		details = appendDigestField(details, labels.Details.Phase, event.Hook.Phase)
		details = appendDigestField(details, labels.Details.Status, event.Hook.Status)
		details = appendDistinctDetail(details, labels.Details.DetailMessage, event.Hook.Message, event.Message)
	case strings.HasPrefix(event.Type, "certificate."):
		details = appendDigestField(details, labels.Details.ID, event.Certificate.RouteID)
		details = appendDigestField(details, labels.Host, event.Certificate.Host)
		details = appendDigestField(details, labels.Details.Status, event.Certificate.Status)
		if event.Certificate.NotAfter != nil {
			details = appendDigestField(details, labels.ExpiresAt, digestTimeInZone(*event.Certificate.NotAfter, timeZone))
		}
		details = appendDigestField(details, labels.Issuer, joinNonEmpty(" / ", event.Certificate.IssuerKind, event.Certificate.IssuerName))
		details = appendDistinctDetail(details, labels.Details.DetailMessage, event.Certificate.Message, event.Message)
	case strings.HasPrefix(event.Type, "gateway."):
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
	return digestTimeRangeInZone(events, "UTC")
}

func digestTimeRangeInZone(events []Event, timeZone string) string {
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
		return digestTimeInZone(first, timeZone)
	}
	return digestTimeInZone(first, timeZone) + " — " + digestTimeInZone(last, timeZone)
}

func digestTime(value time.Time) string {
	return digestTimeInZone(value, "UTC")
}

func digestTimeInZone(value time.Time, timeZone string) string {
	if value.IsZero() {
		return ""
	}
	if strings.EqualFold(strings.TrimSpace(timeZone), "UTC") || strings.TrimSpace(timeZone) == "" {
		return value.UTC().Format("2006-01-02 15:04:05 UTC")
	}
	return FormatTimeInZoneLayout(value, timeZone, "2006-01-02 15:04:05 MST")
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
	"build.started":             "构建已开始",
	"build.succeeded":           "构建成功",
	"build.failed":              "构建失败",
	"build.webhook_rejected":    "Webhook 自动构建被拦截",
	"release.started":           "发布已开始",
	"release.succeeded":         "发布成功",
	"release.failed":            "发布失败",
	"runtime.pod_oom_killed":    "容器内存溢出 (OOMKilled)",
	"runtime.pod_crash_loop":    "容器崩溃循环重启 (CrashLoopBackOff)",
	"runtime.pod_start_failed":  "实例启动异常",
	"runtime.workload_degraded": "工作负载副本降级",
	"runtime.recovered":         "工作负载已恢复正常",
	"hook.started":              "Hook 已开始",
	"hook.succeeded":            "Hook 执行成功",
	"hook.failed":               "Hook 失败",
	"gateway.applied":           "访问入口已生效",
	"gateway.apply_failed":      "访问入口应用失败",
	"certificate.pending":       "证书签发处理中",
	"certificate.issued":        "证书已签发",
	"certificate.renewed":       "证书已续期",
	"certificate.failed":        "证书签发失败",
	"certificate.expired":       "证书已过期",
	"service_binding.created":   "服务绑定已创建",
	"service_binding.updated":   "服务绑定已更新",
	"service_binding.deleted":   "服务绑定已删除",
	"notification.test":         "通知通道连通性测试",
}

var eventTypeLabelsEN = map[string]string{
	"build.started":             "Build started",
	"build.succeeded":           "Build succeeded",
	"build.failed":              "Build failed",
	"build.webhook_rejected":    "Webhook auto-build rejected",
	"release.started":           "Release started",
	"release.succeeded":         "Release succeeded",
	"release.failed":            "Release failed",
	"runtime.pod_oom_killed":    "Container OOMKilled",
	"runtime.pod_crash_loop":    "Container CrashLoopBackOff",
	"runtime.pod_start_failed":  "Pod failed to start",
	"runtime.workload_degraded": "Workload replicas degraded",
	"runtime.recovered":         "Workload recovered",
	"hook.started":              "Hook started",
	"hook.succeeded":            "Hook succeeded",
	"hook.failed":               "Hook failed",
	"gateway.applied":           "Gateway route applied",
	"gateway.apply_failed":      "Gateway route failed",
	"certificate.pending":       "Certificate pending",
	"certificate.issued":        "Certificate issued",
	"certificate.renewed":       "Certificate renewed",
	"certificate.failed":        "Certificate failed",
	"certificate.expired":       "Certificate expired",
	"service_binding.created":   "Service binding created",
	"service_binding.updated":   "Service binding updated",
	"service_binding.deleted":   "Service binding deleted",
	"notification.test":         "Notification channel test",
}

var singleEventEmailTemplate = template.Must(template.New("single-event-email").Parse(`<!doctype html>
<html lang="{{.Lang}}">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Luna DevOps — {{.Title}}</title>
</head>
<body style="margin:0;padding:0;background:#f4f6fb;color:#172033;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Arial,sans-serif;">
  <table role="presentation" width="100%" cellspacing="0" cellpadding="0" border="0" style="width:100%;background:#f4f6fb;">
    <tr>
      <td align="center" style="padding:28px 12px;">
        <table role="presentation" width="640" cellspacing="0" cellpadding="0" border="0" style="width:100%;max-width:640px;background:#ffffff;border-radius:12px;overflow:hidden;border:1px solid #e5e7eb;">
          <tr>
            <td style="height:6px;background:{{.AccentColor}};font-size:0;line-height:0;">&nbsp;</td>
          </tr>
          <tr>
            <td style="padding:26px 32px 20px 32px;border-bottom:1px solid #eef0f4;">
              <table role="presentation" width="100%" cellspacing="0" cellpadding="0" border="0">
                <tr>
                  <td style="vertical-align:top;padding-right:12px;">
                    <p style="margin:0 0 8px 0;color:{{.AccentColor}};font-size:13px;font-weight:700;letter-spacing:.03em;">LUNA DEVOPS</p>
                    <h1 style="margin:0;color:#111827;font-size:22px;line-height:1.35;font-weight:700;">{{.Title}}</h1>
                    {{if .ScopeSubtitle}}<p style="margin:6px 0 0 0;color:#6b7280;font-size:14px;line-height:1.5;">{{.ScopeSubtitle}}</p>{{end}}
                  </td>
                  <td align="right" style="vertical-align:top;white-space:nowrap;">
                    <span style="display:inline-block;padding:5px 11px;border-radius:999px;background:{{.BadgeBg}};color:{{.BadgeText}};font-size:12px;font-weight:700;">{{.SeverityLabel}}</span>
                  </td>
                </tr>
              </table>
            </td>
          </tr>
          <tr>
            <td style="padding:24px 32px;">
              <p style="margin:0 0 18px 0;color:#1f2937;font-size:15px;line-height:1.65;word-break:break-word;">{{.Message}}</p>
              <table role="presentation" width="100%" cellspacing="0" cellpadding="0" border="0" style="font-size:13px;line-height:1.6;background:#f9fafb;border-radius:8px;padding:12px 16px;">
                {{range .Fields}}<tr><td style="width:136px;padding:4px 12px 4px 0;color:#6b7280;vertical-align:top;font-weight:500;">{{.Label}}</td><td style="padding:4px 0;color:#111827;word-break:break-word;">{{.Value}}</td></tr>{{end}}
                {{range .Details}}<tr><td style="width:136px;padding:4px 12px 4px 0;color:#6b7280;vertical-align:top;font-weight:500;">{{.Label}}</td><td style="padding:4px 0;color:#111827;word-break:break-word;">{{.Value}}</td></tr>{{end}}
              </table>
              {{if .DiagSnippet}}
              <div style="margin-top:18px;">
                <p style="margin:0 0 6px 0;color:#374151;font-size:12px;font-weight:700;">{{.DiagTitle}}</p>
                <pre style="margin:0;padding:12px 14px;border-radius:8px;background:#111827;color:#f3f4f6;font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;font-size:12px;line-height:1.55;white-space:pre-wrap;word-break:break-word;">{{.DiagSnippet}}</pre>
              </div>
              {{end}}
              {{if .PrimaryURL}}
              <table role="presentation" cellspacing="0" cellpadding="0" border="0" style="margin-top:22px;"><tr><td style="border-radius:8px;background:{{.AccentColor}};"><a href="{{.PrimaryURL}}" style="display:inline-block;padding:10px 18px;color:#ffffff;text-decoration:none;font-size:14px;font-weight:700;">{{.CTA}}</a></td></tr></table>
              <p style="margin:12px 0 0 0;color:#6b7280;font-size:12px;line-height:1.5;word-break:break-all;">{{.URLLabel}}：<a href="{{.PrimaryURL}}" style="color:#2563eb;text-decoration:underline;">{{.PrimaryURL}}</a></p>
              {{end}}
            </td>
          </tr>
          <tr>
            <td style="padding:16px 32px;background:#f9fafb;border-top:1px solid #eef0f4;color:#6b7280;font-size:12px;line-height:1.5;">
              {{.Footer}}{{if .EventType}} · <span style="font-family:ui-monospace,SFMono-Regular,Menlo,monospace;">{{.EventType}}</span>{{end}}
            </td>
          </tr>
        </table>
      </td>
    </tr>
  </table>
</body>
</html>`))

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
