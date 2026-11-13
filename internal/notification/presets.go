package notification

import "encoding/json"

type WebhookPreset struct {
	ID                   string   `json:"id"`
	Name                 string   `json:"name"`
	Description          string   `json:"description"`
	AdapterKind          string   `json:"adapterKind"`
	ConfigTemplate       string   `json:"configTemplate"`
	JSONBodyTemplate     string   `json:"jsonBodyTemplate"`
	SecretFields         []string `json:"secretFields"`
	OptionalSecretFields []string `json:"optionalSecretFields,omitempty"`
}

func WebhookPresets() []WebhookPreset {
	return []WebhookPreset{
		{
			ID:          "feishu-bot",
			Name:        "Feishu Bot",
			Description: "Feishu custom bot webhook with interactive card content and optional signature verification.",
			AdapterKind: AdapterKindWebhook,
			ConfigTemplate: webhookPresetConfigWithSigning("POST", "https://open.feishu.cn/open-apis/bot/v2/hook/{{.Secrets.WebhookToken}}", map[string]string{
				"Content-Type": "application/json",
			}, "feishu", feishuInteractiveTemplate("zh")),
			JSONBodyTemplate:     feishuInteractiveTemplate("zh"),
			SecretFields:         []string{"WebhookToken"},
			OptionalSecretFields: []string{"SignSecret"},
		},
		{
			ID:          "lark-bot",
			Name:        "Lark Bot",
			Description: "Lark custom bot webhook with interactive card content and optional signature verification.",
			AdapterKind: AdapterKindWebhook,
			ConfigTemplate: webhookPresetConfigWithSigning("POST", "https://open.larksuite.com/open-apis/bot/v2/hook/{{.Secrets.WebhookToken}}", map[string]string{
				"Content-Type": "application/json",
			}, "feishu", feishuInteractiveTemplate("en")),
			JSONBodyTemplate:     feishuInteractiveTemplate("en"),
			SecretFields:         []string{"WebhookToken"},
			OptionalSecretFields: []string{"SignSecret"},
		},
		{
			ID:          "wecom-bot",
			Name:        "WeCom Bot",
			Description: "WeCom group robot webhook with markdown content.",
			AdapterKind: AdapterKindWebhook,
			ConfigTemplate: webhookPresetConfig("POST", "https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key={{.Secrets.WebhookKey}}", map[string]string{
				"Content-Type": "application/json",
			}, wecomMarkdownTemplate),
			JSONBodyTemplate: wecomMarkdownTemplate,
			SecretFields:     []string{"WebhookKey"},
		},
		{
			ID:          "gotify",
			Name:        "Gotify",
			Description: "Gotify application message API with markdown display extras.",
			AdapterKind: AdapterKindWebhook,
			ConfigTemplate: webhookPresetConfig("POST", "https://{{.Secrets.GotifyHost}}/message", map[string]string{
				"Content-Type": "application/json",
				"X-Gotify-Key": "{{.Secrets.AppToken}}",
			}, gotifyMarkdownTemplate),
			JSONBodyTemplate: gotifyMarkdownTemplate,
			SecretFields:     []string{"GotifyHost", "AppToken"},
		},
		{
			ID:          "dingtalk-bot",
			Name:        "DingTalk Bot",
			Description: "DingTalk custom robot webhook with markdown content and optional signature verification.",
			AdapterKind: AdapterKindWebhook,
			ConfigTemplate: webhookPresetConfigWithSigning("POST", "https://oapi.dingtalk.com/robot/send?access_token={{.Secrets.AccessToken}}", map[string]string{
				"Content-Type": "application/json",
			}, "dingtalk", dingtalkMarkdownTemplate),
			JSONBodyTemplate:     dingtalkMarkdownTemplate,
			SecretFields:         []string{"AccessToken"},
			OptionalSecretFields: []string{"SignSecret"},
		},
		{
			ID:          "slack-webhook",
			Name:        "Slack Incoming Webhook",
			Description: "Slack incoming webhook with mrkdwn blocks.",
			AdapterKind: AdapterKindWebhook,
			ConfigTemplate: webhookPresetConfig("POST", "https://hooks.slack.com/services/{{.Secrets.WebhookPath}}", map[string]string{
				"Content-Type": "application/json",
			}, slackBlocksTemplate),
			JSONBodyTemplate: slackBlocksTemplate,
			SecretFields:     []string{"WebhookPath"},
		},
		{
			ID:          "discord-webhook",
			Name:        "Discord Webhook",
			Description: "Discord webhook with embeds.",
			AdapterKind: AdapterKindWebhook,
			ConfigTemplate: webhookPresetConfig("POST", "https://discord.com/api/webhooks/{{.Secrets.WebhookID}}/{{.Secrets.WebhookToken}}", map[string]string{
				"Content-Type": "application/json",
			}, discordEmbedTemplate),
			JSONBodyTemplate: discordEmbedTemplate,
			SecretFields:     []string{"WebhookID", "WebhookToken"},
		},
	}
}

// PersonalWebhookPresets excludes integrations whose destination host is
// user-controlled. Administrators can still use the complete preset catalog.
func PersonalWebhookPresets() []WebhookPreset {
	all := WebhookPresets()
	personal := make([]WebhookPreset, 0, len(all))
	for _, preset := range all {
		if preset.ID != "gotify" {
			personal = append(personal, preset)
		}
	}
	return personal
}

func webhookPresetConfig(method string, url string, headers map[string]string, testJSONBodyTemplate string) string {
	return webhookPresetConfigWithSigning(method, url, headers, "", testJSONBodyTemplate)
}

func webhookPresetConfigWithSigning(method string, url string, headers map[string]string, signingMode string, testJSONBodyTemplate string) string {
	data, _ := json.MarshalIndent(WebhookConfig{
		Method:               method,
		URL:                  url,
		Headers:              headers,
		Timeout:              15,
		SigningMode:          signingMode,
		TestJSONBodyTemplate: testJSONBodyTemplate,
	}, "", "  ")
	return string(data)
}

func feishuInteractiveTemplate(locale string) string {
	return `{{feishuCard .Event "` + locale + `"}}`
}

const wecomMarkdownTemplate = `{
  "msgtype": "markdown",
  "markdown": {
    "content": {{json (printf "**%s**\n%s" (detailsTitle .Event) (details .Event "zh"))}}
  }
}`

const gotifyMarkdownTemplate = `{
  "title": {{json (detailsTitle .Event)}},
  "message": {{json (details .Event "en")}},
  "priority": 5,
  "extras": {
    "client::display": {
      "contentType": "text/markdown"
    }
  }
}`

const dingtalkMarkdownTemplate = `{
  "msgtype": "markdown",
  "markdown": {
    "title": {{json (detailsTitle .Event)}},
    "text": {{json (printf "### %s\n\n%s" (detailsTitle .Event) (details .Event "zh"))}}
  }
}`

const slackBlocksTemplate = `{
  "text": {{json (printf "%s: %s" (detailsTitle .Event) .Event.Message)}},
  "blocks": [
    {
      "type": "header",
      "text": {
        "type": "plain_text",
        "text": {{json (detailsTitle .Event)}}
      }
    },
    {
      "type": "section",
      "text": {
        "type": "mrkdwn",
        "text": {{json (details .Event "en")}}
      }
    }
  ]
}`

const discordEmbedTemplate = `{
  "embeds": [
    {
      "title": {{json (printf "[%s] %s" .Event.Severity .Event.Type)}},
      "description": {{json (details .Event "en")}},
      "color": 15158332,
      "fields": [
        {"name": "Project", "value": {{json (default "-" .Event.Project.Name)}}, "inline": true},
        {"name": "Application", "value": {{json (default "-" .Event.Application.Name)}}, "inline": true},
        {"name": "Deployment", "value": {{json (default "-" .Event.DeploymentTarget.Name)}}, "inline": true},
        {"name": "Status", "value": {{json (default "-" (default .Event.Build.Status .Event.Release.Status))}}, "inline": true}
      ]
    }
  ]
}`
