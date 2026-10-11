package admin

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/mail"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

const authorizationTaskConcurrency = 2

// A bounded worker drains persisted tasks only when woken. Each claimed task
// owns its context and OAuth session; unrelated accounts may run in parallel.
func (h *AccountHandler) SetAuthorizationTasks(repo service.AuthorizationTaskRepository) {
	h.authTaskRepo = repo
	ctx, cancel := context.WithCancel(context.Background())
	h.authTaskCancel = cancel
	h.authTaskWake = make(chan struct{}, 1)
	h.authTaskWG.Add(1)
	go func() {
		defer h.authTaskWG.Done()
		semaphore := make(chan struct{}, authorizationTaskConcurrency)
		for {
			select {
			case <-ctx.Done():
				return
			case <-h.authTaskWake:
			}
		drain:
			for ctx.Err() == nil {
				select {
				case semaphore <- struct{}{}:
				default:
					break drain
				}
				task, err := repo.Claim(ctx)
				if err != nil {
					<-semaphore
					slog.Error("authorization task queue unavailable")
					break
				}
				if task == nil {
					<-semaphore
					break
				}
				h.authTaskWG.Add(1)
				go func(task *service.AuthorizationTask) {
					defer h.authTaskWG.Done()
					defer func() {
						<-semaphore
						h.wakeAuthorizationTasks()
					}()
					h.executeQueuedAuthorization(ctx, task)
				}(task)
			}
		}
	}()
}

func (h *AccountHandler) wakeAuthorizationTasks() {
	select {
	case h.authTaskWake <- struct{}{}:
	default:
	}
}

func (h *AccountHandler) taskSnapshot(ctx context.Context, account *service.Account, source, status string, profileNames ...map[string]string) *service.AuthorizationTask {
	email := codexCredentialString(account.Extra, "one_click_auth_login_email")
	if email == "" {
		email = codexCredentialString(account.Credentials, "email")
	}
	task := &service.AuthorizationTask{AccountID: account.ID, AccountName: account.Name, LoginEmail: email, ProfileID: codexCredentialString(account.Extra, "one_click_auth_profile_id"), Source: source, Status: status}
	if len(profileNames) > 0 {
		task.ProfileName = profileNames[0][task.ProfileID]
	} else if h.reauthSettings != nil {
		profiles, err := h.reauthSettings.ListReauthProfiles(ctx)
		if err == nil {
			for _, p := range profiles {
				if p.ID == task.ProfileID {
					task.ProfileName = p.Name
					break
				}
			}
		}
	}
	return task
}

func (h *AccountHandler) recordedProgress(ctx context.Context, id int64) context.Context {
	original, _ := ctx.Value(authProgressKey{}).(func(string))
	return context.WithValue(ctx, authProgressKey{}, func(stage string) {
		writeCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := h.authTaskRepo.Report(writeCtx, id, stage); err != nil {
			slog.Error("authorization progress could not be saved", "task_id", id)
		}
		if original != nil {
			original(stage)
		}
	})
}

var safeAuthErrorCode = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,79}$`)

func (h *AccountHandler) finishAuthRecord(id int64, success string, err error) {
	status, code, message := success, "", ""
	if err != nil {
		status, code, message = "failed", "AUTH_FAILED", "Authorization failed. Check the configuration or authorize manually."
		var execution *service.ReauthExecutionError
		if errors.As(err, &execution) && safeAuthErrorCode.MatchString(execution.Code) {
			code = execution.Code
			if execution.HTTPStatus >= 400 && execution.HTTPStatus <= 599 {
				message = fmt.Sprintf("Authorization page returned HTTP %d", execution.HTTPStatus)
			}
			if code == "TASK_CANCELLED" {
				status = "cancelled"
			}
		}
		if errors.Is(err, context.Canceled) {
			status = "cancelled"
			code = "TASK_CANCELLED"
		}
		if errors.Is(err, context.DeadlineExceeded) {
			code = "AUTH_TIMEOUT"
		}
		// Never persist arbitrary script/upstream text; only a validated numeric HTTP status.
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := h.authTaskRepo.Finish(ctx, id, status, code, message); err != nil {
		slog.Error("authorization task result could not be saved", "task_id", id)
	}
}

func (h *AccountHandler) authorizeRecorded(ctx context.Context, id int64, source string) (*service.Account, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	account, err := h.adminService.GetAccount(ctx, id)
	if err != nil {
		return nil, err
	}
	task := h.taskSnapshot(ctx, account, source, "running")
	if err = h.authTaskRepo.Create(ctx, []*service.AuthorizationTask{task}); err != nil {
		return nil, err
	}
	h.authTaskControllers.Store(task.ID, cancel)
	defer h.authTaskControllers.Delete(task.ID)
	currentTask, readErr := h.authTaskRepo.Get(ctx, task.ID)
	if readErr != nil || currentTask.Status != "running" {
		return nil, &service.ReauthExecutionError{Code: "TASK_CANCELLED", Message: "Task cancelled"}
	}
	result, err := h.authorizeOneClick(h.recordedProgress(ctx, task.ID), id)
	if ctx.Err() != nil {
		err = ctx.Err()
	}
	h.finishAuthRecord(task.ID, "succeeded", err)
	return result, err
}

func (h *AccountHandler) executeQueuedAuthorization(parent context.Context, task *service.AuthorizationTask) {
	ctx, cancel := context.WithTimeout(parent, 10*time.Minute)
	h.authTaskControllers.Store(task.ID, cancel)
	defer h.authTaskControllers.Delete(task.ID)
	defer cancel()
	var resultErr error
	defer func() {
		if recover() != nil {
			resultErr = &service.ReauthExecutionError{Code: "TASK_INTERRUPTED", Message: "Task interrupted"}
		}
		if ctx.Err() != nil {
			resultErr = ctx.Err()
		}
		h.finishAuthRecord(task.ID, "succeeded", resultErr)
	}()
	currentTask, readErr := h.authTaskRepo.Get(ctx, task.ID)
	if readErr != nil || currentTask.Status != "running" {
		resultErr = context.Canceled
		return
	}
	account, err := h.adminService.GetAccount(ctx, task.AccountID)
	if err != nil {
		resultErr = &service.ReauthExecutionError{Code: "ACCOUNT_UNAVAILABLE", Message: "Account no longer available"}
		return
	}
	current := h.taskSnapshot(ctx, account, task.Source, "running", map[string]string{})
	if current.ProfileID != task.ProfileID || !strings.EqualFold(current.LoginEmail, task.LoginEmail) {
		resultErr = &service.ReauthExecutionError{Code: "ACCOUNT_CHANGED", Message: "Account authorization binding changed while queued"}
		return
	}
	_, resultErr = h.authorizeOneClick(h.recordedProgress(ctx, task.ID), task.AccountID)
}

func (h *AccountHandler) SubmitAuthorizationTasks(c *gin.Context) {
	var input struct {
		AccountIDs     []int64 `json:"account_ids" binding:"required,min=1,max=100"`
		ProfileID      string  `json:"profile_id" binding:"max=128"`
		LoginEmail     string  `json:"login_email" binding:"max=254"`
		CustomInput    *string `json:"custom_input" binding:"omitempty,max=16384"`
		WorkspaceIndex *int    `json:"workspace_index" binding:"omitempty,min=1"`
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 32*1024)
	if err := c.ShouldBindJSON(&input); err != nil {
		response.BadRequest(c, "Select between 1 and 100 accounts")
		return
	}
	tasks := make([]*service.AuthorizationTask, 0, len(input.AccountIDs))
	seen := make(map[int64]bool)
	names := make(map[string]string)
	enabled := make(map[string]bool)
	custom := make(map[string]bool)
	if h.reauthSettings != nil {
		profiles, err := h.reauthSettings.ListReauthProfiles(c.Request.Context())
		if err != nil {
			response.ErrorFrom(c, err)
			return
		}
		for _, p := range profiles {
			names[p.ID] = p.Name
			enabled[p.ID] = p.Enabled
			custom[p.ID] = p.MailboxMode == "api"
		}
	}
	if input.ProfileID != "" || input.LoginEmail != "" || input.WorkspaceIndex != nil {
		input.LoginEmail = strings.TrimSpace(input.LoginEmail)
		address, err := mail.ParseAddress(input.LoginEmail)
		if len(input.AccountIDs) != 1 || !enabled[input.ProfileID] || (!custom[input.ProfileID] && (err != nil || address.Address != input.LoginEmail)) {
			response.BadRequest(c, "Choose an enabled profile and valid login email for one account")
			return
		}
	}
	source := "manual"
	if len(input.AccountIDs) > 1 {
		source = "batch"
	}
	for _, id := range input.AccountIDs {
		if id < 1 || seen[id] {
			response.BadRequest(c, "Account IDs must be positive and unique")
			return
		}
		seen[id] = true
		account, err := h.adminService.GetAccount(c.Request.Context(), id)
		if err != nil {
			response.ErrorFrom(c, err)
			return
		}
		if account.Platform != service.PlatformOpenAI || !account.IsOAuth() || account.IsShadow() {
			response.BadRequest(c, "Select non-shadow OpenAI OAuth accounts")
			return
		}
		if input.ProfileID != "" {
			updates := map[string]any{"one_click_auth_profile_id": input.ProfileID, "one_click_auth_login_email": input.LoginEmail}
			if custom[input.ProfileID] {
				delete(updates, "one_click_auth_login_email")
				if input.CustomInput != nil {
					updates["one_click_auth_custom_input"] = *input.CustomInput
				}
			}
			if input.WorkspaceIndex != nil {
				updates["one_click_auth_workspace_index"] = *input.WorkspaceIndex
			}
			if account.Extra == nil {
				account.Extra = make(map[string]any)
			}
			if account.Extra["one_click_auth_profile_id"] != input.ProfileID || account.Extra["one_click_auth_login_email"] != input.LoginEmail {
				updates["one_click_auth_auto_attempt"] = ""
			}
			if err := h.adminService.UpdateAccountExtra(c.Request.Context(), id, updates); err != nil {
				response.ErrorFrom(c, err)
				return
			}
			for key, value := range updates {
				account.Extra[key] = value
			}
		}
		tasks = append(tasks, h.taskSnapshot(c.Request.Context(), account, source, "queued", names))
	}
	if err := h.authTaskRepo.Create(c.Request.Context(), tasks); err != nil {
		writeReauthError(c, err)
		return
	}
	h.wakeAuthorizationTasks()
	response.Success(c, tasks)
}

func (h *AccountHandler) ListAuthorizationTasks(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 || page > 1000000 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}
	status := c.Query("status")
	switch status {
	case "", "active", "queued", "running", "succeeded", "ready", "failed", "cancelled":
	default:
		response.BadRequest(c, "Invalid task status")
		return
	}
	q := strings.TrimSpace(c.Query("q"))
	if len(q) > 254 {
		response.BadRequest(c, "Search is too long")
		return
	}
	result, err := h.authTaskRepo.List(c.Request.Context(), q, status, page, size)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *AccountHandler) GetAuthorizationTask(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id < 1 {
		response.BadRequest(c, "Invalid task ID")
		return
	}
	task, err := h.authTaskRepo.Get(c.Request.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		response.NotFound(c, "Task not found")
		return
	}
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, task)
}

func (h *AccountHandler) CancelAuthorizationTask(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id < 1 {
		response.BadRequest(c, "Invalid task ID")
		return
	}
	if err := h.authTaskRepo.Cancel(c.Request.Context(), id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if cancel, ok := h.authTaskControllers.Load(id); ok {
		cancel.(context.CancelFunc)()
	}
	task, err := h.authTaskRepo.Get(c.Request.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		response.NotFound(c, "Task not found")
		return
	}
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, task)
}

func (h *AccountHandler) DeleteAuthorizationTask(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id < 1 {
		response.BadRequest(c, "Invalid task ID")
		return
	}
	if cancel, ok := h.authTaskControllers.Load(id); ok {
		cancel.(context.CancelFunc)()
	}
	if err := h.authTaskRepo.Cancel(c.Request.Context(), id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if err := h.authTaskRepo.Delete(c.Request.Context(), id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"id": id})
}
