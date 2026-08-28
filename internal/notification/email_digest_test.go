package notification

import (
	"strings"
	"testing"
	"time"
)

func TestRenderPersonalEmailDigestIncludesMultipleLocalizedEvents(t *testing.T) {
	first := Event{
		ID:               "evt_build",
		Type:             "build.failed",
		Severity:         SeverityError,
		Project:          EntityRef{ID: "prj_1", Name: "轻雪项目空间", Identifier: "liteyuki"},
		Application:      EntityRef{ID: "app_1", Name: "OpenList", Identifier: "openlist"},
		DeploymentTarget: EntityRef{ID: "dplt_1", Name: "生产", Identifier: "prod"},
		Build: BuildContext{
			ID: "build_1", Status: "failed", Message: "依赖安装失败",
			Image: "registry.example.com/openlist:main", GitRef: "refs/heads/main", GitSHA: "0123456789abcdef",
		},
		Actor:         ActorContext{Name: "远野千束", Email: "operator@example.com"},
		Links:         map[string]string{"primary": "https://devops.example.com/projects/prj_1/apps/app_1?tab=builds#buildRunId=build_1"},
		CorrelationID: "build_1",
		OccurredAt:    time.Date(2026, 8, 28, 7, 10, 0, 0, time.UTC),
		Message:       `构建失败：<script>alert("x")</script>`,
	}
	second := Event{
		ID:               "evt_certificate",
		Type:             "certificate.expired",
		Severity:         SeverityError,
		Project:          first.Project,
		Application:      first.Application,
		DeploymentTarget: first.DeploymentTarget,
		Certificate: CertificateContext{
			RouteID: "gwr_1", Host: "openlist.example.com", Status: "expired",
			IssuerKind: "ClusterIssuer", IssuerName: "letsencrypt",
		},
		Links:      map[string]string{"primary": "https://devops.example.com/projects/prj_1/apps/app_1?tab=gateway"},
		OccurredAt: time.Date(2026, 8, 28, 7, 14, 34, 0, time.UTC),
		Message:    "证书已过期",
	}

	message, err := RenderPersonalEmailDigest([]Event{second, first}, "zh-CN")
	if err != nil {
		t.Fatalf("RenderPersonalEmailDigest() error = %v", err)
	}
	if message.Subject != "Luna DevOps · 2 条通知 · OpenList" {
		t.Fatalf("Subject = %q", message.Subject)
	}
	for _, want := range []string{
		"1. [错误] 构建失败",
		"2. [错误] 证书已过期",
		"项目空间: 轻雪项目空间 (liteyuki)",
		"Git SHA: 0123456789abcdef",
		"主机: openlist.example.com",
		"签发者: ClusterIssuer / letsencrypt",
		"可访问地址: https://devops.example.com/projects/prj_1/apps/app_1?tab=builds#buildRunId=build_1",
	} {
		if !strings.Contains(message.Body, want) {
			t.Fatalf("Body does not contain %q:\n%s", want, message.Body)
		}
	}
	if strings.Index(message.Body, "构建失败") > strings.Index(message.Body, "证书已过期") {
		t.Fatalf("events are not ordered by occurredAt:\n%s", message.Body)
	}
	for _, want := range []string{
		"<html lang=\"zh-CN\">",
		"&lt;script&gt;alert(&#34;x&#34;)&lt;/script&gt;",
		"https://devops.example.com/projects/prj_1/apps/app_1?tab=builds#buildRunId=build_1",
		">查看详情</a>",
	} {
		if !strings.Contains(message.HTMLBody, want) {
			t.Fatalf("HTMLBody does not contain %q:\n%s", want, message.HTMLBody)
		}
	}
	if strings.Contains(message.HTMLBody, "<script>") {
		t.Fatal("HTMLBody contains unescaped event markup")
	}
}

func TestRenderPersonalEmailDigestOmitsUnsafeOrRelativePrimaryURL(t *testing.T) {
	for _, primaryURL := range []string{"/events/evt_1", "javascript:alert(1)", "https://user:pass@devops.example.com/events/evt_1"} {
		t.Run(primaryURL, func(t *testing.T) {
			message, err := RenderPersonalEmailDigest([]Event{{
				ID: "evt_1", Type: "build.failed", Severity: SeverityError,
				Links: map[string]string{"primary": primaryURL}, Message: "failed",
			}}, "en-US")
			if err != nil {
				t.Fatalf("RenderPersonalEmailDigest() error = %v", err)
			}
			if strings.Contains(message.HTMLBody, "View details") || strings.Contains(message.Body, primaryURL) {
				t.Fatalf("unsafe or relative URL rendered: %q", primaryURL)
			}
		})
	}
}

func TestRenderPersonalEmailDigestRejectsEmptyDigest(t *testing.T) {
	if _, err := RenderPersonalEmailDigest(nil, "zh-CN"); err == nil {
		t.Fatal("expected empty digest to fail")
	}
}

func TestRenderPersonalEmailDigestFallsBackToUnknownEventType(t *testing.T) {
	unknownType := "custom.failed" + strings.Repeat("x", maxEmailDigestTitleRunes)
	message, err := RenderPersonalEmailDigest([]Event{{Type: unknownType, Message: "failed"}}, "zh-CN")
	if err != nil {
		t.Fatalf("RenderPersonalEmailDigest() error = %v", err)
	}
	if !strings.Contains(message.Subject, "custom.failed") || !strings.Contains(message.Body, "custom.failed") {
		t.Fatalf("unknown event type was not preserved: %#v", message)
	}
	if len([]rune(message.Subject)) > maxEmailDigestSubjectRunes || strings.Contains(message.Body, unknownType) {
		t.Fatalf("unknown event type was not truncated: %#v", message)
	}
}

func TestRenderPersonalEmailDigestSupportsBoundedTwentyEventBatch(t *testing.T) {
	events := make([]Event, 20)
	for index := range events {
		events[index] = Event{
			ID: "evt_batch", Type: "build.failed", Severity: SeverityError,
			Build:   BuildContext{ID: "build_batch", Message: strings.Repeat("<failure>", 400)},
			Message: strings.Repeat("<message>", 400),
		}
	}
	message, err := RenderPersonalEmailDigest(events, "en-US")
	if err != nil {
		t.Fatalf("RenderPersonalEmailDigest() error = %v", err)
	}
	if len(message.Body) > maxRenderedEmailDigestBytes || len(message.HTMLBody) > maxRenderedEmailDigestBytes {
		t.Fatalf("digest size = text %d, html %d", len(message.Body), len(message.HTMLBody))
	}
	if strings.Contains(message.HTMLBody, "<failure>") || strings.Contains(message.HTMLBody, "<message>") {
		t.Fatal("batched event markup was not escaped")
	}
}
