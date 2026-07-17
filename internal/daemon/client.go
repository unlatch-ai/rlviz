package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	StatusPath   = "/api/v1/daemon/status"
	StopPath     = "/api/v1/daemon/stop"
	RegisterPath = "/api/v1/sources"
	maxResponse  = 1 << 20
)

// Status describes a daemon that accepted an authenticated health request.
type Status struct {
	Status  string `json:"status"`
	PID     int    `json:"pid"`
	Version string `json:"version"`
}

// RegisterRequest asks the daemon to open a read-only trajectory source.
type RegisterRequest struct {
	Path         string          `json:"path"`
	Adapter      string          `json:"adapter,omitempty"`
	Presentation json.RawMessage `json:"presentation"`
}

// RegisterResponse describes the registered source and browser destination.
type RegisterResponse struct {
	SourceID string `json:"source_id,omitempty"`
	Path     string `json:"path"`
	URL      string `json:"url"`
}

// Client calls the private loopback daemon API. HTTP may be supplied by tests
// or embedders; redirects are rejected even when a custom client is supplied.
type Client struct {
	HTTP *http.Client
}

type APIError struct {
	StatusCode int
	Code       string
	Message    string
	Details    map[string]any
}

func (err *APIError) Error() string {
	if err.Code == "" {
		return err.Message
	}
	return fmt.Sprintf("%s: %s", err.Code, err.Message)
}

func (client Client) httpClient() *http.Client {
	configured := client.HTTP
	if configured == nil {
		configured = &http.Client{Timeout: 30 * time.Second}
	}
	copy := *configured
	copy.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return &copy
}

// Status performs an authenticated daemon status request.
func (client Client) Status(ctx context.Context, metadata Metadata) (Status, error) {
	var response Status
	if err := client.doJSON(ctx, metadata, http.MethodGet, StatusPath, nil, &response); err != nil {
		return Status{}, err
	}
	if response.Status == "" {
		return Status{}, fmt.Errorf("daemon status response is incomplete")
	}
	return response, nil
}

// Register asks the daemon to validate and expose a source to the viewer.
func (client Client) Register(ctx context.Context, metadata Metadata, request RegisterRequest) (RegisterResponse, error) {
	var response RegisterResponse
	if err := client.doJSON(ctx, metadata, http.MethodPost, RegisterPath, request, &response); err != nil {
		return RegisterResponse{}, err
	}
	if response.URL == "" {
		return RegisterResponse{}, fmt.Errorf("daemon register response has no viewer URL")
	}
	return response, nil
}

// Stop asks the daemon to shut down. A successful empty response is accepted.
func (client Client) Stop(ctx context.Context, metadata Metadata) error {
	return client.doJSON(ctx, metadata, http.MethodPost, StopPath, nil, nil)
}

func (client Client) doJSON(ctx context.Context, metadata Metadata, method, path string, input, output any) error {
	if err := metadata.Validate(); err != nil {
		return fmt.Errorf("validate daemon metadata: %w", err)
	}
	var body io.Reader
	if input != nil {
		payload, err := json.Marshal(input)
		if err != nil {
			return fmt.Errorf("encode daemon request: %w", err)
		}
		body = bytes.NewReader(payload)
	}
	request, err := http.NewRequestWithContext(ctx, method, "http://"+metadata.Address+path, body)
	if err != nil {
		return fmt.Errorf("create daemon request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+metadata.Token)
	request.Header.Set("Accept", "application/json")
	if input != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := client.httpClient().Do(request)
	if err != nil {
		return fmt.Errorf("call daemon: %w", err)
	}
	defer response.Body.Close()
	limited := io.LimitReader(response.Body, maxResponse+1)
	payload, err := io.ReadAll(limited)
	if err != nil {
		return fmt.Errorf("read daemon response: %w", err)
	}
	if len(payload) > maxResponse {
		return fmt.Errorf("daemon response exceeds %d bytes", maxResponse)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		values := make(map[string]any)
		if json.Unmarshal(payload, &values) == nil {
			code, _ := values["code"].(string)
			message, _ := values["error"].(string)
			if message == "" {
				message = response.Status
			}
			return &APIError{StatusCode: response.StatusCode, Code: code, Message: message, Details: values}
		}
		message := strings.TrimSpace(string(payload))
		if message == "" {
			message = response.Status
		}
		return &APIError{StatusCode: response.StatusCode, Message: message}
	}
	if output == nil || len(bytes.TrimSpace(payload)) == 0 {
		return nil
	}
	if err := json.Unmarshal(payload, output); err != nil {
		return fmt.Errorf("decode daemon response: %w", err)
	}
	return nil
}
