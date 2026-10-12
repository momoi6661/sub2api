package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/imroc/req/v3"
	"github.com/stretchr/testify/require"
)

type subscriptionQueryProxyRepo struct {
	ProxyRepository
	proxy *Proxy
}

type subscriptionSnapshotRepo struct {
	AccountRepository
	updates []map[string]any
}

func (r *subscriptionSnapshotRepo) UpdateExtra(_ context.Context, _ int64, updates map[string]any) error {
	r.updates = append(r.updates, updates)
	return nil
}

func (r subscriptionQueryProxyRepo) GetByID(context.Context, int64) (*Proxy, error) {
	return r.proxy, nil
}

func TestQueryAccountSubscriptionFetchesSavedIdentityAndCaches(t *testing.T) {
	var calls atomic.Int32
	server := newChatGPTBackendTestServer(t, chatGPTBackendTestServerConfig{
		onSubscription: func(accountID string) map[string]any {
			calls.Add(1)
			require.Equal(t, "saved-workspace", accountID)
			return map[string]any{"active_until": "2027-01-01T00:00:00Z"}
		},
	})
	defer server.Close()
	proxyID := int64(3)
	snapshotRepo := &subscriptionSnapshotRepo{}
	svc := &OpenAIOAuthService{
		accountRepo: snapshotRepo,
		proxyRepo:   subscriptionQueryProxyRepo{proxy: &Proxy{ID: 3, Status: StatusActive, Protocol: "http", Host: "proxy.test", Port: 7890}},
		privacyClientFactory: func(proxyURL string) (*req.Client, error) {
			require.Equal(t, "http://proxy.test:7890", proxyURL)
			return req.C().SetTimeout(time.Second), nil
		},
	}
	account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, ProxyID: &proxyID, Credentials: map[string]any{
		"access_token": "saved-token", "chatgpt_account_id": "saved-workspace", "subscription_expires_at": "2000-01-01T00:00:00Z",
	}}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := svc.QueryAccountSubscription(context.Background(), account, false)
			require.NoError(t, err)
			require.Equal(t, "2027-01-01T00:00:00Z", result.SubscriptionExpiresAt)
			require.NotEmpty(t, result.CheckedAt)
		}()
	}
	wg.Wait()
	require.EqualValues(t, 1, calls.Load())
	// A newly imported/rotated token must perform a fresh upstream query.
	account.Credentials["access_token"] = "new-token"
	_, err := svc.QueryAccountSubscription(context.Background(), account, false)
	require.NoError(t, err)
	require.EqualValues(t, 2, calls.Load())
	// Expiring the server cache refreshes the date without refreshing the token.
	svc.subscriptionQueries.mu.Lock()
	for key, entry := range svc.subscriptionQueries.cache {
		entry.until = time.Now().Add(-time.Minute)
		svc.subscriptionQueries.cache[key] = entry
	}
	svc.subscriptionQueries.mu.Unlock()
	_, err = svc.QueryAccountSubscription(context.Background(), account, false)
	require.NoError(t, err)
	require.EqualValues(t, 3, calls.Load())
	_, err = svc.QueryAccountSubscription(context.Background(), account, true)
	require.NoError(t, err)
	require.EqualValues(t, 4, calls.Load(), "explicit refresh must bypass a still valid cache")
	require.Equal(t, "2000-01-01T00:00:00Z", account.GetCredential("subscription_expires_at"), "query must not rewrite saved credentials")
	require.Len(t, snapshotRepo.updates, 4)
	require.Equal(t, "2027-01-01T00:00:00Z", snapshotRepo.updates[len(snapshotRepo.updates)-1][OpenAISubscriptionExpiryExtraKey])
	require.NotEmpty(t, snapshotRepo.updates[len(snapshotRepo.updates)-1][OpenAISubscriptionCheckedAtExtraKey])
	require.Equal(t, "2027-01-01T00:00:00Z", account.GetExtraString(OpenAISubscriptionExpiryExtraKey))
}

func TestQueryAccountSubscriptionOnlyUsesMatchingEntitlement(t *testing.T) {
	for _, sourceID := range []string{"saved-workspace", "other-workspace"} {
		t.Run(sourceID, func(t *testing.T) {
			const expiry = "2027-01-01T00:00:00Z"
			server := newChatGPTBackendTestServer(t, chatGPTBackendTestServerConfig{accountsCheck: map[string]any{
				"accounts": map[string]any{sourceID: map[string]any{
					"account":     map[string]any{"account_id": sourceID, "plan_type": "team", "is_default": true},
					"entitlement": map[string]any{"expires_at": expiry},
				}},
			}})
			defer server.Close()
			svc := &OpenAIOAuthService{privacyClientFactory: newTestPrivacyClientFactory()}
			account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{
				"access_token": "saved-token", "chatgpt_account_id": "saved-workspace", "subscription_expires_at": "2000-01-01T00:00:00Z",
			}}
			result, err := svc.QueryAccountSubscription(context.Background(), account, false)
			require.NoError(t, err)
			if sourceID == "saved-workspace" {
				require.Equal(t, expiry, result.SubscriptionExpiresAt)
			} else {
				require.Empty(t, result.SubscriptionExpiresAt, "do not display another workspace's expiry or the file's date")
			}
		})
	}
}

func TestQueryAccountSubscriptionRejectsUnavailableSelectedProxy(t *testing.T) {
	proxyID := int64(3)
	svc := &OpenAIOAuthService{proxyRepo: subscriptionQueryProxyRepo{}, privacyClientFactory: func(string) (*req.Client, error) {
		t.Fatal("must not silently query directly when a selected proxy is unavailable")
		return nil, nil
	}}
	_, err := svc.QueryAccountSubscription(context.Background(), &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, ProxyID: &proxyID, Credentials: map[string]any{
		"access_token": "saved-token", "chatgpt_account_id": "saved-workspace",
	}}, false)
	require.Error(t, err)
}

func TestQueryCodexSubscriptionExpiryUsesIdentityWithoutMutatingSharedClient(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "HTTP/1.1", r.Proto)
		require.Equal(t, "Bearer saved-token", r.Header.Get("Authorization"))
		require.Equal(t, "application/json", r.Header.Get("Accept"))
		require.Equal(t, "codex-1", r.Header.Get("OpenAI-Beta"))
		require.Equal(t, "saved-workspace", r.Header.Get("ChatGPT-Account-ID"))
		require.Equal(t, "saved-workspace", r.URL.Query().Get("account_id"))
		require.NotEmpty(t, r.Header.Get("Originator"))
		require.Equal(t, CodexCanonicalUserAgent(), r.Header.Get("User-Agent"))
		require.Empty(t, r.Header.Get("Referer"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"active_until":"2027-01-01T00:00:00Z"}`))
	}))
	defer server.Close()
	oldURL := chatGPTSubscriptionsURL
	chatGPTSubscriptionsURL = server.URL
	t.Cleanup(func() { chatGPTSubscriptionsURL = oldURL })
	shared := req.C().SetCommonHeader("Referer", "https://chatgpt.com/")
	expiresAt := queryCodexSubscriptionExpiry(context.Background(), func(string) (*req.Client, error) {
		return shared, nil
	}, "saved-token", "", "saved-workspace")
	require.Equal(t, "2027-01-01T00:00:00Z", expiresAt)
	require.Equal(t, "https://chatgpt.com/", shared.Headers.Get("Referer"))
	require.Empty(t, shared.Headers.Get("Authorization"))
}

func TestQueryCodexSubscriptionExpiryHandlesClosedConnection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, _, err := w.(http.Hijacker).Hijack()
		require.NoError(t, err)
		_ = conn.Close()
	}))
	defer server.Close()
	oldURL := chatGPTSubscriptionsURL
	chatGPTSubscriptionsURL = server.URL
	t.Cleanup(func() { chatGPTSubscriptionsURL = oldURL })
	require.Empty(t, queryCodexSubscriptionExpiry(context.Background(), newTestPrivacyClientFactory(), "saved-token", "", "saved-workspace"))
}

func TestQueryAccountSubscriptionDoesNotCacheUnauthorizedAsEmptyExpiry(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()
	oldSubscriptionURL, oldCheckURL := chatGPTSubscriptionsURL, chatGPTAccountsCheckURL
	chatGPTSubscriptionsURL, chatGPTAccountsCheckURL = server.URL, server.URL
	t.Cleanup(func() { chatGPTSubscriptionsURL, chatGPTAccountsCheckURL = oldSubscriptionURL, oldCheckURL })
	svc := &OpenAIOAuthService{privacyClientFactory: newTestPrivacyClientFactory()}
	account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{
		"access_token": "rejected-token", "chatgpt_account_id": "saved-workspace",
	}}
	result, err := svc.QueryAccountSubscription(context.Background(), account, true)
	require.Nil(t, result)
	require.ErrorContains(t, err, "401")
	require.Empty(t, svc.subscriptionQueries.cache)
}
