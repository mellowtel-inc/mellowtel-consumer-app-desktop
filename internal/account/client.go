// Package account connects the desktop app to Earnbear's first-party account
// API. Cognito credentials remain on the server; the desktop stores only the
// user's session cookies in its private application config directory.
package account

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const productionBaseURL = "https://earnbear.app"

type State struct {
	Authenticated bool   `json:"authenticated"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"emailVerified"`
	Name          string `json:"name"`
}

type SignUpResult struct {
	Confirmed bool `json:"confirmed"`
}

type storedSession struct {
	Email   string            `json:"email"`
	Cookies map[string]string `json:"cookies"`
}

type Client struct {
	baseURL string
	path    string
	http    *http.Client
	mu      sync.Mutex
	session storedSession
}

func New(configDir string) *Client {
	return NewWithBaseURL(configDir, productionBaseURL)
}

func NewWithBaseURL(configDir, baseURL string) *Client {
	c := &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		path:    filepath.Join(configDir, "account-session.json"),
		http:    &http.Client{Timeout: 15 * time.Second},
		session: storedSession{Cookies: map[string]string{}},
	}
	c.load()
	return c
}

func (c *Client) SignUp(email, password, affiliateCode string) (SignUpResult, error) {
	var response struct {
		Success   bool   `json:"success"`
		Confirmed bool   `json:"confirmed"`
		Message   string `json:"message"`
	}
	if err := c.post("/api/auth/signup", map[string]string{
		"email": email, "password": password, "affiliateCode": affiliateCode,
	}, &response); err != nil {
		return SignUpResult{}, err
	}
	return SignUpResult{Confirmed: response.Confirmed}, nil
}

func (c *Client) ConfirmSignUp(email, code string) error {
	return c.post("/api/auth/confirm", map[string]string{"email": email, "code": code}, nil)
}

func (c *Client) ResendSignUpCode(email string) error {
	return c.post("/api/auth/resend", map[string]string{"email": email}, nil)
}

func (c *Client) SignIn(email, password string) (State, error) {
	if err := c.post("/api/auth/login", map[string]string{"email": email, "password": password}, nil); err != nil {
		return State{}, err
	}
	c.mu.Lock()
	c.session.Email = strings.TrimSpace(strings.ToLower(email))
	err := c.saveLocked()
	c.mu.Unlock()
	if err != nil {
		return State{}, err
	}
	return c.GetState()
}

func (c *Client) GetState() (State, error) {
	request, err := http.NewRequest(http.MethodGet, c.baseURL+"/api/auth/session", nil)
	if err != nil {
		return State{}, err
	}
	c.applyCookies(request)

	response, err := c.http.Do(request)
	if err != nil {
		return State{}, fmt.Errorf("reach Earnbear: %w", err)
	}
	defer response.Body.Close()
	c.captureCookies(response)

	var payload struct {
		Configured bool `json:"configured"`
		User       *struct {
			Email             string `json:"email"`
			EmailVerified     bool   `json:"emailVerified"`
			Name              string `json:"name"`
			PreferredUsername string `json:"preferredUsername"`
		} `json:"user"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&payload); err != nil {
		return State{}, fmt.Errorf("read Earnbear session: %w", err)
	}
	if payload.User == nil {
		c.clear()
		return State{}, nil
	}
	name := payload.User.Name
	if name == "" {
		name = payload.User.PreferredUsername
	}
	return State{
		Authenticated: true,
		Email:         payload.User.Email,
		EmailVerified: payload.User.EmailVerified,
		Name:          name,
	}, nil
}

// HasSession checks locally whether the app has account credentials available.
// It keeps connect and auto-connect responsive; GetState performs validation.
func (c *Client) HasSession() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.session.Cookies["earnbear_access"] != "" || c.session.Cookies["earnbear_refresh"] != ""
}

func (c *Client) SignOut() error {
	return c.clear()
}

func (c *Client) post(path string, body any, target any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	request, err := http.NewRequest(http.MethodPost, c.baseURL+path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	c.applyCookies(request)

	response, err := c.http.Do(request)
	if err != nil {
		return fmt.Errorf("reach Earnbear: %w", err)
	}
	defer response.Body.Close()
	c.captureCookies(response)

	var envelope struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	content, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return err
	}
	if len(content) > 0 {
		_ = json.Unmarshal(content, &envelope)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 || !envelope.Success {
		if envelope.Message == "" {
			envelope.Message = "Earnbear could not complete that request."
		}
		return errors.New(envelope.Message)
	}
	if target != nil && len(content) > 0 {
		if err := json.Unmarshal(content, target); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) applyCookies(request *http.Request) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for name, value := range c.session.Cookies {
		request.AddCookie(&http.Cookie{Name: name, Value: value})
	}
}

func (c *Client) captureCookies(response *http.Response) {
	cookies := response.Cookies()
	if len(cookies) == 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, cookie := range cookies {
		if cookie.MaxAge < 0 || cookie.Value == "" {
			delete(c.session.Cookies, cookie.Name)
			continue
		}
		c.session.Cookies[cookie.Name] = cookie.Value
	}
	_ = c.saveLocked()
}

func (c *Client) clear() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.session = storedSession{Cookies: map[string]string{}}
	if err := os.Remove(c.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (c *Client) load() {
	data, err := os.ReadFile(c.path)
	if err != nil {
		return
	}
	var session storedSession
	if json.Unmarshal(data, &session) == nil && session.Cookies != nil {
		c.session = session
	}
}

func (c *Client) saveLocked() error {
	data, err := json.Marshal(c.session)
	if err != nil {
		return err
	}
	tmp := c.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, c.path); err != nil {
		return err
	}
	return os.Chmod(c.path, 0o600)
}
