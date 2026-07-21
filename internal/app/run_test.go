package app

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
)

type testListener struct{ address net.Addr }

func (listener testListener) Accept() (net.Conn, error) { return nil, errors.New("test listener") }
func (listener testListener) Close() error              { return nil }
func (listener testListener) Addr() net.Addr            { return listener.address }

func TestForegroundViewerValidatesPresentationBeforeSource(t *testing.T) {
	_, err := StartViewer(Viewer{
		SourcePath:   filepath.Join(t.TempDir(), "missing.ndjson"),
		Presentation: json.RawMessage(`{"api_version":"rlviz.dev/v1alpha1","script":"bad"}`),
	})
	if err == nil || !strings.Contains(err.Error(), "presentation") {
		t.Fatalf("error=%v", err)
	}
}

func TestShutdownDrainsRequestsBeforeCleanup(t *testing.T) {
	draining := make(chan struct{})
	release := make(chan struct{})
	cleaned := make(chan struct{})
	viewer := &RunningViewer{cleanup: func() { close(cleaned) }}
	shutdownDone := make(chan error, 1)
	go func() {
		shutdownDone <- viewer.finishShutdown(func() error {
			close(draining)
			<-release
			return nil
		})
	}()
	<-draining
	select {
	case <-cleaned:
		t.Fatal("cleanup ran before the active request drained")
	default:
	}
	close(release)
	if err := <-shutdownDone; err != nil {
		t.Fatal(err)
	}
	<-cleaned
}

func TestFailedShutdownDoesNotCleanupUndrainedState(t *testing.T) {
	cleaned := false
	viewer := &RunningViewer{cleanup: func() { cleaned = true }}
	want := errors.New("shutdown deadline")
	if err := viewer.finishShutdown(func() error { return want }); !errors.Is(err, want) {
		t.Fatalf("shutdown error = %v", err)
	}
	if cleaned {
		t.Fatal("cleanup ran after shutdown failed before draining")
	}
}

func TestForegroundViewerRequiresFragmentToken(t *testing.T) {
	listener := testListener{address: &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 4173}}
	viewer, err := startViewer(Viewer{SourcePath: filepath.Join("..", "..", "fixtures", "canonical", "linear.ndjson")}, listener)
	if err != nil {
		t.Fatal(err)
	}
	defer viewer.runCleanup()

	viewerURL, err := url.Parse(viewer.URL)
	if err != nil {
		t.Fatal(err)
	}
	token := viewerURL.Fragment
	values, err := url.ParseQuery(token)
	if err != nil || values.Get("token") == "" {
		t.Fatalf("viewer URL has no token fragment: %s", viewer.URL)
	}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/indexed/browse", nil)
	response := httptest.NewRecorder()
	viewer.Server.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d", response.Code)
	}
	request = httptest.NewRequest(http.MethodGet, "/api/v1/indexed/browse", nil)
	request.Header.Set("Authorization", "Bearer "+values.Get("token"))
	response = httptest.NewRecorder()
	viewer.Server.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"count":1`) {
		t.Fatalf("authenticated browse = %d %s", response.Code, response.Body.String())
	}
}

func TestForegroundViewerRotatesTokenAcrossRestart(t *testing.T) {
	source := filepath.Join("..", "..", "fixtures", "canonical", "linear.ndjson")
	first, err := startViewer(Viewer{SourcePath: source}, testListener{address: &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 4173}})
	if err != nil {
		t.Fatal(err)
	}
	firstURL, err := url.Parse(first.URL)
	if err != nil {
		t.Fatal(err)
	}
	firstToken, err := url.ParseQuery(firstURL.Fragment)
	if err != nil {
		t.Fatal(err)
	}
	first.runCleanup()

	second, err := startViewer(Viewer{SourcePath: source}, testListener{address: &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 4173}})
	if err != nil {
		t.Fatal(err)
	}
	defer second.runCleanup()
	secondURL, err := url.Parse(second.URL)
	if err != nil {
		t.Fatal(err)
	}
	secondToken, err := url.ParseQuery(secondURL.Fragment)
	if err != nil {
		t.Fatal(err)
	}
	if firstToken.Get("token") == secondToken.Get("token") {
		t.Fatal("foreground restart reused its bearer token")
	}

	request := httptest.NewRequest(http.MethodGet, "/api/v1/indexed/browse", nil)
	request.Header.Set("Authorization", "Bearer "+firstToken.Get("token"))
	response := httptest.NewRecorder()
	second.Server.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("rotated daemon accepted stale token: status=%d body=%s", response.Code, response.Body.String())
	}
	request = httptest.NewRequest(http.MethodGet, "/api/v1/indexed/browse", nil)
	request.Header.Set("Authorization", "Bearer "+secondToken.Get("token"))
	response = httptest.NewRecorder()
	second.Server.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("rotated daemon rejected fresh token: status=%d body=%s", response.Code, response.Body.String())
	}
}
