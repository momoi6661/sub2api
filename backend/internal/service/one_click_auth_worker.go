package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/util/logredact"
)

// OneClickAuthWorker performs one attempt per invalid credential state. The
// marker is persisted before execution, so restarts cannot create retry loops.
type OneClickAuthWorker struct {
	admin  AdminService
	locks  LeaderLockCache
	run    func(context.Context, int64) error
	cancel context.CancelFunc
	done   chan struct{}
	once   sync.Once
}

func NewOneClickAuthWorker(admin AdminService, locks LeaderLockCache, run func(context.Context, int64) error) *OneClickAuthWorker {
	return &OneClickAuthWorker{admin: admin, locks: locks, run: run, done: make(chan struct{})}
}

func (w *OneClickAuthWorker) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	w.cancel = cancel
	go func() {
		defer close(w.done)
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				w.scan(ctx)
			}
		}
	}()
}

func (w *OneClickAuthWorker) Stop() {
	w.once.Do(func() {
		if w.cancel != nil {
			w.cancel()
			<-w.done
		}
	})
}

func IsOneClickAuthFailure(a *Account) bool {
	if a == nil || a.Platform != PlatformOpenAI || !a.IsOAuth() || a.IsShadow() || a.Status != StatusError {
		return false
	}
	enabled, _ := a.Extra["one_click_auth_enabled"].(bool)
	if !enabled {
		return false
	}
	message := strings.ToLower(a.ErrorMessage)
	for _, marker := range []string{"invalid_grant", "token_expired", "token_revoked", "token_invalidated", "invalid_token", "invalid_refresh_token", "refresh_token_reused", "authentication_error", "unauthorized", "401", "token refresh failed (non-retryable)"} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}

func OneClickAuthFailureFingerprint(a *Account) string {
	data, _ := json.Marshal([]any{a.Credentials, a.Extra["one_click_auth_profile_id"], a.Extra["one_click_auth_login_email"]})
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

func (w *OneClickAuthWorker) scan(ctx context.Context) {
	if w.admin == nil || w.run == nil {
		return
	}
	// Collect candidates before execution: successful authorization removes rows
	// from the error filter and would otherwise shift later pagination offsets.
	var candidates []Account
	for page := 1; ctx.Err() == nil; page++ {
		accounts, total, err := w.admin.ListAccounts(ctx, page, 100, PlatformOpenAI, "oauth", StatusError, "", 0, "", "id", "asc")
		if err != nil {
			slog.Warn("one_click_auth.scan_failed")
			return
		}
		for _, account := range accounts {
			if IsOneClickAuthFailure(&account) {
				candidates = append(candidates, account)
			}
		}
		if len(accounts) == 0 || int64(page*100) >= total {
			break
		}
	}
	for i := range candidates {
		if ctx.Err() != nil {
			return
		}
		w.attempt(ctx, &candidates[i])
	}
}

func (w *OneClickAuthWorker) attempt(ctx context.Context, account *Account) {
	if !IsOneClickAuthFailure(account) {
		return
	}
	// Automatic execution fails closed if distributed locking is unavailable.
	if w.locks == nil {
		return
	}
	key := fmt.Sprintf("one_click_auth_auto:%d", account.ID)
	owner := fmt.Sprintf("%d", time.Now().UnixNano())
	ok, err := w.locks.TryAcquireLeaderLock(ctx, key, owner, 150*time.Second)
	if err != nil || !ok {
		return
	}
	defer func() {
		releaseCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = w.locks.ReleaseLeaderLock(releaseCtx, key, owner)
	}()
	current, err := w.admin.GetAccount(ctx, account.ID)
	if err != nil || !IsOneClickAuthFailure(current) {
		return
	}
	fingerprint := OneClickAuthFailureFingerprint(current)
	if current.Extra["one_click_auth_auto_attempt"] == fingerprint {
		return
	}
	if err = w.admin.UpdateAccountExtra(ctx, current.ID, map[string]any{"one_click_auth_auto_attempt": fingerprint}); err != nil {
		return
	}
	err = w.run(ctx, current.ID)
	status, code, message := "succeeded", "", ""
	if err != nil {
		status, code, message = "failed", "AUTHORIZATION_FAILED", logredact.RedactText(err.Error())
		var execution *ReauthExecutionError
		if errors.As(err, &execution) {
			code = execution.Code
		}
		if len(message) > 1000 {
			message = message[:1000]
		}
	}
	resultCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = w.admin.UpdateAccountExtra(resultCtx, current.ID, map[string]any{"one_click_auth_last_status": status, "one_click_auth_last_code": code, "one_click_auth_last_error": message, "one_click_auth_last_at": time.Now().UTC().Format(time.RFC3339)})
}
