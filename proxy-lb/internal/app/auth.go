package app

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"

	"proxy-lb/internal/logging"
)

type authStore struct {
	path string
	mu   sync.RWMutex
	data authData
}

type authData struct {
	Users  map[string]authUser  `json:"users"`
	Tokens map[string]authToken `json:"tokens"`
}

type authUser struct {
	Username     string `json:"username"`
	PasswordHash string `json:"password_hash"`
	CreatedAt    string `json:"created_at"`
}

type authToken struct {
	Username  string `json:"username"`
	Label     string `json:"label,omitempty"`
	CreatedAt string `json:"created_at"`
	LastUsed  string `json:"last_used,omitempty"`
}

type authPrincipal struct {
	Username  string `json:"username"`
	TokenHash string `json:"-"`
	IsIssued  bool   `json:"is_issued"`
	IsStatic  bool   `json:"is_static"`
}

type authResponse struct {
	Username string `json:"username"`
	Token    string `json:"token"`
}

func newAuthStore(path string) (*authStore, error) {
	s := &authStore{path: path}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *authStore) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.data = authData{
		Users:  map[string]authUser{},
		Tokens: map[string]authToken{},
	}

	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			logging.Info("auth store not found, starting fresh: %s", s.path)
			return nil
		}
		return fmt.Errorf("read auth store: %w", err)
	}
	if len(data) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, &s.data); err != nil {
		return fmt.Errorf("parse auth store: %w", err)
	}
	if s.data.Users == nil {
		s.data.Users = map[string]authUser{}
	}
	if s.data.Tokens == nil {
		s.data.Tokens = map[string]authToken{}
	}
	logging.Info("auth store loaded: users=%d tokens=%d path=%s", len(s.data.Users), len(s.data.Tokens), s.path)
	return nil
}

func (s *authStore) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("create auth dir: %w", err)
	}
	data, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal auth store: %w", err)
	}
	if err := os.WriteFile(s.path, data, 0o600); err != nil {
		return fmt.Errorf("write auth store: %w", err)
	}
	return nil
}

func (s *authStore) register(username, password, label string) (*authResponse, error) {
	username = normalizeUsername(username)
	if err := validateCredentials(username, password); err != nil {
		return nil, err
	}
	logging.Info("register attempt username=%s label=%s", username, strings.TrimSpace(label))

	hashBytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.data.Users[username]; exists {
		logging.Warn("register rejected username=%s reason=already_exists", username)
		return nil, fmt.Errorf("username already exists")
	}

	s.data.Users[username] = authUser{
		Username:     username,
		PasswordHash: string(hashBytes),
		CreatedAt:    now,
	}
	token, tokenHash, err := generateBearerToken()
	if err != nil {
		return nil, err
	}
	s.data.Tokens[tokenHash] = authToken{
		Username:  username,
		Label:     strings.TrimSpace(label),
		CreatedAt: now,
		LastUsed:  now,
	}
	if err := s.saveLocked(); err != nil {
		return nil, err
	}
	logging.Info("register success username=%s", username)
	return &authResponse{Username: username, Token: token}, nil
}

func (s *authStore) login(username, password, label string) (*authResponse, error) {
	username = normalizeUsername(username)
	if err := validateCredentials(username, password); err != nil {
		return nil, err
	}
	logging.Info("login attempt username=%s label=%s", username, strings.TrimSpace(label))

	s.mu.RLock()
	user, exists := s.data.Users[username]
	s.mu.RUnlock()
	if !exists {
		logging.Warn("login rejected username=%s reason=user_not_found", username)
		return nil, fmt.Errorf("invalid username or password")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		logging.Warn("login rejected username=%s reason=bad_password", username)
		return nil, fmt.Errorf("invalid username or password")
	}

	token, tokenHash, err := generateBearerToken()
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC().Format(time.RFC3339)

	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.Tokens[tokenHash] = authToken{
		Username:  username,
		Label:     strings.TrimSpace(label),
		CreatedAt: now,
		LastUsed:  now,
	}
	if err := s.saveLocked(); err != nil {
		return nil, err
	}
	logging.Info("login success username=%s", username)
	return &authResponse{Username: username, Token: token}, nil
}

func (s *authStore) issueToken(username, label string) (*authResponse, error) {
	username = normalizeUsername(username)
	if username == "" {
		return nil, fmt.Errorf("username is required")
	}
	logging.Info("issue token attempt username=%s label=%s", username, strings.TrimSpace(label))

	s.mu.RLock()
	_, exists := s.data.Users[username]
	s.mu.RUnlock()
	if !exists {
		logging.Warn("issue token rejected username=%s reason=user_not_found", username)
		return nil, fmt.Errorf("user not found")
	}

	token, tokenHash, err := generateBearerToken()
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC().Format(time.RFC3339)

	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.Tokens[tokenHash] = authToken{
		Username:  username,
		Label:     strings.TrimSpace(label),
		CreatedAt: now,
		LastUsed:  now,
	}
	if err := s.saveLocked(); err != nil {
		return nil, err
	}
	logging.Info("issue token success username=%s", username)
	return &authResponse{Username: username, Token: token}, nil
}

func (s *authStore) authenticate(token string) (*authPrincipal, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, fmt.Errorf("missing token")
	}
	tokenHash := hashToken(token)

	s.mu.Lock()
	defer s.mu.Unlock()
	record, exists := s.data.Tokens[tokenHash]
	if !exists {
		logging.Warn("issued token auth rejected reason=not_found")
		return nil, fmt.Errorf("invalid token")
	}
	record.LastUsed = time.Now().UTC().Format(time.RFC3339)
	s.data.Tokens[tokenHash] = record
	if err := s.saveLocked(); err != nil {
		return nil, err
	}
	logging.Info("issued token auth success username=%s", record.Username)
	return &authPrincipal{
		Username:  record.Username,
		TokenHash: tokenHash,
		IsIssued:  true,
		IsStatic:  false,
	}, nil
}

func normalizeUsername(username string) string {
	return strings.ToLower(strings.TrimSpace(username))
}

func validateCredentials(username, password string) error {
	if username == "" {
		return fmt.Errorf("username is required")
	}
	if len(username) < 3 {
		return fmt.Errorf("username must be at least 3 characters")
	}
	if len(password) < 6 {
		return fmt.Errorf("password must be at least 6 characters")
	}
	return nil
}

func generateBearerToken() (raw string, tokenHash string, err error) {
	buf := make([]byte, 32)
	if _, err = rand.Read(buf); err != nil {
		return "", "", fmt.Errorf("generate token: %w", err)
	}
	raw = "lb_" + base64.RawURLEncoding.EncodeToString(buf)
	return raw, hashToken(raw), nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
