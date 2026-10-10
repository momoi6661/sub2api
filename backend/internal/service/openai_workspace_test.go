package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/imroc/req/v3"
)

func TestFetchOpenAIWorkspaceName(t *testing.T) {
	for _, tt := range []struct {
		name, body, target, want string
		status                   int
	}{
		{"selected", `{"accounts":{"personal":{"account":{"account_id":"personal","name":"Personal","plan_type":"free"}},"other":{"account":{"account_id":"other","name":"Other Team","plan_type":"team"}},"selected":{"account":{"account_id":"selected","name":"work5","plan_type":"team"}}}}`, "selected", "work5", 200},
		{"nested ID", `{"accounts":{"different-key":{"account":{"account_id":"selected","name":"work5","plan_type":"team"}}}}`, "selected", "work5", 200},
		{"missing selected", `{"accounts":{"other":{"account":{"account_id":"other","name":"Other Team","plan_type":"team"}}}}`, "selected", "", 200},
		{"conflicting ID", `{"accounts":{"selected":{"account":{"account_id":"other","name":"Other Team","plan_type":"team"}}}}`, "selected", "", 200},
		{"unavailable", `{}`, "selected", "", 403},
	} {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer test-token" {
					t.Error("missing authorization")
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer srv.Close()
			old := chatGPTAccountsCheckURL
			chatGPTAccountsCheckURL = srv.URL
			defer func() { chatGPTAccountsCheckURL = old }()
			factory := func(proxy string) (*req.Client, error) {
				if proxy != "configured-proxy" {
					t.Error("proxy lost")
				}
				return req.C(), nil
			}
			if got := fetchOpenAIWorkspaceName(context.Background(), factory, "test-token", "configured-proxy", tt.target); got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMergeOpenAIWorkspaceNote(t *testing.T) {
	notes := "手动备注\n工作空间: Personal"
	got := mergeOpenAIWorkspaceNote(&notes, "work5")
	if *got != "手动备注\n工作空间: work5" {
		t.Fatal(*got)
	}
	if *mergeOpenAIWorkspaceNote(got, "work5") != *got {
		t.Fatal("duplicate note")
	}
	if mergeOpenAIWorkspaceNote(&notes, "") != &notes {
		t.Fatal("failed query changed notes")
	}
}
