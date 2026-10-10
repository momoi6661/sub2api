package admin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type authProgressKey struct{}

// Progress is sent on the task request itself; no polling or background jobs.
func startAuthProgress(c *gin.Context) context.Context {
	ctx := c.Request.Context()
	if c.Query("stream") != "1" {
		return ctx
	}
	c.Set("one_click_auth_stream", true)
	c.Header("Content-Type", "application/x-ndjson")
	c.Header("Cache-Control", "no-store")
	c.Header("X-Accel-Buffering", "no")
	ctx = context.WithValue(ctx, authProgressKey{}, func(stage string) {
		writeAuthEvent(c, map[string]string{"event": "progress", "stage": stage})
	})
	authProgress(ctx, "preparing")
	return ctx
}

func authProgress(ctx context.Context, stage string) {
	if report, ok := ctx.Value(authProgressKey{}).(func(string)); ok && ctx.Err() == nil {
		report(stage)
	}
}

func writeAuthEvent(c *gin.Context, event any) {
	if c.Request.Context().Err() != nil {
		return
	}
	_ = json.NewEncoder(c.Writer).Encode(event)
	c.Writer.Flush()
}

func finishAuthRequest(c *gin.Context, data any, err error) {
	if !c.GetBool("one_click_auth_stream") {
		if err != nil {
			writeReauthError(c, err)
		} else {
			response.Success(c, data)
		}
		return
	}
	if err == nil {
		writeAuthEvent(c, response.Response{Code: 0, Message: "success", Data: data})
		return
	}
	status, info := infraerrors.ToHTTP(err)
	result := response.Response{Code: status, Message: info.Message, Reason: info.Reason, Metadata: info.Metadata}
	var execution *service.ReauthExecutionError
	if errors.As(err, &execution) {
		result.Code = http.StatusUnprocessableEntity
		result.Message, result.Reason = execution.Message, execution.Code
		result.Metadata = map[string]string{"retryable": strconv.FormatBool(execution.Retryable)}
	}
	writeAuthEvent(c, result)
}
