package service

import (
	"context"
	"errors"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

type autoAuthEventAdmin struct {
	AdminService
	account *Account
	saved   chan struct{}
}

func (a *autoAuthEventAdmin) GetAccount(context.Context, int64) (*Account, error) {
	return a.account, nil
}
func (a *autoAuthEventAdmin) UpdateAccountExtra(_ context.Context, _ int64, updates map[string]any) error {
	for k, v := range updates {
		a.account.Extra[k] = v
	}
	if _, ok := updates["one_click_auth_last_status"]; ok && a.saved != nil {
		a.saved <- struct{}{}
	}
	return nil
}

type autoAuthEventLocks struct{}

func (autoAuthEventLocks) TryAcquireLeaderLock(context.Context, string, string, time.Duration) (bool, error) {
	return true, nil
}
func (autoAuthEventLocks) ReleaseLeaderLock(context.Context, string, string) error { return nil }
func autoAuthEventAccount() *Account {
	return &Account{ID: 7, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusError, ErrorMessage: "401 unauthorized", Credentials: map[string]any{"access_token": "old"}, Extra: map[string]any{"one_click_auth_enabled": true, "one_click_auth_profile_id": "profile"}}
}
func TestOneClickAuthWorkerRunsFromErrorEvent(t *testing.T) {
	a := &autoAuthEventAdmin{account: autoAuthEventAccount(), saved: make(chan struct{}, 1)}
	w := NewOneClickAuthWorker(a, autoAuthEventLocks{}, func(ctx context.Context, id int64) error {
		require.Equal(t, int64(7), id)
		require.Equal(t, OneClickAuthFailureFingerprint(a.account), a.account.Extra["one_click_auth_auto_attempt"])
		return errors.New("authorization failed")
	})
	events := NewAccountErrorEvents()
	events.SetHandler(w.Notify)
	events.Notify(7, "401 unauthorized")
	w.Start()
	select {
	case <-a.saved:
	case <-time.After(2 * time.Second):
		t.Fatal("error event did not execute authorization")
	}
	w.Stop()
	require.Equal(t, "failed", a.account.Extra["one_click_auth_last_status"])
}
func TestOneClickAuthWorkerFailureIsNotRetriedAfterRestart(t *testing.T) {
	a := &autoAuthEventAdmin{account: autoAuthEventAccount()}
	runs := 0
	run := func(context.Context, int64) error { runs++; return errors.New("failed") }
	w := NewOneClickAuthWorker(a, autoAuthEventLocks{}, run)
	w.attempt(context.Background(), a.account)
	w.attempt(context.Background(), a.account)
	restarted := NewOneClickAuthWorker(a, autoAuthEventLocks{}, run)
	restarted.attempt(context.Background(), a.account)
	require.Equal(t, 1, runs)
	require.Equal(t, "failed", a.account.Extra["one_click_auth_last_status"])
	a.account.Credentials["access_token"] = "new"
	restarted.attempt(context.Background(), a.account)
	require.Equal(t, 2, runs, "a different credential state may have one attempt")
}
func TestAccountErrorEventsIgnoreOtherFailures(t *testing.T) {
	events := NewAccountErrorEvents()
	calls := 0
	events.SetHandler(func(int64) { calls++ })
	events.Notify(7, "proxy connection timeout")
	events.Notify(7, "429 rate limited")
	require.Zero(t, calls)
	events.Notify(7, "invalid_grant")
	require.Equal(t, 1, calls)
	a := autoAuthEventAccount()
	a.Extra["one_click_auth_enabled"] = false
	w := NewOneClickAuthWorker(&autoAuthEventAdmin{account: a}, autoAuthEventLocks{}, func(context.Context, int64) error { t.Fatal("disabled automatic auth must not run"); return nil })
	w.attempt(context.Background(), a)
	require.NotContains(t, a.Extra, "one_click_auth_auto_attempt")
}
