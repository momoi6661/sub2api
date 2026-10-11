package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestReauthRunnerProtocolResultAndProgress(t *testing.T) {
	for _, tc := range []struct {
		name, mode, inputType, outputType string
		wantError                         bool
	}{
		{"code login produces auth file", "oauth_protocol", "code", "auth_file", false},
		{"login rejects raw code", "oauth_protocol", "code", "code", true},
		{"direct file script", "protocol", "auth_file", "auth_file", false},
		{"standalone code script test", "protocol", "code", "code", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/execute" || r.Header.Get("Authorization") != "Bearer 01234567890123456789012345678901" {
					t.Errorf("unexpected runner request")
				}
				var input ReauthRunnerTask
				if err := json.NewDecoder(r.Body).Decode(&input); err != nil || input.Mode != tc.mode || input.ResultType != tc.inputType || input.ResendAfterSeconds != 45 {
					t.Errorf("wrong runner task: %+v, %v", input, err)
				}
				w.Header().Set("Content-Type", "application/x-ndjson")
				fmt.Fprintln(w, `{"event":"progress","stage":"protocol_start"}`)
				fmt.Fprintln(w, `{"event":"progress","stage":"token_exchange"}`)
				fmt.Fprintf(w, "{\"type\":%q,\"auth\":\"{\\\"type\\\":\\\"codex\\\"}\"}\n", tc.outputType)
			}))
			defer server.Close()
			t.Setenv("ONE_CLICK_AUTH_RUNNER_URL", server.URL)
			t.Setenv("ONE_CLICK_AUTH_RUNNER_TOKEN", "01234567890123456789012345678901")
			var stages []string
			result, err := ExecuteReauthRunner(context.Background(), ReauthRunnerTask{
				Mode: tc.mode, ResultType: tc.inputType, ResendAfterSeconds: 45, OnProgress: func(stage string) { stages = append(stages, stage) },
			})
			if tc.wantError {
				var execution *ReauthExecutionError
				if !errors.As(err, &execution) || execution.Code != "RESULT_TYPE_MISMATCH" {
					t.Fatalf("expected result mismatch, got %v", err)
				}
			} else if err != nil || result.Type != tc.outputType {
				t.Fatalf("unexpected result: %+v, %v", result, err)
			}
			if len(stages) != 2 || stages[0] != "protocol_start" || stages[1] != "token_exchange" {
				t.Fatalf("missing protocol progress: %v", stages)
			}
		})
	}
}

func TestReauthRunnerPreservesScriptReturnBeforeAuthorizationError(t *testing.T) {
	returned := `{"type":"code","code":"123456","custom":{"items":[1,2]}}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"type":"error","error":{"code":"CONSENT_FAILED","message":"Consent failed"},"script_result":%s}`, returned)
	}))
	defer server.Close()
	t.Setenv("ONE_CLICK_AUTH_RUNNER_URL", server.URL)
	t.Setenv("ONE_CLICK_AUTH_RUNNER_TOKEN", "01234567890123456789012345678901")
	var saved json.RawMessage
	_, err := ExecuteReauthRunner(context.Background(), ReauthRunnerTask{
		Mode: "oauth_protocol", ResultType: "code", OnResult: func(value json.RawMessage) { saved = value },
	})
	var execution *ReauthExecutionError
	if !errors.As(err, &execution) || execution.Code != "CONSENT_FAILED" {
		t.Fatalf("expected authorization error, got %v", err)
	}
	if string(saved) != returned {
		t.Fatalf("script return was lost: %s", saved)
	}
}
