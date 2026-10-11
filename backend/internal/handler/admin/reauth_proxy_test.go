package admin

import (
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestDraftReauthProxySelection(t *testing.T) {
	selected := int64(7)
	profile := &service.ReauthProfile{ProxyIDs: []int64{99}}
	for _, id := range []*int64{nil, &selected} {
		got, err := chooseReauthAccountProxy(profile, &service.Account{ProxyID: id})
		if err != nil || got != id {
			t.Fatalf("draft proxy selector was overridden: got %v, err %v", got, err)
		}
	}
	got, err := chooseReauthAccountProxy(profile, &service.Account{ID: 1, ProxyID: &selected})
	if err != nil || got == nil || *got != 99 {
		t.Fatalf("existing-account profile pool selection failed: got %v, err %v", got, err)
	}
}

func TestAuthorizationWorkspaceIndexValidation(t *testing.T) {
	for _, tc := range []struct {
		value string
		valid bool
	}{
		{"", true}, {",\"workspace_index\":1", true}, {",\"workspace_index\":2", true},
		{",\"workspace_index\":0", true}, {",\"workspace_index\":-1", false}, {",\"workspace_index\":1.5", false},
	} {
		t.Run(tc.value, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("POST", "/", strings.NewReader(`{"profile_id":"test","login_email":"owner@example.com"`+tc.value+`}`))
			c.Request.Header.Set("Content-Type", "application/json")
			h := &AccountHandler{}
			h.AuthorizeNewAccount(c)
			want := 400
			if tc.valid {
				want = 503
			} // Valid input proceeds to the runner configuration check.
			if w.Code != want {
				t.Fatalf("got status %d, want %d", w.Code, want)
			}
		})
	}
}

func TestReauthWorkspaceSelection(t *testing.T) {
	for _, tc := range []struct {
		name     string
		extra    map[string]any
		index    int
		expected string
		invalid  bool
	}{
		{"legacy identity", nil, 1, "old-workspace", false},
		{"explicit second workspace", map[string]any{"one_click_auth_workspace_index": float64(2)}, 2, "", false},
		{"explicit default", map[string]any{"one_click_auth_workspace_index": 1}, 1, "", false},
		{"personal selected", map[string]any{"one_click_auth_workspace_index": 0}, 0, "", false},
		{"fraction rejected", map[string]any{"one_click_auth_workspace_index": 1.5}, 0, "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			index, expected, err := reauthWorkspaceSelection(&service.Account{Extra: tc.extra, Credentials: map[string]any{"chatgpt_account_id": "old-workspace"}})
			if (err != nil) != tc.invalid || index != tc.index || expected != tc.expected {
				t.Fatalf("got index %d, expected %q, error %v", index, expected, err)
			}
		})
	}
}
