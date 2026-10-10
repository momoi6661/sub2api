package service

import (
	"context"
	"strings"
	"time"
)

// fetchOpenAIWorkspaceName only returns the selected ChatGPT account's name.
// API organization titles in ID tokens describe a different object.
func fetchOpenAIWorkspaceName(ctx context.Context, factory PrivacyClientFactory, token, proxyURL, accountID string) string {
	if factory == nil || token == "" || accountID == "" {
		return ""
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	client, err := factory(proxyURL)
	if err != nil {
		return ""
	}
	var result struct {
		Accounts map[string]struct {
			Account struct {
				ID   string `json:"account_id"`
				Name string `json:"name"`
				Plan string `json:"plan_type"`
			} `json:"account"`
		} `json:"accounts"`
	}
	resp, err := client.R().SetContext(ctx).
		SetHeader("Authorization", "Bearer "+token).
		SetHeader("Origin", "https://chatgpt.com").
		SetHeader("Referer", "https://chatgpt.com/").
		SetHeader("Accept", "application/json").
		SetSuccessResult(&result).Get(chatGPTAccountsCheckURL)
	if err != nil || !resp.IsSuccessState() {
		return ""
	}
	for key, entry := range result.Accounts {
		if entry.Account.ID != accountID && (entry.Account.ID != "" || key != accountID) {
			continue
		}
		if isOpenAITeamPlan(entry.Account.Plan) {
			return strings.TrimSpace(entry.Account.Name)
		}
	}
	return ""
}

func isOpenAITeamPlan(plan string) bool {
	plan = strings.ToLower(strings.TrimSpace(plan))
	return plan == "team" || plan == "business" || strings.Contains(plan, "business")
}

func mergeOpenAIWorkspaceNote(notes *string, name string) *string {
	if name == "" {
		return notes
	}
	var lines []string
	if notes != nil {
		for _, line := range strings.Split(*notes, "\n") {
			if !strings.HasPrefix(strings.TrimSpace(line), "工作空间:") {
				lines = append(lines, line)
			}
		}
	}
	text := strings.TrimSpace(strings.Join(lines, "\n"))
	if text != "" {
		text += "\n"
	}
	text += "工作空间: " + name
	return &text
}

func (s *adminServiceImpl) enrichOpenAIWorkspace(ctx context.Context, account *Account) {
	if account.Platform != PlatformOpenAI || account.Type != AccountTypeOAuth || account.IsCredentialShadow() {
		return
	}
	var proxyURL string
	if account.ProxyID != nil {
		if s.proxyRepo == nil {
			return
		}
		proxy, err := s.proxyRepo.GetByID(ctx, *account.ProxyID)
		if err != nil || proxy == nil {
			return
		}
		proxyURL = proxy.URL()
	}
	name := fetchOpenAIWorkspaceName(ctx, s.privacyClientFactory, account.GetCredential("access_token"), proxyURL, account.GetCredential("chatgpt_account_id"))
	if name != "" {
		account.Notes = mergeOpenAIWorkspaceNote(account.Notes, name)
	}
}
