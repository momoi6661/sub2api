package admin

import (
	"net/http"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func (h *SettingHandler) TestReauthScript(c *gin.Context) {
	var input struct {
		Profile    service.ReauthProfile `json:"profile"`
		LoginEmail string                `json:"login_email"`
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 64*1024)
	if err := c.ShouldBindJSON(&input); err != nil {
		response.BadRequest(c, "Invalid script test request")
		return
	}
	if input.Profile.MailboxMode != "api" || input.LoginEmail == "" {
		response.BadRequest(c, "API profile and login email are required")
		return
	}
	profile := input.Profile
	if profile.ID != "" && (profile.CodeScript == "" || (profile.HasSecret && profile.Secret == "")) {
		material, err := h.settingService.ReadReauthScript(c.Request.Context(), profile.ID)
		if err != nil {
			writeReauthError(c, err)
			return
		}
		if profile.CodeScript == "" {
			profile.CodeScript = material["code_script"]
		}
		if profile.Secret == "" {
			profile.Secret = material["secret"]
		}
	}
	if profile.APIResultType == "" {
		profile.APIResultType = "code"
	}
	_, err := service.ExecuteReauthRunner(c.Request.Context(), service.ReauthRunnerTask{Mode: "protocol", Script: profile.CodeScript, ResultType: profile.APIResultType, Context: map[string]any{
		"accountId": 0, "loginEmail": input.LoginEmail, "since": time.Now().UTC().Add(-2 * time.Minute).Format(time.RFC3339), "apiURL": profile.APIURL, "mailboxAddress": profile.MailboxAddress, "secret": profile.Secret,
	}, Input: profile.CustomInput})
	if err != nil {
		writeReauthError(c, err)
		return
	}
	response.Success(c, map[string]bool{"succeeded": true})
}

func (h *SettingHandler) ListReauthProfiles(c *gin.Context) {
	profiles, err := h.settingService.ListReauthProfiles(c.Request.Context())
	if response.ErrorFrom(c, err) {
		return
	}
	response.Success(c, profiles)
}

func (h *SettingHandler) SaveReauthProfile(c *gin.Context) {
	var input service.ReauthProfile
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 32*1024)
	if err := c.ShouldBindJSON(&input); err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid authorization profile")
		return
	}
	profile, err := h.settingService.SaveReauthProfile(c.Request.Context(), c.Param("id"), input)
	if response.ErrorFrom(c, err) {
		return
	}
	response.Success(c, profile)
}

func (h *SettingHandler) DeleteReauthProfile(c *gin.Context) {
	if response.ErrorFrom(c, h.settingService.DeleteReauthProfile(c.Request.Context(), c.Param("id"))) {
		return
	}
	response.Success(c, nil)
}

func (h *SettingHandler) ReadReauthScript(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	material, err := h.settingService.ReadReauthScript(c.Request.Context(), c.Param("id"))
	if response.ErrorFrom(c, err) {
		return
	}
	response.Success(c, material)
}

func (h *SettingHandler) ReadReauthSecret(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	secret, err := h.settingService.ReadReauthSecret(c.Request.Context(), c.Param("id"))
	if response.ErrorFrom(c, err) {
		return
	}
	response.Success(c, map[string]string{"secret": secret})
}
