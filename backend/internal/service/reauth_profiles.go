package service

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
	"regexp"
	"sort"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const reauthProfilePrefix = "one_click_auth_profile."

// ReauthProfile describes a shared inbox, not the upstream account's login identity.
// List responses omit secrets; explicit administrator reads decrypt them on demand.
type ReauthProfile struct {
	ID                 string  `json:"id"`
	Name               string  `json:"name"`
	Enabled            bool    `json:"enabled"`
	MailboxMode        string  `json:"mailbox_mode"`
	APIResultType      string  `json:"api_result_type"`
	MailboxAddress     string  `json:"mailbox_address"`
	IMAPHost           string  `json:"imap_host"`
	IMAPPort           int     `json:"imap_port"`
	IMAPUsername       string  `json:"imap_username"`
	APIURL             string  `json:"api_url"`
	CodeScript         string  `json:"code_script,omitempty"`
	CustomInput        string  `json:"custom_input,omitempty"`
	HasCodeScript      bool    `json:"has_code_script"`
	Folder             string  `json:"folder"`
	SenderFilter       string  `json:"sender_filter"`
	CodePattern        string  `json:"code_pattern"`
	MailboxWaitSeconds int     `json:"mailbox_wait_seconds"`
	EmailResendSeconds int     `json:"email_resend_seconds"`
	ProxyID            *int64  `json:"proxy_id"`
	ProxyIDs           []int64 `json:"proxy_ids"`
	Secret             string  `json:"secret,omitempty"`
	HasSecret          bool    `json:"has_secret"`
}

type storedReauthProfile struct {
	ReauthProfile
	EncryptedSecret string `json:"encrypted_secret,omitempty"`
	EncryptedScript string `json:"encrypted_script,omitempty"`
}

func (s *SettingService) ListReauthProfiles(ctx context.Context) ([]ReauthProfile, error) {
	values, err := s.settingRepo.GetAll(ctx)
	if err != nil {
		return nil, err
	}
	profiles := make([]ReauthProfile, 0)
	for key, value := range values {
		if !strings.HasPrefix(key, reauthProfilePrefix) {
			continue
		}
		var stored storedReauthProfile
		if err := json.Unmarshal([]byte(value), &stored); err != nil {
			return nil, fmt.Errorf("decode authorization profile: %w", err)
		}
		if stored.MailboxWaitSeconds == 0 {
			stored.MailboxWaitSeconds = 60
		}
		if stored.EmailResendSeconds == 0 {
			stored.EmailResendSeconds = 30
		}
		stored.Secret = ""
		stored.CodeScript = ""
		stored.HasCodeScript = stored.EncryptedScript != ""
		stored.HasSecret = stored.EncryptedSecret != ""
		profiles = append(profiles, stored.ReauthProfile)
	}
	sort.Slice(profiles, func(i, j int) bool { return profiles[i].ID < profiles[j].ID })
	return profiles, nil
}

func validReauthProfileID(id string) bool {
	decoded, err := hex.DecodeString(id)
	return err == nil && len(decoded) == 16
}

func (s *SettingService) SaveReauthProfile(ctx context.Context, id string, profile ReauthProfile) (*ReauthProfile, error) {
	bad := func(message string) (*ReauthProfile, error) {
		return nil, infraerrors.BadRequest("INVALID_AUTH_PROFILE", message)
	}
	if profile.MailboxWaitSeconds == 0 {
		profile.MailboxWaitSeconds = 60
	}
	if profile.MailboxWaitSeconds < 30 || profile.MailboxWaitSeconds > 90 {
		return bad("mailbox wait must be between 30 and 90 seconds")
	}
	if profile.EmailResendSeconds == 0 {
		profile.EmailResendSeconds = 30
	}
	if profile.EmailResendSeconds < 10 || profile.EmailResendSeconds > 90 {
		return bad("email resend countdown must be between 10 and 90 seconds")
	}
	profile.Name = strings.TrimSpace(profile.Name)
	profile.MailboxAddress = strings.TrimSpace(profile.MailboxAddress)
	if profile.Name == "" || len(profile.Name) > 120 {
		return bad("profile name is required (maximum 120 bytes)")
	}
	if profile.APIResultType == "" {
		profile.APIResultType = "code"
	}
	if profile.APIResultType != "code" && profile.APIResultType != "auth_file" {
		return bad("API result type must be code or auth_file")
	}
	if address, err := mail.ParseAddress(profile.MailboxAddress); profile.MailboxMode == "imap" && (err != nil || address.Address != profile.MailboxAddress) {
		return bad("a valid mailbox address is required")
	}
	if profile.MailboxMode != "imap" && profile.MailboxMode != "api" {
		return bad("mailbox mode must be imap or api")
	}
	if profile.MailboxMode == "imap" {
		profile.IMAPHost = strings.TrimSpace(profile.IMAPHost)
		if profile.IMAPHost == "" || strings.ContainsAny(profile.IMAPHost, "/:@ \r\n") || profile.IMAPPort < 1 || profile.IMAPPort > 65535 {
			return bad("valid IMAP host and port are required")
		}
		if strings.TrimSpace(profile.IMAPUsername) == "" {
			profile.IMAPUsername = profile.MailboxAddress
		}
	}
	if len(profile.CustomInput) > 16384 {
		return bad("custom input exceeds size limit")
	}
	if len(profile.CodePattern) > 500 || len(profile.Secret) > 8192 || len(profile.CodeScript) > 16384 {
		return bad("profile field exceeds size limit")
	}
	pattern, err := regexp.Compile(profile.CodePattern)
	if profile.MailboxMode == "imap" && (err != nil || pattern.NumSubexp() != 1) {
		return bad("verification code pattern must have exactly one capture group")
	}
	if len(profile.ProxyIDs) > 100 {
		return bad("at most 100 authorization proxies are allowed")
	}
	seenProxies := make(map[int64]bool)
	for _, proxyID := range profile.ProxyIDs {
		if proxyID < 1 || seenProxies[proxyID] {
			return bad("proxy IDs must be positive and unique")
		}
		seenProxies[proxyID] = true
	}
	if profile.ProxyID != nil {
		seenProxies[*profile.ProxyID] = true
	}
	for proxyID := range seenProxies {
		if s.proxyRepo == nil {
			return bad("proxy repository unavailable")
		}
		if _, err := s.proxyRepo.GetByID(ctx, proxyID); err != nil {
			return bad("proxy does not exist")
		}
	}
	stored := storedReauthProfile{}
	if id == "" {
		b := make([]byte, 16)
		if _, err := rand.Read(b); err != nil {
			return nil, err
		}
		id = hex.EncodeToString(b)
	} else {
		if !validReauthProfileID(id) {
			return bad("invalid profile ID")
		}
		value, err := s.settingRepo.GetValue(ctx, reauthProfilePrefix+id)
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(value), &stored); err != nil {
			return nil, err
		}
	}
	if profile.MailboxMode == "api" && strings.TrimSpace(profile.CodeScript) == "" && stored.EncryptedScript == "" {
		return bad("custom code is required for this read method")
	}
	if profile.Secret != "" || profile.CodeScript != "" {
		if s.cfg == nil || !s.cfg.Totp.EncryptionKeyConfigured {
			return bad("Set a fixed TOTP_ENCRYPTION_KEY before saving mailbox credentials")
		}
		key, err := hex.DecodeString(s.cfg.Totp.EncryptionKey)
		if err != nil || len(key) != 32 {
			return bad("invalid secret encryption key")
		}
		block, err := aes.NewCipher(key)
		if err != nil {
			return nil, err
		}
		gcm, err := cipher.NewGCM(block)
		if err != nil {
			return nil, err
		}
		seal := func(value, purpose string) (string, error) {
			nonce := make([]byte, gcm.NonceSize())
			if _, err := rand.Read(nonce); err != nil {
				return "", err
			}
			return base64.StdEncoding.EncodeToString(gcm.Seal(nonce, nonce, []byte(value), []byte(reauthProfilePrefix+id+purpose))), nil
		}
		if profile.Secret != "" {
			stored.EncryptedSecret, err = seal(profile.Secret, "")
			if err != nil {
				return nil, err
			}
		}
		if profile.CodeScript != "" {
			stored.EncryptedScript, err = seal(profile.CodeScript, ".script")
			if err != nil {
				return nil, err
			}
		}
	}
	if profile.MailboxMode == "api" {
		// Custom scripts receive their own input scope; mailbox credentials are not used.
		stored.EncryptedSecret = ""
		profile.MailboxAddress = ""
		profile.IMAPUsername = ""
	}
	profile.ID, profile.Secret = id, ""
	profile.CodeScript = ""
	profile.HasCodeScript = stored.EncryptedScript != ""
	profile.HasSecret = stored.EncryptedSecret != ""
	stored.ReauthProfile = profile
	encoded, err := json.Marshal(stored)
	if err != nil {
		return nil, err
	}
	if err := s.settingRepo.Set(ctx, reauthProfilePrefix+id, string(encoded)); err != nil {
		return nil, err
	}
	return &profile, nil
}

// ReadReauthScript is an explicit administrator-only sensitive read for a
// server runner execution. Ordinary list responses never return secrets/code.
func (s *SettingService) ReadReauthScript(ctx context.Context, id string) (map[string]string, error) {
	return s.readReauthMaterial(ctx, id, true)
}

func (s *SettingService) ReadReauthMaterial(ctx context.Context, id string) (map[string]string, error) {
	return s.readReauthMaterial(ctx, id, false)
}

// ReadReauthSecret permits administrators to inspect saved credentials even when
// the profile is disabled, without returning the script or enabling execution.
func (s *SettingService) ReadReauthSecret(ctx context.Context, id string) (string, error) {
	if !validReauthProfileID(id) {
		return "", infraerrors.BadRequest("INVALID_AUTH_PROFILE", "invalid profile ID")
	}
	value, err := s.settingRepo.GetValue(ctx, reauthProfilePrefix+id)
	if err != nil {
		return "", err
	}
	var stored storedReauthProfile
	if err := json.Unmarshal([]byte(value), &stored); err != nil {
		return "", err
	}
	return s.decryptReauthValue(id, stored.EncryptedSecret, "")
}

func (s *SettingService) readReauthMaterial(ctx context.Context, id string, scriptOnly bool) (map[string]string, error) {
	if !validReauthProfileID(id) {
		return nil, infraerrors.BadRequest("INVALID_AUTH_PROFILE", "invalid profile ID")
	}
	value, err := s.settingRepo.GetValue(ctx, reauthProfilePrefix+id)
	if err != nil {
		return nil, err
	}
	var stored storedReauthProfile
	if err := json.Unmarshal([]byte(value), &stored); err != nil {
		return nil, err
	}
	if !stored.Enabled || (scriptOnly && stored.MailboxMode != "api") || (stored.MailboxMode == "api" && stored.EncryptedScript == "") || (stored.MailboxMode == "imap" && stored.EncryptedSecret == "") {
		return nil, infraerrors.BadRequest("AUTH_SCRIPT_UNAVAILABLE", "enabled API profile with code is required")
	}
	script, err := s.decryptReauthValue(id, stored.EncryptedScript, ".script")
	if err != nil {
		return nil, err
	}
	secret, err := s.decryptReauthValue(id, stored.EncryptedSecret, "")
	if err != nil {
		return nil, err
	}
	return map[string]string{"code_script": script, "secret": secret, "api_url": stored.APIURL, "mailbox_address": stored.MailboxAddress, "custom_input": stored.CustomInput}, nil
}

func (s *SettingService) decryptReauthValue(id, encoded, purpose string) (string, error) {
	if encoded == "" {
		return "", nil
	}
	if s.cfg == nil || !s.cfg.Totp.EncryptionKeyConfigured {
		return "", infraerrors.BadRequest("AUTH_SCRIPT_UNAVAILABLE", "fixed secret encryption key required")
	}
	key, err := hex.DecodeString(s.cfg.Totp.EncryptionKey)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(data) < gcm.NonceSize() {
		return "", fmt.Errorf("invalid encrypted authorization material")
	}
	plain, err := gcm.Open(nil, data[:gcm.NonceSize()], data[gcm.NonceSize():], []byte(reauthProfilePrefix+id+purpose))
	return string(plain), err
}

func (s *SettingService) DeleteReauthProfile(ctx context.Context, id string) error {
	if !validReauthProfileID(id) {
		return infraerrors.BadRequest("INVALID_AUTH_PROFILE", "invalid profile ID")
	}
	_, err := s.settingRepo.GetValue(ctx, reauthProfilePrefix+id)
	if err != nil && !errors.Is(err, ErrSettingNotFound) {
		return err
	}
	return s.settingRepo.Delete(ctx, reauthProfilePrefix+id)
}
