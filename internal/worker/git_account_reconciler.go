package worker

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	gitprovider "github.com/LiteyukiStudio/devops/internal/provider/git"
	"github.com/LiteyukiStudio/devops/internal/tasks"
	"github.com/LiteyukiStudio/devops/internal/telemetry"
	"github.com/hibiken/asynq"
	"go.opentelemetry.io/otel/attribute"
	"golang.org/x/oauth2"
)

func (r *Runner) handleGitAccountRefresh(ctx context.Context, task *asynq.Task) error {
	var payload tasks.GitAccountRefreshPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return err
	}
	accounts, err := r.gitAccountsDueForRefresh(time.Now())
	if err != nil {
		return err
	}
	for _, account := range accounts {
		err := workerStage(ctx, "git_account.refresh", func(stageCtx context.Context) error {
			return r.refreshGitAccount(stageCtx, account)
		}, attribute.String("git.account.id", account.ID))
		if err == nil {
			telemetry.Logger().InfoContext(ctx, "git account refreshed",
				slog.String("event.name", "git_account.refreshed"),
				slog.String("git.account.id", account.ID),
			)
		}
	}
	return nil
}

func (r *Runner) gitAccountsDueForRefresh(now time.Time) ([]model.GitAccount, error) {
	var accounts []model.GitAccount
	err := r.db.Where("refresh_token_ref <> '' and expires_at is not null and expires_at <= ?", now.Add(5*time.Minute)).
		Find(&accounts).Error
	return accounts, err
}

func gitAccountDueForWorkerRefresh(account model.GitAccount, now time.Time) bool {
	return strings.TrimSpace(account.RefreshTokenRef) != "" &&
		account.ExpiresAt != nil &&
		!account.ExpiresAt.After(now.Add(5*time.Minute))
}

func (r *Runner) refreshGitAccount(ctx context.Context, account model.GitAccount) error {
	var provider model.GitProvider
	if err := r.db.First(&provider, "id = ? and enabled = ?", account.ProviderID, true).Error; err != nil {
		return err
	}
	refreshToken := r.secrets.Resolve(account.RefreshTokenRef)
	if strings.TrimSpace(refreshToken) == "" {
		return r.auditGitAccountRefresh(account, false, "git account has no refresh token")
	}
	oauthConfig, err := gitprovider.OAuthConfig(provider, "", r.secrets.Resolve(provider.ClientSecretRef))
	if err != nil {
		return r.auditGitAccountRefresh(account, false, "git OAuth provider configuration is invalid")
	}
	ctx = context.WithValue(ctx, oauth2.HTTPClient, telemetry.InstrumentHTTPClient(nil))
	tokenSource := oauthConfig.TokenSource(ctx, &oauth2.Token{
		RefreshToken: refreshToken,
		Expiry:       time.Now().Add(-time.Minute),
	})
	token, err := tokenSource.Token()
	if err != nil {
		return r.auditGitAccountRefresh(account, false, "git token refresh failed")
	}
	account.AccessTokenRef = r.secrets.Store(token.AccessToken, account.UserID, "git_account:"+account.ID+":access")
	if token.RefreshToken != "" {
		account.RefreshTokenRef = r.secrets.Store(token.RefreshToken, account.UserID, "git_account:"+account.ID+":refresh")
	}
	if !token.Expiry.IsZero() {
		account.ExpiresAt = &token.Expiry
	}
	if err := r.db.Save(&account).Error; err != nil {
		return err
	}
	return r.auditGitAccountRefresh(account, true, account.Username)
}

func (r *Runner) auditGitAccountRefresh(account model.GitAccount, success bool, message string) error {
	entry := model.AuditLog{
		ID:       id.New("aud"),
		UserID:   account.UserID,
		Action:   "git_account.refresh",
		Resource: account.ID,
		Success:  success,
		Message:  message,
	}
	return r.db.Create(&entry).Error
}
