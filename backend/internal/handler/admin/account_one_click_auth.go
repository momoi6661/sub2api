package admin

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/mail"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

var oneClickAccountLocks sync.Map

func (h *AccountHandler) SetReauthSettings(settings *SettingHandler, locks service.LeaderLockCache) {
	if settings != nil {
		h.reauthSettings = settings.settingService
		h.reauthLocks = locks
		h.reauthWorker = service.NewOneClickAuthWorker(h.adminService, locks, func(ctx context.Context, id int64) error { _, err := h.authorizeOneClick(ctx, id); return err })
		h.reauthWorker.Start()
	}
}

func writeReauthError(c *gin.Context, err error) {
	var execution *service.ReauthExecutionError
	if errors.As(err, &execution) {
		status := http.StatusUnprocessableEntity
		if execution.Code == "RUNNER_NOT_CONFIGURED" || execution.Code == "RUNNER_UNAVAILABLE" {
			status = http.StatusServiceUnavailable
		}
		if execution.Code == "TASK_ALREADY_RUNNING" || execution.Code == "RUNNER_BUSY" {
			status = http.StatusConflict
		}
		response.ErrorWithDetails(c, status, execution.Message, execution.Code, map[string]string{"retryable": strconv.FormatBool(execution.Retryable)})
		return
	}
	response.ErrorFrom(c, err)
}

func (h *AccountHandler) RunOneClickAuth(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id < 1 {
		response.BadRequest(c, "Invalid account ID")
		return
	}
	account, err := h.authorizeOneClick(c.Request.Context(), id)
	if err != nil {
		writeReauthError(c, err)
		return
	}
	response.Success(c, h.buildAccountResponseWithRuntime(c.Request.Context(), account))
}

func (h *AccountHandler) authorizeOneClick(parent context.Context, id int64) (*service.Account, error) {
	if _, loaded := oneClickAccountLocks.LoadOrStore(id, true); loaded {
		return nil, &service.ReauthExecutionError{Code: "TASK_ALREADY_RUNNING", Message: "This account already has an authorization task"}
	}
	defer oneClickAccountLocks.Delete(id)
	if h.reauthSettings == nil {
		return nil, &service.ReauthExecutionError{Code: "RUNNER_NOT_CONFIGURED", Message: "Authorization settings unavailable"}
	}
	ctx, cancel := context.WithTimeout(parent, 135*time.Second)
	defer cancel()
	if h.reauthLocks != nil {
		var nonce [16]byte
		if _, err := rand.Read(nonce[:]); err != nil {
			return nil, err
		}
		owner, key := hex.EncodeToString(nonce[:]), fmt.Sprintf("one_click_auth_task:%d", id)
		acquired, err := h.reauthLocks.TryAcquireLeaderLock(ctx, key, owner, 150*time.Second)
		if err != nil {
			return nil, &service.ReauthExecutionError{Code: "TASK_LOCK_UNAVAILABLE", Message: "Authorization task lock unavailable", Retryable: true}
		}
		if !acquired {
			return nil, &service.ReauthExecutionError{Code: "TASK_ALREADY_RUNNING", Message: "This account already has an authorization task"}
		}
		defer func() {
			releaseCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_ = h.reauthLocks.ReleaseLeaderLock(releaseCtx, key, owner)
		}()
	}

	account, err := h.adminService.GetAccount(ctx, id)
	if err != nil {
		return nil, err
	}
	if account.Platform != service.PlatformOpenAI || !account.IsOAuth() || account.IsShadow() {
		return nil, &service.ReauthExecutionError{Code: "ACCOUNT_NOT_SUPPORTED", Message: "Only non-shadow OpenAI OAuth accounts are supported"}
	}
	payload, err := h.authorizeReauthProfile(ctx, account)
	if err != nil {
		return nil, err
	}
	profileID := codexCredentialString(account.Extra, "one_click_auth_profile_id")
	current, err := h.adminService.GetAccount(ctx, id)
	if err != nil {
		return nil, err
	}
	if !reflect.DeepEqual(current.Credentials, account.Credentials) || current.Extra["one_click_auth_profile_id"] != profileID || current.Extra["one_click_auth_login_email"] != account.Extra["one_click_auth_login_email"] {
		return nil, &service.ReauthExecutionError{Code: "ACCOUNT_CHANGED", Message: "Account credentials or authorization binding changed during this task"}
	}
	return h.applyOAuthCredentials(ctx, id, payload)
}

// AuthorizeNewAccount returns verified credentials without creating a placeholder account.
func (h *AccountHandler) AuthorizeNewAccount(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	var input struct {
		ProfileID  string `json:"profile_id" binding:"required,max=128"`
		LoginEmail string `json:"login_email" binding:"required,max=254"`
		ProxyID    *int64 `json:"proxy_id"`
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	if err := c.ShouldBindJSON(&input); err != nil {
		response.BadRequest(c, "Select an authorization profile and enter a login email")
		return
	}
	input.LoginEmail = strings.TrimSpace(input.LoginEmail)
	address, err := mail.ParseAddress(input.LoginEmail)
	if err != nil || address.Address != input.LoginEmail || (input.ProxyID != nil && *input.ProxyID < 1) {
		response.BadRequest(c, "Invalid login email or proxy ID")
		return
	}
	if h.reauthSettings == nil {
		writeReauthError(c, &service.ReauthExecutionError{Code: "RUNNER_NOT_CONFIGURED", Message: "Authorization settings unavailable"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 135*time.Second)
	defer cancel()
	// Keep email out of lock keys, and prevent repeat requests for the same draft.
	digest := sha256.Sum256([]byte(input.ProfileID + "\x00" + strings.ToLower(input.LoginEmail)))
	key := fmt.Sprintf("one_click_auth_draft:%x", digest)
	if _, loaded := oneClickAccountLocks.LoadOrStore(key, true); loaded {
		writeReauthError(c, &service.ReauthExecutionError{Code: "TASK_ALREADY_RUNNING", Message: "This email already has an authorization task"})
		return
	}
	defer oneClickAccountLocks.Delete(key)
	if h.reauthLocks != nil {
		var nonce [16]byte
		if _, err := rand.Read(nonce[:]); err != nil {
			writeReauthError(c, err)
			return
		}
		owner := hex.EncodeToString(nonce[:])
		acquired, err := h.reauthLocks.TryAcquireLeaderLock(ctx, key, owner, 150*time.Second)
		if err != nil || !acquired {
			execution := &service.ReauthExecutionError{Code: "TASK_ALREADY_RUNNING", Message: "This email already has an authorization task"}
			if err != nil {
				execution = &service.ReauthExecutionError{Code: "TASK_LOCK_UNAVAILABLE", Message: "Authorization task lock unavailable", Retryable: true}
			}
			writeReauthError(c, execution)
			return
		}
		defer func() {
			releaseCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_ = h.reauthLocks.ReleaseLeaderLock(releaseCtx, key, owner)
		}()
	}
	draft := &service.Account{Platform: service.PlatformOpenAI, Type: "oauth", ProxyID: input.ProxyID, Extra: map[string]any{
		"one_click_auth_profile_id": input.ProfileID, "one_click_auth_login_email": input.LoginEmail,
	}}
	payload, err := h.authorizeReauthProfile(ctx, draft)
	if err != nil {
		writeReauthError(c, err)
		return
	}
	response.Success(c, payload)
}

// Shared execution keeps draft and existing-account authorization rules identical.
func (h *AccountHandler) authorizeReauthProfile(ctx context.Context, account *service.Account) (*ApplyOAuthCredentialsRequest, error) {
	profileID := codexCredentialString(account.Extra, "one_click_auth_profile_id")
	profiles, err := h.reauthSettings.ListReauthProfiles(ctx)
	if err != nil {
		return nil, err
	}
	var profile *service.ReauthProfile
	for i := range profiles {
		if profiles[i].ID == profileID && profiles[i].Enabled {
			profile = &profiles[i]
			break
		}
	}
	if profile == nil {
		return nil, &service.ReauthExecutionError{Code: "PROFILE_REQUIRED", Message: "Select an enabled authorization profile in account settings"}
	}

	material, err := h.reauthSettings.ReadReauthMaterial(ctx, profileID)
	if err != nil {
		return nil, err
	}
	email := codexCredentialString(account.Extra, "one_click_auth_login_email")
	if email == "" {
		email = codexCredentialString(account.Credentials, "email")
	}
	if email == "" {
		return nil, &service.ReauthExecutionError{Code: "LOGIN_EMAIL_REQUIRED", Message: "Configure this account's login email"}
	}
	resultType := profile.APIResultType
	if profile.MailboxMode == "imap" || resultType == "" {
		resultType = "code"
	}
	task := service.ReauthRunnerTask{Mode: "protocol", Script: material["code_script"], ResultType: resultType, Context: map[string]any{
		"accountId": account.ID, "loginEmail": email, "since": time.Now().UTC().Format(time.RFC3339), "apiURL": material["api_url"], "mailboxAddress": material["mailbox_address"], "secret": material["secret"],
	}}
	if profile.MailboxMode == "imap" {
		task.Mailbox = &service.ReauthRunnerMailbox{Host: profile.IMAPHost, Port: profile.IMAPPort, Username: profile.IMAPUsername, Folder: profile.Folder, SenderFilter: profile.SenderFilter, CodePattern: profile.CodePattern}
	}

	var sessionID string
	proxyID, err := chooseReauthProxy(profile, account.ProxyID)
	if err != nil {
		return nil, err
	}
	if resultType == "code" {
		if h.openaiOAuthService == nil {
			return nil, &service.ReauthExecutionError{Code: "OAUTH_UNAVAILABLE", Message: "OAuth service unavailable"}
		}
		task.Mode = "browser"
		task.RedirectURI = "http://localhost:1455/auth/callback"
		auth, err := h.openaiOAuthService.GenerateAuthURL(ctx, proxyID, task.RedirectURI, service.PlatformOpenAI)
		if err != nil {
			return nil, err
		}
		task.AuthURL, sessionID = auth.AuthURL, auth.SessionID
		u, err := url.Parse(auth.AuthURL)
		if err != nil {
			return nil, err
		}
		task.State = u.Query().Get("state")
	}
	if proxyID != nil {
		proxy, err := h.adminService.GetProxy(ctx, *proxyID)
		if err != nil {
			return nil, err
		}
		if !proxy.IsActive() || proxy.IsExpired(time.Now()) {
			return nil, &service.ReauthExecutionError{Code: "PROXY_UNAVAILABLE", Message: "Selected authorization proxy is inactive or expired"}
		}
		task.ProxyURL = proxy.URL()
		task.Context["proxyURL"] = task.ProxyURL
	}

	result, err := service.ExecuteReauthRunner(ctx, task)
	if err != nil {
		return nil, err
	}
	var payload *ApplyOAuthCredentialsRequest
	if resultType == "auth_file" {
		content := string(result.Auth)
		var text string
		if json.Unmarshal(result.Auth, &text) == nil {
			content = text
		}
		payload, err = normalizeReauthAuthFile(content, account)
	} else {
		if result.State != task.State || result.Code == "" {
			return nil, &service.ReauthExecutionError{Code: "OAUTH_CALLBACK_INVALID", Message: "OAuth callback state or code invalid"}
		}
		tokenInfo, exchangeErr := h.openaiOAuthService.ExchangeCode(ctx, &service.OpenAIExchangeCodeInput{SessionID: sessionID, Code: result.Code, State: result.State, RedirectURI: task.RedirectURI, ProxyID: proxyID})
		if exchangeErr != nil {
			return nil, exchangeErr
		}
		if !strings.EqualFold(email, tokenInfo.Email) {
			return nil, &service.ReauthExecutionError{Code: "IDENTITY_MISMATCH", Message: "Authorized email does not match this account"}
		}
		expectedID := codexCredentialString(account.Credentials, "chatgpt_account_id")
		if expectedID != "" && expectedID != tokenInfo.ChatGPTAccountID {
			return nil, &service.ReauthExecutionError{Code: "IDENTITY_MISMATCH", Message: "Authorized identity does not match this account"}
		}
		payload = &ApplyOAuthCredentialsRequest{Type: "oauth", Credentials: h.openaiOAuthService.BuildAccountCredentials(tokenInfo)}
		if account.ID == 0 {
			payload.Extra = map[string]any{"email": tokenInfo.Email}
			if tokenInfo.PrivacyMode != "" {
				payload.Extra["privacy_mode"] = tokenInfo.PrivacyMode
			}
		}
	}
	if err != nil {
		return nil, &service.ReauthExecutionError{Code: "INVALID_AUTH_FILE", Message: err.Error()}
	}
	return payload, nil
}

// Pick once per task; OAuth login and token exchange must share the same route.
func chooseReauthProxy(profile *service.ReauthProfile, fallback *int64) (*int64, error) {
	if len(profile.ProxyIDs) == 0 {
		if profile.ProxyID != nil {
			return profile.ProxyID, nil
		}
		return fallback, nil
	}
	index, err := rand.Int(rand.Reader, big.NewInt(int64(len(profile.ProxyIDs))))
	if err != nil {
		return nil, err
	}
	id := profile.ProxyIDs[index.Int64()]
	return &id, nil
}

func (h *AccountHandler) StopOneClickAuth() {
	if h.reauthWorker != nil {
		h.reauthWorker.Stop()
	}
}
