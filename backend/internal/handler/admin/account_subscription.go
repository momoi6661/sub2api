package admin

import (
	"net/http"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/gin-gonic/gin"
)

// GetSubscription reads current upstream metadata using the saved account identity.
func (h *AccountHandler) GetSubscription(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id < 1 {
		response.BadRequest(c, "Invalid account ID")
		return
	}
	if h.openaiOAuthService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Subscription query service is unavailable")
		return
	}
	account, err := h.adminService.GetAccount(c.Request.Context(), id)
	if response.ErrorFrom(c, err) {
		return
	}
	if account.ParentAccountID != nil {
		account, err = h.adminService.GetAccount(c.Request.Context(), *account.ParentAccountID)
		if response.ErrorFrom(c, err) {
			return
		}
	}
	result, err := h.openaiOAuthService.QueryAccountSubscription(c.Request.Context(), account, c.Query("force") == "true")
	if response.ErrorFrom(c, err) {
		return
	}
	c.Header("Cache-Control", "no-store")
	response.Success(c, result)
}
