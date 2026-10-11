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
		{",\"workspace_index\":0", false}, {",\"workspace_index\":-1", false}, {",\"workspace_index\":1.5", false},
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
