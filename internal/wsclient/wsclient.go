// Package wsclient manages the node registration WebSocket connection to
// Mellowtel. Registration data is carried as query parameters on the connection
// URL (there is no JSON handshake). The client sends protocol ping frames as a
// heartbeat and reconnects with exponential backoff on failure.
package wsclient

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"runtime"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"
)

// State is the connection lifecycle state, surfaced to the UI.
type State string

const (
	StateDisconnected State = "disconnected"
	StateConnecting   State = "connecting"
	StateConnected    State = "connected"
)

const (
	pingPeriod = 60 * time.Second
	pongWait   = 70 * time.Second
	writeWait  = 10 * time.Second
)

// Config configures a Client.
type Config struct {
	Log            zerolog.Logger
	BaseURL        string // wss://ws.mellow.tel
	DeviceID       string
	DeviceToken    string // short-lived proof issued by earnbear.app
	Version        string
	PlatformPrefix string // e.g. "desktop"; OS suffix appended automatically
	SpeedDownload  int    // Mbps; <=0 omits the parameter

	// OnMessage is invoked for every inbound text frame (raw bytes).
	OnMessage func(data []byte)
	// OnState is invoked whenever the connection state changes.
	OnState func(State)
	// OnDisconnectCommand is invoked when the server asks the node to stop.
	OnDisconnectCommand func()
}

// Client is a reconnecting WebSocket client.
type Client struct {
	cfg Config
	log zerolog.Logger
}

// New builds a Client.
func New(cfg Config) *Client {
	return &Client{cfg: cfg, log: cfg.Log.With().Str("component", "ws").Logger()}
}

// osLabel maps GOOS to the label the platform parameter expects.
func osLabel() string {
	switch runtime.GOOS {
	case "darwin":
		return "macos"
	case "windows":
		return "windows"
	default:
		return "linux"
	}
}

// connURL assembles the full connection URL with registration query params.
func (c *Client) connURL() (string, error) {
	u, err := url.Parse(c.cfg.BaseURL)
	if err != nil {
		return "", fmt.Errorf("parse ws base url: %w", err)
	}
	q := u.Query()
	q.Set("device_id", c.cfg.DeviceID)
	q.Set("version", c.cfg.Version)
	q.Set("platform", fmt.Sprintf("%s-%s", c.cfg.PlatformPrefix, osLabel()))
	if c.cfg.SpeedDownload > 0 {
		q.Set("speed_download", fmt.Sprintf("%d", c.cfg.SpeedDownload))
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func (c *Client) setState(s State) {
	if c.cfg.OnState != nil {
		c.cfg.OnState(s)
	}
}

// Run connects and services the socket, reconnecting until ctx is cancelled.
// It blocks; callers should run it in a goroutine.
func (c *Client) Run(ctx context.Context) {
	backoff := newBackoff()
	for {
		if ctx.Err() != nil {
			c.setState(StateDisconnected)
			return
		}

		c.setState(StateConnecting)
		err := c.connectAndServe(ctx)
		if ctx.Err() != nil {
			c.setState(StateDisconnected)
			return
		}
		if err != nil {
			c.log.Warn().Err(err).Msg("connection ended; will retry")
		}
		c.setState(StateDisconnected)

		delay := backoff.next()
		c.log.Info().Dur("delay", delay).Msg("reconnecting after backoff")
		select {
		case <-ctx.Done():
			c.setState(StateDisconnected)
			return
		case <-time.After(delay):
		}
	}
}

// connectAndServe performs one connection lifecycle. It returns when the socket
// closes or errors.
func (c *Client) connectAndServe(ctx context.Context) error {
	target, err := c.connURL()
	if err != nil {
		return err
	}

	dialCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	c.log.Info().Str("url", redact(target)).Msg("dialing registration socket")
	headers := make(http.Header)
	if c.cfg.DeviceToken != "" {
		headers.Set("X-Earnbear-Device-Token", c.cfg.DeviceToken)
	}
	conn, _, err := websocket.DefaultDialer.DialContext(dialCtx, target, headers)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	defer conn.Close()

	c.setState(StateConnected)
	c.log.Info().Msg("registration socket connected")

	// Heartbeat plumbing.
	conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	// Serve context: cancelling stops the pinger when the reader exits.
	serveCtx, serveCancel := context.WithCancel(ctx)
	defer serveCancel()
	go c.pinger(serveCtx, conn)

	// ReadMessage below blocks and does not observe context cancellation, so a
	// shutdown would otherwise stall until the read deadline (up to pongWait).
	// Closing the connection is what actually unblocks the reader.
	go func() {
		<-serveCtx.Done()
		conn.Close()
	}()

	for {
		mtype, data, err := conn.ReadMessage()
		if err != nil {
			return fmt.Errorf("read: %w", err)
		}
		if mtype != websocket.TextMessage {
			continue
		}
		c.handleMessage(data)
	}
}

// pinger sends periodic ping frames; a missing pong trips the read deadline.
func (c *Client) pinger(ctx context.Context, conn *websocket.Conn) {
	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(writeWait)); err != nil {
				c.log.Debug().Err(err).Msg("ping write failed")
				return
			}
		}
	}
}

// handleMessage dispatches an inbound text frame.
func (c *Client) handleMessage(data []byte) {
	// Peek at type_event to short-circuit control frames without a full parse.
	ev := peekTypeEvent(data)
	switch ev {
	case "heartbeat":
		c.log.Debug().Msg("heartbeat received")
		return
	case "disconnect_device":
		c.log.Info().Msg("server requested device disconnect")
		if c.cfg.OnDisconnectCommand != nil {
			c.cfg.OnDisconnectCommand()
		}
		return
	}
	if c.cfg.OnMessage != nil {
		c.cfg.OnMessage(data)
	}
}
