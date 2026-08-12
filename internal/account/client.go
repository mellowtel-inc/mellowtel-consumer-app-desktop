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

// HTTPError preserves the response status so background reporters can stop
// retrying revoked or unauthenticated credentials while retaining data for a
// later successful registration.
type HTTPError struct {
	StatusCode int
	Message    string
}

func (e *HTTPError) Error() string { return e.Message }

// DeviceRegistration is the public metadata needed to bind this installation
// to the signed-in Earnbear account. The server derives the user identity from
// the HttpOnly Cognito session; the desktop never sends or stores AWS secrets.
type DeviceRegistration struct {
	DeviceID        string `json:"deviceId"`
	AppVersion      string `json:"appVersion"`
	ProtocolVersion string `json:"protocolVersion"`
	Platform        string `json:"platform"`
	Integration     string `json:"integration"`
}

// DeviceRegistrationResult contains the expiring proof accepted by the
// Mellowtel node gateway. It is kept in memory and refreshed before sharing.
type DeviceRegistrationResult struct {
	DeviceToken string `json:"deviceToken"`
	LinkedAt    string `json:"linkedAt"`
}

// ClientActivity is one provisional completion record inside a durable batch.
type ClientActivity struct {
	ActivityID string `json:"activityId"`
	BytesUsed  int64  `json:"bytesUsed"`
	OccurredAt string `json:"occurredAt"`
}

// ClientActivityBatch amortizes authentication and ingestion work across up
// to 1,000 provisional completion records without uploading per-job details.
type ClientActivityBatch struct {
	DeviceID       string `json:"deviceId"`
	DeviceToken    string `json:"deviceToken"`
	BatchID        string `json:"batchId"`
	ActivityCount  int    `json:"activityCount"`
	ActivityBytes  int64  `json:"activityBytes"`
	FirstOccurred  string `json:"firstOccurredAt"`
	LastOccurred   string `json:"lastOccurredAt"`
	ActivityDigest string `json:"activityDigest"`
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

func (c *Client) SignUp(email, password string) (SignUpResult, error) {
	var response struct {
		Success   bool   `json:"success"`
		Confirmed bool   `json:"confirmed"`
		Message   string `json:"message"`
	}
	if err := c.post("/api/auth/signup", map[string]string{"email": email, "password": password}, &response); err != nil {
		return SignUpResult{}, err
	}
	return SignUpResult{Confirmed: response.Confirmed}, nil
}

// RegisterDevice links a stable desktop device ID to the authenticated user.
// This request goes through earnbear.app so credentials and AWS signing remain
// on trusted server infrastructure.
func (c *Client) RegisterDevice(registration DeviceRegistration) (DeviceRegistrationResult, error) {
	var response struct {
		Success     bool   `json:"success"`
		DeviceToken string `json:"deviceToken"`
		LinkedAt    string `json:"linkedAt"`
	}
	if err := c.post("/api/devices/register", registration, &response); err != nil {
		return DeviceRegistrationResult{}, err
	}
	if strings.TrimSpace(response.DeviceToken) == "" {
		return DeviceRegistrationResult{}, errors.New("Earnbear did not return a device credential")
	}
	return DeviceRegistrationResult{DeviceToken: response.DeviceToken, LinkedAt: response.LinkedAt}, nil
}

// RecordClientActivityBatch sends deduplicated, non-monetary completion
// signals. AWS accepts the batch into a queue before processing it.
func (c *Client) RecordClientActivityBatch(batch ClientActivityBatch) error {
	return c.post("/api/rewards/activity", batch, nil)
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
		return &HTTPError{StatusCode: response.StatusCode, Message: envelope.Message}
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
