package daemon

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClientAuthenticatedLifecycleRequests(t *testing.T) {
	tokenMetadata := testMetadata(t, "127.0.0.1:1")
	requests := make(chan string, 3)
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if got, want := request.Header.Get("Authorization"), "Bearer "+tokenMetadata.Token; got != want {
			t.Errorf("Authorization = %q, want %q", got, want)
			http.Error(response, "unauthorized", http.StatusUnauthorized)
			return
		}
		requests <- request.Method + " " + request.URL.Path
		response.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case StatusPath:
			_ = json.NewEncoder(response).Encode(Status{Status: "running", PID: tokenMetadata.PID, Version: tokenMetadata.Version})
		case RegisterPath:
			var input RegisterRequest
			if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
				t.Errorf("decode register request: %v", err)
			}
			if input.Path != "/tmp/trace.jsonl" || input.Adapter != "custom" {
				t.Errorf("register input = %#v", input)
			}
			_ = json.NewEncoder(response).Encode(RegisterResponse{SourceID: "source-1", Path: input.Path, URL: "/sources/source-1"})
		case StopPath:
			response.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()
	tokenMetadata.Address = strings.TrimPrefix(server.URL, "http://")
	client := Client{HTTP: server.Client()}

	status, err := client.Status(context.Background(), tokenMetadata)
	if err != nil {
		t.Fatal(err)
	}
	if status.Status != "running" {
		t.Fatalf("status = %#v", status)
	}
	registered, err := client.Register(context.Background(), tokenMetadata, RegisterRequest{Path: "/tmp/trace.jsonl", Adapter: "custom"})
	if err != nil {
		t.Fatal(err)
	}
	if registered.URL != "/sources/source-1" {
		t.Fatalf("registered = %#v", registered)
	}
	if err := client.Stop(context.Background(), tokenMetadata); err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{
		"GET " + StatusPath,
		"POST " + RegisterPath,
		"POST " + StopPath,
	} {
		if got := <-requests; got != want {
			t.Fatalf("request = %q, want %q", got, want)
		}
	}
}

func TestClientRejectsRedirectWithoutLeakingToken(t *testing.T) {
	tokenMetadata := testMetadata(t, "127.0.0.1:1")
	receivedAtTarget := false
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		receivedAtTarget = true
	}))
	defer target.Close()
	redirector := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		http.Redirect(response, request, target.URL, http.StatusTemporaryRedirect)
	}))
	defer redirector.Close()
	tokenMetadata.Address = strings.TrimPrefix(redirector.URL, "http://")
	if _, err := (Client{HTTP: redirector.Client()}).Status(context.Background(), tokenMetadata); err == nil {
		t.Fatal("Status accepted redirect")
	}
	if receivedAtTarget {
		t.Fatal("redirect target received daemon request")
	}
}
