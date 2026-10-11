package service

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"golang.org/x/sync/singleflight"
)

// OpenAIAccountSubscription contains only queried metadata, never OAuth credentials.
type OpenAIAccountSubscription struct {
	SubscriptionExpiresAt string `json:"subscription_expires_at"`
	CheckedAt             string `json:"checked_at"`
}

type openAISubscriptionCacheEntry struct {
	result OpenAIAccountSubscription
	until  time.Time
}

type openAISubscriptionQueries struct {
	mu     sync.Mutex
	cache  map[string]openAISubscriptionCacheEntry
	flight singleflight.Group
	slots  chan struct{}
}

func (q *openAISubscriptionQueries) cached(key string) (OpenAIAccountSubscription, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	entry, ok := q.cache[key]
	return entry.result, ok && time.Now().Before(entry.until)
}

// QueryAccountSubscription queries an already authorized account without token
// refresh, reauthorization, file parsing or changes to the account's privacy settings.
func (s *OpenAIOAuthService) QueryAccountSubscription(ctx context.Context, account *Account, force bool) (*OpenAIAccountSubscription, error) {
	if account == nil || !account.IsOpenAIOAuth() || account.IsOpenAIAgentIdentity() {
		return nil, infraerrors.New(http.StatusBadRequest, "OPENAI_SUBSCRIPTION_INVALID_ACCOUNT", "Subscription queries require an OpenAI OAuth account")
	}
	if s.privacyClientFactory == nil {
		return nil, infraerrors.New(http.StatusServiceUnavailable, "OPENAI_SUBSCRIPTION_UNAVAILABLE", "Subscription query client is unavailable")
	}
	token := strings.TrimSpace(account.GetCredential("access_token"))
	if token == "" {
		return nil, infraerrors.New(http.StatusBadRequest, "OPENAI_SUBSCRIPTION_MISSING_TOKEN", "Account has no access token")
	}
	accountID := strings.TrimSpace(account.GetCredential("chatgpt_account_id"))
	if accountID == "" {
		if claims, err := openai.DecodeIDToken(token); err == nil && claims.OpenAIAuth != nil {
			accountID = claims.OpenAIAuth.ChatGPTAccountID
		}
	}
	if accountID == "" {
		return nil, infraerrors.New(http.StatusBadRequest, "OPENAI_SUBSCRIPTION_MISSING_IDENTITY", "Account has no ChatGPT account identity")
	}
	var proxyURL string
	if account.ProxyID != nil {
		if s.proxyRepo == nil {
			return nil, infraerrors.New(http.StatusServiceUnavailable, "OPENAI_SUBSCRIPTION_PROXY_UNAVAILABLE", "Account proxy is unavailable")
		}
		proxy, err := s.proxyRepo.GetByID(ctx, *account.ProxyID)
		if err != nil {
			return nil, err
		}
		if proxy == nil || !proxy.IsActive() || proxy.IsExpired(time.Now()) {
			return nil, infraerrors.New(http.StatusBadRequest, "OPENAI_SUBSCRIPTION_PROXY_UNAVAILABLE", "Account proxy is inactive or expired")
		}
		proxyURL = proxy.URL()
	}
	s.subscriptionOnce.Do(func() {
		s.subscriptionQueries = &openAISubscriptionQueries{cache: make(map[string]openAISubscriptionCacheEntry), slots: make(chan struct{}, 4)}
	})
	q := s.subscriptionQueries
	// A token, workspace or proxy change must not reuse another identity's result.
	key := fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%d\x00%s\x00%s\x00%s", account.ID, token, accountID, proxyURL))))
	if result, ok := q.cached(key); ok && !force {
		return &result, nil
	}
	resultCh := q.flight.DoChan(key, func() (any, error) {
		if result, ok := q.cached(key); ok && !force {
			return result, nil
		}
		// One caller leaving the page must not cancel a shared query for another.
		queryCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 45*time.Second)
		defer cancel()
		select {
		case q.slots <- struct{}{}:
			defer func() { <-q.slots }()
		case <-queryCtx.Done():
			return nil, queryCtx.Err()
		}
		expiresAt := fetchChatGPTSubscriptionExpiresAt(queryCtx, s.privacyClientFactory, token, proxyURL, accountID)
		if expiresAt == "" {
			if info := fetchChatGPTAccountInfo(queryCtx, s.privacyClientFactory, token, proxyURL, accountID); info != nil && strings.EqualFold(info.AccountID, accountID) {
				expiresAt = info.SubscriptionExpiresAt
			}
		}
		if expiresAt == "" {
			expiresAt = queryCodexSubscriptionExpiry(queryCtx, s.privacyClientFactory, token, proxyURL, accountID)
		}
		if err := queryCtx.Err(); err != nil {
			return nil, err
		}
		result := OpenAIAccountSubscription{SubscriptionExpiresAt: expiresAt, CheckedAt: time.Now().UTC().Format(time.RFC3339)}
		ttl := 5 * time.Minute
		if expiresAt == "" {
			ttl = time.Minute
		}
		q.mu.Lock()
		for cachedKey, entry := range q.cache {
			if time.Now().After(entry.until) || len(q.cache) >= 512 {
				delete(q.cache, cachedKey)
			}
		}
		q.cache[key] = openAISubscriptionCacheEntry{result: result, until: time.Now().Add(ttl)}
		q.mu.Unlock()
		return result, nil
	})
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case result := <-resultCh:
		if result.Err != nil {
			return nil, result.Err
		}
		value := result.Val.(OpenAIAccountSubscription)
		return &value, nil
	}
}

// Codex OAuth credentials can query subscriptions directly with the Codex
// identity and HTTP/1.1. Clone the pooled client so this request cannot change
// the headers or HTTP version used by concurrent privacy/usage requests.
func queryCodexSubscriptionExpiry(ctx context.Context, factory PrivacyClientFactory, token, proxyURL, accountID string) string {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	client, err := factory(proxyURL)
	if err != nil {
		return ""
	}
	client = client.Clone().EnableForceHTTP1()
	client.Headers = make(http.Header)
	defer client.Transport.CloseIdleConnections()
	var result struct {
		ActiveUntil string `json:"active_until"`
		ExpiresAt   string `json:"expires_at"`
	}
	identity := resolveCodexOutboundIdentity("")
	resp, err := client.R().SetContext(ctx).
		SetHeader("Authorization", "Bearer "+token).
		SetHeader("Accept", "application/json").
		SetHeader("ChatGPT-Account-ID", accountID).
		SetHeader("OpenAI-Beta", "codex-1").
		SetHeader("Originator", identity.originator).
		SetHeader("User-Agent", identity.userAgent).
		SetSuccessResult(&result).
		SetQueryParam("account_id", accountID).
		Get(chatGPTSubscriptionsURL)
	if err != nil || resp == nil || resp.Response == nil || !resp.IsSuccessState() {
		status := 0
		if resp != nil && resp.Response != nil {
			status = resp.StatusCode
		}
		slog.Debug("codex_subscription_query_failed", "status", status)
		return ""
	}
	expiresAt := strings.TrimSpace(result.ActiveUntil)
	if expiresAt == "" {
		expiresAt = strings.TrimSpace(result.ExpiresAt)
	}
	if _, err := time.Parse(time.RFC3339, expiresAt); err != nil {
		return ""
	}
	return expiresAt
}
