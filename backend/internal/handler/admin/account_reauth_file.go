package admin

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// normalizeReauthAuthFile accepts one identity matching the requested account
// or draft login email. It never creates accounts or chooses the target from JSON.
func normalizeReauthAuthFile(content string, existing *service.Account) (*ApplyOAuthCredentialsRequest, error) {
	if existing == nil || existing.Platform != service.PlatformOpenAI || !existing.IsOAuth() || existing.ParentAccountID != nil {
		return nil, errors.New("authorization files require a non-shadow OpenAI OAuth account")
	}
	var object map[string]any
	if err := json.Unmarshal([]byte(content), &object); err != nil || object == nil {
		return nil, errors.New("return one OAuth authorization JSON object")
	}
	item, err := normalizeCodexImportEntry(codexImportEntry{Index: 1, Value: object})
	if err != nil {
		return nil, errors.New("invalid or expired OAuth authorization file")
	}
	if item.TokenExpiresAt == nil || !item.TokenExpiresAt.After(time.Now()) {
		return nil, errors.New("authorization file requires a future access token expiry")
	}
	if item.IsAgentIdentity || item.AccessToken == "" {
		return nil, errors.New("file must contain OAuth access tokens")
	}
	expectedEmail := codexCredentialString(existing.Extra, "one_click_auth_login_email")
	if expectedEmail == "" {
		expectedEmail = codexCredentialString(existing.Credentials, "email")
	}
	expectedID := codexCredentialString(existing.Credentials, "chatgpt_account_id")
	if existing.ID != 0 && expectedEmail == "" && expectedID == "" {
		return nil, errors.New("configure account login email before using an authorization file")
	}
	if expectedEmail != "" && !strings.EqualFold(expectedEmail, item.Email) {
		return nil, errors.New("authorization file email does not match target account")
	}
	if expectedID != "" && expectedID != item.AccountID {
		return nil, errors.New("authorization file identity does not match target account")
	}
	return &ApplyOAuthCredentialsRequest{Type: "oauth", Credentials: item.Credentials, Extra: item.Extra}, nil
}

// Protocol authorization already verifies the OAuth response against the chosen
// workspace in the runner. A workspace switch may change the ChatGPT account ID,
// but must never change the account owner. Manual/custom files keep strict ID matching.
func normalizeProtocolReauthAuthFile(content string, existing *service.Account) (*ApplyOAuthCredentialsRequest, error) {
	if existing == nil {
		return normalizeReauthAuthFile(content, existing)
	}
	if _, _, err := reauthWorkspaceSelection(existing); err != nil {
		return nil, err
	}
	email := codexCredentialString(existing.Extra, "one_click_auth_login_email")
	if email == "" {
		email = codexCredentialString(existing.Credentials, "email")
	}
	if email == "" {
		return nil, errors.New("configure account login email before switching workspace")
	}
	selection := *existing
	selection.Credentials = make(map[string]any, len(existing.Credentials))
	for key, value := range existing.Credentials {
		if key != "chatgpt_account_id" {
			selection.Credentials[key] = value
		}
	}
	return normalizeReauthAuthFile(content, &selection)
}

func (h *AccountHandler) ApplyReauthFile(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id < 1 {
		response.BadRequest(c, "Invalid account ID")
		return
	}
	var input struct {
		Content string `json:"content"`
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 128*1024)
	if err := c.ShouldBindJSON(&input); err != nil {
		response.BadRequest(c, "Invalid authorization file request")
		return
	}
	existing, err := h.adminService.GetAccount(c.Request.Context(), id)
	if response.ErrorFrom(c, err) {
		return
	}
	payload, err := normalizeReauthAuthFile(input.Content, existing)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	encoded, err := json.Marshal(payload)
	if response.ErrorFrom(c, err) {
		return
	}
	// Reuse the established credential merge, cache invalidation and recovery.
	c.Request.Body = io.NopCloser(strings.NewReader(string(encoded)))
	c.Request.ContentLength = int64(len(encoded))
	h.ApplyOAuthCredentials(c)
}
