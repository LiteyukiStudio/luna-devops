package api

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/observation"
	gitprovider "github.com/LiteyukiStudio/devops/internal/provider/git"
)

const gitObservationConcurrency = 4

func (h *Handlers) observeGitAccounts(ctx context.Context, user model.User, accounts []model.GitAccount) {
	sem := make(chan struct{}, gitObservationConcurrency)
	var wg sync.WaitGroup
	for index := range accounts {
		wg.Add(1)
		go func(account *model.GitAccount) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			h.observeGitAccount(ctx, user, account)
		}(&accounts[index])
	}
	wg.Wait()
}

func (h *Handlers) observeGitAccount(ctx context.Context, user model.User, account *model.GitAccount) {
	now := time.Now().UTC()
	account.ObservedAt = &now
	account.Status = observation.StatusUnavailable
	account.ObservationCode = "git_account_upstream_unavailable"

	client, configured := h.gitClientForObservation(user, account.ProviderID, account.ID, ctx)
	if !configured {
		account.Status = observation.StatusNotConfigured
		account.ObservationCode = "git_account_not_configured"
		return
	}
	if _, err := client.CurrentUser(ctx); err != nil {
		account.Status, account.ObservationCode = gitObservationFromError(err, "git_account")
		return
	}
	account.Status = observation.StatusReady
	account.ObservationCode = "git_account_ready"
}

func (h *Handlers) observeRepositoryBindings(ctx context.Context, user model.User, bindings []repositoryBindingResponse) {
	sem := make(chan struct{}, gitObservationConcurrency)
	var wg sync.WaitGroup
	for index := range bindings {
		wg.Add(1)
		go func(binding *repositoryBindingResponse) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			h.observeRepositoryBinding(ctx, user, binding)
		}(&bindings[index])
	}
	wg.Wait()
}

func (h *Handlers) observeRepositoryBinding(ctx context.Context, user model.User, binding *repositoryBindingResponse) {
	now := time.Now().UTC()
	binding.WebhookObservedAt = &now
	if !binding.WebhookEnabled {
		binding.WebhookStatus = observation.StatusNotConfigured
		binding.WebhookObservationCode = "git_webhook_disabled"
		return
	}
	if strings.TrimSpace(binding.WebhookID) == "" {
		binding.WebhookStatus = observation.StatusNotFound
		binding.WebhookObservationCode = "git_webhook_not_found"
		return
	}

	client, configured := h.gitClientForObservation(user, binding.GitProviderID, binding.GitAccountID, ctx)
	if !configured {
		binding.WebhookStatus = observation.StatusNotConfigured
		binding.WebhookObservationCode = "git_webhook_not_configured"
		return
	}
	snapshot, err := client.GetWebhook(ctx, binding.Owner, binding.Repo, binding.WebhookID)
	if err != nil {
		binding.WebhookStatus, binding.WebhookObservationCode = gitObservationFromError(err, "git_webhook")
		return
	}
	if !snapshot.Active {
		binding.WebhookStatus = observation.StatusDegraded
		binding.WebhookObservationCode = "git_webhook_inactive"
		return
	}
	if expected := strings.TrimSpace(binding.WebhookCallbackURL); expected != "" &&
		!strings.EqualFold(strings.TrimRight(snapshot.URL, "/"), strings.TrimRight(expected, "/")) {
		binding.WebhookStatus = observation.StatusDegraded
		binding.WebhookObservationCode = "git_webhook_callback_mismatch"
		return
	}
	binding.WebhookStatus = observation.StatusReady
	binding.WebhookObservationCode = "git_webhook_ready"
}

func (h *Handlers) gitClientForObservation(user model.User, providerID, accountID string, ctx context.Context) (gitprovider.Client, bool) {
	var account model.GitAccount
	if err := h.dbWithContext(ctx).First(&account, "id = ?", strings.TrimSpace(accountID)).Error; err != nil {
		return gitprovider.Client{}, false
	}
	if !h.canUseScopedResourceByID(user, account.Scope, account.OwnerRef, scopedResourceGitAccount, account.ID, ctx) {
		return gitprovider.Client{}, false
	}
	var provider model.GitProvider
	if err := h.dbWithContext(ctx).First(&provider, "id = ? and enabled = ?", strings.TrimSpace(providerID), true).Error; err != nil {
		return gitprovider.Client{}, false
	}
	if account.ProviderID != provider.ID ||
		!h.canUseScopedResourceByID(user, provider.Scope, provider.OwnerRef, scopedResourceGitProvider, provider.ID, ctx) {
		return gitprovider.Client{}, false
	}
	token := strings.TrimSpace(h.secrets.ResolveContext(ctx, account.AccessTokenRef))
	if token == "" {
		return gitprovider.Client{}, false
	}
	return gitprovider.NewClientWithPolicy(provider, token, h.egressPolicyForUser(user, ctx)), true
}

func gitObservationFromError(err error, prefix string) (string, string) {
	if upstreamErr, ok := gitprovider.AsUpstreamError(err); ok {
		switch upstreamErr.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			return observation.StatusDegraded, prefix + "_credential_rejected"
		case http.StatusNotFound:
			return observation.StatusNotFound, prefix + "_not_found"
		}
	}
	return observation.StatusUnavailable, prefix + "_upstream_unavailable"
}
