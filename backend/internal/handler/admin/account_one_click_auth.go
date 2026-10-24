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

func (h *AccountHandler) SetReauthSettings(settings *SettingHandler, locks service.LeaderLockCache, events *service.AccountErrorEvents) {
	if settings != nil {
		h.reauthSettings = settings.settingService
		h.reauthLocks = locks
		h.reauthWorker = service.NewOneClickAuthWorker(h.adminService, locks, func(ctx context.Context, id int64) error {
			_, err := h.authorizeRecorded(ctx, id, "automatic")
			return err
		})
		h.reauthWorker.Start()
		if events != nil {
			events.SetHandler(h.reauthWorker.Notify)
		}
		h.wakeAuthorizationTasks()
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
	ctx := startAuthProgress(c)
	account, err := h.authorizeRecorded(ctx, id, "manual")
	if err != nil {
		finishAuthRequest(c, nil, err)
		return
	}
	finishAuthRequest(c, h.buildAccountResponseWithRuntime(c.Request.Context(), account), nil)
}

func (h *AccountHandler) authorizeOneClick(parent context.Context, id int64) (*service.Account, error) {
	if _, loaded := oneClickAccountLocks.LoadOrStore(id, true); loaded {
		return nil, &service.ReauthExecutionError{Code: "TASK_ALREADY_RUNNING", Message: "This account already has an authorization task"}
	}
	defer oneClickAccountLocks.Delete(id)
	if h.reauthSettings == nil {
		return nil, &service.ReauthExecutionError{Code: "RUNNER_NOT_CONFIGURED", Message: "Authorization settings unavailable"}
	}
	ctx, cancel := context.WithTimeout(parent, 10*time.Minute)
	defer cancel()
	if h.reauthLocks != nil {
		var nonce [16]byte
		if _, err := rand.Read(nonce[:]); err != nil {
			return nil, err
		}
		owner, key := hex.EncodeToString(nonce[:]), fmt.Sprintf("one_click_auth_task:%d", id)
		acquired, err := h.reauthLocks.TryAcquireLeaderLock(ctx, key, owner, 11*time.Minute)
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
	if !reflect.DeepEqual(current.Credentials, account.Credentials) || current.Extra["one_click_auth_profile_id"] != profileID || current.Extra["one_click_auth_login_email"] != account.Extra["one_click_auth_login_email"] || !reflect.DeepEqual(current.Extra["one_click_auth_workspace_index"], account.Extra["one_click_auth_workspace_index"]) || !reflect.DeepEqual(current.Extra["one_click_auth_custom_input"], account.Extra["one_click_auth_custom_input"]) {
		return nil, &service.ReauthExecutionError{Code: "ACCOUNT_CHANGED", Message: "Account credentials or authorization binding changed during this task"}
	}
	authProgress(ctx, "saving")
	return h.applyOAuthCredentials(ctx, id, payload)
}

// AuthorizeNewAccount returns verified credentials without creating a placeholder account.
func (h *AccountHandler) AuthorizeNewAccount(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	var input struct {
		ProfileID      string  `json:"profile_id" binding:"required,max=128"`
		LoginEmail     string  `json:"login_email" binding:"max=254"`
		CustomInput    *string `json:"custom_input" binding:"omitempty,max=16384"`
		ProxyID        *int64  `json:"proxy_id"`
		WorkspaceIndex *int    `json:"workspace_index" binding:"omitempty,min=0"`
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 32*1024)
	if err := c.ShouldBindJSON(&input); err != nil {
		response.BadRequest(c, "Select an authorization profile and enter a login email")
		return
	}
	input.LoginEmail = strings.TrimSpace(input.LoginEmail)
	address, err := mail.ParseAddress(input.LoginEmail)
	if (input.CustomInput == nil && (err != nil || address.Address != input.LoginEmail)) || (input.ProxyID != nil && *input.ProxyID < 1) {
		response.BadRequest(c, "Invalid login email or proxy ID")
		return
	}
	if h.reauthSettings == nil {
		writeReauthError(c, &service.ReauthExecutionError{Code: "RUNNER_NOT_CONFIGURED", Message: "Authorization settings unavailable"})
		return
	}
	ctx, cancel := context.WithTimeout(startAuthProgress(c), 10*time.Minute)
	ctx, taskCancel := context.WithCancel(ctx)
	defer cancel()
	defer taskCancel()
	workspaceIndex := 1
	if input.WorkspaceIndex != nil {
		workspaceIndex = *input.WorkspaceIndex
	}
	draft := &service.Account{Platform: service.PlatformOpenAI, Type: "oauth", ProxyID: input.ProxyID, Extra: map[string]any{
		"one_click_auth_profile_id": input.ProfileID, "one_click_auth_login_email": input.LoginEmail, "one_click_auth_workspace_index": workspaceIndex,
	}}
	if input.CustomInput != nil {
		draft.Extra["one_click_auth_custom_input"] = *input.CustomInput
	}
	record := h.taskSnapshot(ctx, draft, "new_account", "running")
	if err := h.authTaskRepo.Create(ctx, []*service.AuthorizationTask{record}); err != nil {
		finishAuthRequest(c, nil, err)
		return
	}
	h.authTaskControllers.Store(record.ID, taskCancel)
	defer h.authTaskControllers.Delete(record.ID)
	currentTask, readErr := h.authTaskRepo.Get(ctx, record.ID)
	if readErr != nil || currentTask.Status != "running" {
		finishAuthRequest(c, nil, context.Canceled)
		return
	}
	ctx = h.recordedProgress(ctx, record.ID)
	c.Set("auth_task_finish", func(err error) {
		if ctx.Err() != nil {
			err = ctx.Err()
		}
		h.finishAuthRecord(record.ID, "ready", err)
	})
	// Keep email out of lock keys, and prevent repeat requests for the same draft.
	digest := sha256.Sum256([]byte(input.ProfileID + "\x00" + strings.ToLower(input.LoginEmail)))
	key := fmt.Sprintf("one_click_auth_draft:%x", digest)
	if _, loaded := oneClickAccountLocks.LoadOrStore(key, true); loaded {
		finishAuthRequest(c, nil, &service.ReauthExecutionError{Code: "TASK_ALREADY_RUNNING", Message: "This email already has an authorization task"})
		return
	}
	defer oneClickAccountLocks.Delete(key)
	if h.reauthLocks != nil {
		var nonce [16]byte
		if _, err := rand.Read(nonce[:]); err != nil {
			finishAuthRequest(c, nil, err)
			return
		}
		owner := hex.EncodeToString(nonce[:])
		acquired, err := h.reauthLocks.TryAcquireLeaderLock(ctx, key, owner, 11*time.Minute)
		if err != nil || !acquired {
			execution := &service.ReauthExecutionError{Code: "TASK_ALREADY_RUNNING", Message: "This email already has an authorization task"}
			if err != nil {
				execution = &service.ReauthExecutionError{Code: "TASK_LOCK_UNAVAILABLE", Message: "Authorization task lock unavailable", Retryable: true}
			}
			finishAuthRequest(c, nil, execution)
			return
		}
		defer func() {
			releaseCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_ = h.reauthLocks.ReleaseLeaderLock(releaseCtx, key, owner)
		}()
	}
	payload, err := h.authorizeReauthProfile(ctx, draft)
	if err != nil {
		finishAuthRequest(c, nil, err)
		return
	}
	finishAuthRequest(c, payload, nil)
}

// Shared execution keeps draft and existing-account authorization rules identical.
func (h *AccountHandler) authorizeReauthProfile(ctx context.Context, account *service.Account) (*ApplyOAuthCredentialsRequest, error) {
	authProgress(ctx, "configuration")
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
	if email == "" && !(profile.MailboxMode == "api" && profile.APIResultType == "auth_file") {
		return nil, &service.ReauthExecutionError{Code: "LOGIN_EMAIL_REQUIRED", Message: "Configure this account's login email"}
	}
	resultType := profile.APIResultType
	if profile.MailboxMode == "imap" || resultType == "" {
		resultType = "code"
	}
	task := service.ReauthRunnerTask{ResendAfterSeconds: profile.EmailResendSeconds, Mode: "protocol", Script: material["code_script"], ResultType: resultType, Input: material["custom_input"], Context: map[string]any{
		"accountId": account.ID, "loginEmail": email, "since": time.Now().UTC().Format(time.RFC3339), "apiURL": material["api_url"], "mailboxAddress": material["mailbox_address"], "secret": material["secret"],
	}}
	if profile.MailboxMode == "api" {
		task.Input, _ = account.Extra["one_click_auth_custom_input"].(string)
		delete(task.Context, "secret")
		delete(task.Context, "mailboxAddress")
	}
	workspaceIndex, expectedWorkspaceID, err := reauthWorkspaceSelection(account)
	if err != nil {
		return nil, err
	}
	task.Context["workspaceIndex"] = workspaceIndex
	task.OnProgress = func(stage string) { authProgress(ctx, stage) }
	if profile.MailboxMode == "imap" {
		task.Mailbox = &service.ReauthRunnerMailbox{Host: profile.IMAPHost, Port: profile.IMAPPort, Username: profile.IMAPUsername, Folder: profile.Folder, SenderFilter: profile.SenderFilter, CodePattern: profile.CodePattern, WaitSeconds: profile.MailboxWaitSeconds}
		// Google displays app passwords in groups; spaces are not part of the password.
		if strings.EqualFold(profile.IMAPHost, "imap.gmail.com") {
			task.Context["secret"] = strings.ReplaceAll(material["secret"], " ", "")
		}
	}

	authProgress(ctx, "connecting")
	proxyID, err := chooseReauthAccountProxy(profile, account)
	if err != nil {
		return nil, err
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

	if resultType == "auth_file" {
		authProgress(ctx, "authorization_file")
	} else {
		task.Mode = "oauth_protocol"
		task.Context["expectedAccountID"] = expectedWorkspaceID
		authProgress(ctx, "protocol_authorization")
	}
	task.OnResult, _ = ctx.Value(authResultKey{}).(func(json.RawMessage))
	result, err := service.ExecuteReauthRunner(ctx, task)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	authProgress(ctx, "verifying")
	content := string(result.Auth)
	var text string
	if json.Unmarshal(result.Auth, &text) == nil {
		content = text
	}
	payload, err := normalizeProfileReauthAuthFile(content, account)
	if err != nil {
		return nil, &service.ReauthExecutionError{Code: "INVALID_AUTH_FILE", Message: err.Error()}
	}
	// Both protocol login and custom authorization files need subscription metadata.
	// Identity has already been checked by normalizeReauthAuthFile above.
	if h.openaiOAuthService != nil {
		credentials := payload.Credentials
		authorizedEmail := codexCredentialString(credentials, "email")
		expires, _ := time.Parse(time.RFC3339, codexCredentialString(credentials, "expires_at"))
		tokenInfo := &service.OpenAITokenInfo{
			AccessToken: codexCredentialString(credentials, "access_token"), RefreshToken: codexCredentialString(credentials, "refresh_token"),
			IDToken: codexCredentialString(credentials, "id_token"), ClientID: codexCredentialString(credentials, "client_id"), ExpiresAt: expires.Unix(),
			Email: codexCredentialString(credentials, "email"), ChatGPTAccountID: codexCredentialString(credentials, "chatgpt_account_id"),
			ChatGPTUserID: codexCredentialString(credentials, "chatgpt_user_id"), OrganizationID: codexCredentialString(credentials, "organization_id"),
			PlanType: codexCredentialString(credentials, "plan_type"),
		}
		h.openaiOAuthService.EnrichAuthorizedTokenInfo(ctx, tokenInfo, task.ProxyURL)
		if (authorizedEmail != "" && !strings.EqualFold(authorizedEmail, tokenInfo.Email)) || tokenInfo.ChatGPTAccountID != codexCredentialString(credentials, "chatgpt_account_id") {
			return nil, &service.ReauthExecutionError{Code: "IDENTITY_MISMATCH", Message: "Authorized identity does not match this account"}
		}
		payload.Credentials = service.MergeCredentials(credentials, h.openaiOAuthService.BuildAccountCredentials(tokenInfo))
		if account.ID == 0 && tokenInfo.PrivacyMode != "" {
			payload.Extra["privacy_mode"] = tokenInfo.PrivacyMode
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return payload, nil
}

// Pick once per task; OAuth login and token exchange must share the same route.
func chooseReauthAccountProxy(profile *service.ReauthProfile, account *service.Account) (*int64, error) {
	// The draft selector is authoritative, including nil for a direct route.
	if account.ID == 0 {
		return account.ProxyID, nil
	}
	return chooseReauthProxy(profile, account.ProxyID)
}

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
	if h.authTaskCancel != nil {
		h.authTaskCancel()
	}
	if h.reauthWorker != nil {
		h.reauthWorker.Stop()
	}
	h.authTaskWG.Wait()
}

// An explicitly saved index is the administrator's selection. Legacy accounts
// retain their original identity until an index is configured.
func reauthWorkspaceSelection(account *service.Account) (int, string, error) {
	value, explicit := account.Extra["one_click_auth_workspace_index"]
	if !explicit {
		return 1, codexCredentialString(account.Credentials, "chatgpt_account_id"), nil
	}
	index := 1
	encoded, err := json.Marshal(value)
	if err != nil || json.Unmarshal(encoded, &index) != nil || index < 0 {
		return 0, "", &service.ReauthExecutionError{Code: "INVALID_WORKSPACE_INDEX", Message: "Workspace index must be a non-negative integer (0 selects personal)"}
	}
	return index, "", nil
}
