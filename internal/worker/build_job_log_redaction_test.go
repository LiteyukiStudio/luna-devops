package worker

import "testing"

func TestRedactSensitiveLogContentMasksLikeGitHubActions(t *testing.T) {
	privateKey := "-----BEGIN KEY-----\nline-one-secret\nline-two-secret\n-----END KEY-----"
	for _, test := range []struct {
		name    string
		content string
		values  []string
		want    string
	}{
		{name: "plain value", content: "token is s3cr3t-value", values: []string{"s3cr3t-value"}, want: "token is ***"},
		{name: "multiline secret printed per line", content: "line-two-secret", values: []string{privateKey}, want: "***"},
		{name: "longer secret wins over its prefix", content: "abcd-extended", values: []string{"abcd", "abcd-extended"}, want: "***"},
		{name: "url escaped value", content: "https://host/?p=a%2Fb%2Bc", values: []string{"a/b+c"}, want: "https://host/?p=***"},
		{name: "short values are ignored", content: "abc", values: []string{"abc"}, want: "abc"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := redactSensitiveLogContent(test.content, test.values); got != test.want {
				t.Fatalf("redacted = %q, want %q", got, test.want)
			}
		})
	}
}
