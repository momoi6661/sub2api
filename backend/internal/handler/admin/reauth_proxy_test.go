package admin

import (
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
