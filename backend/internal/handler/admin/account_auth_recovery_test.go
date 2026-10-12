package admin

import (
	"context"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

type authRecoveryAdmin struct {
	*stubAdminService
	extra        map[string]any
	extraUpdates int
}

func (a *authRecoveryAdmin) UpdateAccountExtra(_ context.Context, _ int64, updates map[string]any) error {
	a.extraUpdates++
	for k, v := range updates {
		a.extra[k] = v
	}
	return nil
}

func TestApplyOAuthCredentialsWithoutAutomaticAttemptLeavesExtraAlone(t *testing.T) {
	base := newStubAdminService()
	base.getAccountResult = &service.Account{ID: 7, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth, Status: service.StatusError, Credentials: map[string]any{"access_token": "old"}}
	admin := &authRecoveryAdmin{stubAdminService: base, extra: map[string]any{}}
	base.getAccountResult.Extra = admin.extra
	handler := NewAccountHandler(admin, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	_, err := handler.applyOAuthCredentials(context.Background(), 7, &ApplyOAuthCredentialsRequest{Type: "oauth", Credentials: map[string]any{"access_token": "new"}})
	require.NoError(t, err)
	require.Zero(t, admin.extraUpdates)
	require.NotContains(t, admin.extra, "one_click_auth_auto_attempt")
}
func TestApplyOAuthCredentialsClearsAutomaticAttemptOnlyOnSuccess(t *testing.T) {
	for _, fails := range []bool{false, true} {
		t.Run(map[bool]string{false: "successful_manual_auth", true: "failed_manual_auth"}[fails], func(t *testing.T) {
			base := newStubAdminService()
			base.getAccountResult = &service.Account{ID: 7, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth, Status: service.StatusError, Credentials: map[string]any{"access_token": "old"}}
			if fails {
				base.updateAccountErr = errors.New("save failed")
			}
			admin := &authRecoveryAdmin{stubAdminService: base, extra: map[string]any{"one_click_auth_auto_attempt": "previous-attempt"}}
			base.getAccountResult.Extra = admin.extra
			handler := NewAccountHandler(admin, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
			account, err := handler.applyOAuthCredentials(context.Background(), 7, &ApplyOAuthCredentialsRequest{Type: "oauth", Credentials: map[string]any{"access_token": "new"}})
			if fails {
				require.Error(t, err)
				require.Equal(t, "previous-attempt", admin.extra["one_click_auth_auto_attempt"])
			} else {
				require.NoError(t, err)
				require.Equal(t, "", admin.extra["one_click_auth_auto_attempt"])
				require.Equal(t, "", account.Extra["one_click_auth_auto_attempt"])
				require.True(t, account.Schedulable)
				require.Equal(t, service.StatusActive, account.Status)
			}
		})
	}
}
