package admin

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestReauthProtocolAuthFileIdentityAndExpiry(t *testing.T) {
	for _, tc := range []struct {
		name, email, accountID    string
		expired, draft, wantError bool
	}{
		{"existing account", "owner@example.com", "account-1", false, false, false},
		{"new panel account with existing provider identity", "owner@example.com", "account-1", false, true, false},
		{"wrong mailbox", "other@example.com", "account-1", false, false, true},
		{"wrong workspace", "owner@example.com", "account-2", false, false, true},
		{"expired file", "owner@example.com", "account-1", true, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			expires := time.Now().Add(time.Hour)
			if tc.expired {
				expires = time.Now().Add(-time.Hour)
			}
			identity := map[string]any{"email": tc.email, "https://api.openai.com/auth": map[string]any{"chatgpt_account_id": tc.accountID}}
			token := buildCodexImportTestJWT(t, expires, identity)
			content, err := json.Marshal(map[string]any{
				"type": "codex", "access_token": token, "id_token": token, "refresh_token": "test-refresh",
				"email": tc.email, "account_id": tc.accountID, "expires_at": expires.UTC().Format(time.RFC3339), "expired": expires.UTC().Format(time.RFC3339),
			})
			if err != nil {
				t.Fatal(err)
			}
			account := &service.Account{ID: 10, Platform: service.PlatformOpenAI, Type: "oauth", Credentials: map[string]any{"chatgpt_account_id": "account-1"}, Extra: map[string]any{"one_click_auth_login_email": "owner@example.com"}}
			if tc.draft {
				account.ID = 0
				account.Credentials = nil
			}
			payload, err := normalizeReauthAuthFile(string(content), account)
			if tc.wantError {
				if err == nil {
					t.Fatal("invalid file accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if payload.Credentials["chatgpt_account_id"] != "account-1" || payload.Credentials["refresh_token"] != "test-refresh" {
				t.Fatalf("credentials were not normalized correctly: %v", payload.Credentials)
			}
		})
	}
}
