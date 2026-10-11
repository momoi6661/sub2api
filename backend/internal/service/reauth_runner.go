package service

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type ReauthExecutionError struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	Retryable  bool   `json:"retryable"`
	HTTPStatus int    `json:"httpStatus,omitempty"`
}

func (e *ReauthExecutionError) Error() string { return e.Message }

type ReauthRunnerMailbox struct {
	Host         string `json:"host"`
	Port         int    `json:"port"`
	Username     string `json:"username"`
	Folder       string `json:"folder"`
	SenderFilter string `json:"senderFilter"`
	CodePattern  string `json:"codePattern"`
	WaitSeconds  int    `json:"waitSeconds"`
}

type ReauthRunnerTask struct {
	ResendAfterSeconds int                  `json:"resendAfterSeconds,omitempty"`
	OnProgress         func(string)         `json:"-"`
	Mailbox            *ReauthRunnerMailbox `json:"mailbox,omitempty"`
	Mode               string               `json:"mode"`
	Script             string               `json:"script"`
	ResultType         string               `json:"resultType"`
	Context            map[string]any       `json:"context"`
	ProxyURL           string               `json:"proxyURL,omitempty"`
}

type ReauthRunnerResult struct {
	Type  string                `json:"type"`
	Code  string                `json:"code"`
	State string                `json:"state"`
	Auth  json.RawMessage       `json:"auth"`
	Error *ReauthExecutionError `json:"error,omitempty"`
}

func ExecuteReauthRunner(ctx context.Context, task ReauthRunnerTask) (*ReauthRunnerResult, error) {
	endpoint := strings.TrimRight(os.Getenv("ONE_CLICK_AUTH_RUNNER_URL"), "/")
	token := os.Getenv("ONE_CLICK_AUTH_RUNNER_TOKEN")
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || len(token) < 32 {
		return nil, &ReauthExecutionError{Code: "RUNNER_NOT_CONFIGURED", Message: "Configure the one-click authorization runner URL and token first"}
	}
	body, err := json.Marshal(task)
	if err != nil {
		return nil, err
	}
	runCtx, cancel := context.WithTimeout(ctx, 605*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(runCtx, http.MethodPost, endpoint+"/execute", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/x-ndjson")
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 605 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		if runCtx.Err() != nil {
			return nil, runCtx.Err()
		}
		return nil, &ReauthExecutionError{Code: "RUNNER_UNAVAILABLE", Message: "Authorization runner unavailable or timed out", Retryable: true}
	}
	defer resp.Body.Close()
	limited := io.LimitReader(resp.Body, 256*1024+1)
	var data []byte
	if strings.HasPrefix(resp.Header.Get("Content-Type"), "application/x-ndjson") {
		scanner := bufio.NewScanner(limited)
		scanner.Buffer(make([]byte, 4096), 256*1024)
		total := 0
		for scanner.Scan() {
			line := scanner.Bytes()
			total += len(line) + 1
			if total > 256*1024 {
				return nil, &ReauthExecutionError{Code: "INVALID_RUNNER_RESULT", Message: "Runner response exceeds size limit"}
			}
			var progress struct {
				Event string `json:"event"`
				Stage string `json:"stage"`
			}
			if json.Unmarshal(line, &progress) == nil && progress.Event == "progress" {
				switch progress.Stage {
				case "mailbox_connect", "protocol_start", "email_login", "requesting_code", "waiting_code", "submitting_code", "consent", "token_exchange":
					if task.OnProgress != nil {
						task.OnProgress(progress.Stage)
					}
				}
			} else {
				data = append(data[:0], line...)
			}
		}
		err = scanner.Err()
	} else {
		data, err = io.ReadAll(limited)
	}
	if err != nil || len(data) > 256*1024 {
		if runCtx.Err() != nil {
			return nil, runCtx.Err()
		}
		return nil, &ReauthExecutionError{Code: "INVALID_RUNNER_RESULT", Message: "Runner response exceeds size limit"}
	}
	var result ReauthRunnerResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, &ReauthExecutionError{Code: "INVALID_RUNNER_RESULT", Message: "Runner returned invalid JSON"}
	}
	if result.Type == "error" && result.Error != nil {
		if result.Error.Code == "" || result.Error.Message == "" {
			return nil, &ReauthExecutionError{Code: "INVALID_ERROR_RESULT", Message: "Runner error requires code and message"}
		}
		return nil, result.Error
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReauthExecutionError{Code: "RUNNER_REJECTED", Message: fmt.Sprintf("Runner rejected the request (HTTP %d)", resp.StatusCode), Retryable: resp.StatusCode == 429}
	}
	if (task.Mode == "oauth_protocol" && result.Type != "auth_file") || (task.Mode == "protocol" && result.Type != task.ResultType) {
		return nil, &ReauthExecutionError{Code: "RESULT_TYPE_MISMATCH", Message: "Runner result type does not match the task"}
	}
	return &result, nil
}
