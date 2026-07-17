package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestForegroundViewerValidatesPresentationBeforeSource(t *testing.T) {
	_, err := StartViewer(Viewer{
		SourcePath:   filepath.Join(t.TempDir(), "missing.ndjson"),
		Presentation: json.RawMessage(`{"api_version":"rlviz.dev/v1alpha1","script":"bad"}`),
	})
	if err == nil || !strings.Contains(err.Error(), "presentation") {
		t.Fatalf("error=%v", err)
	}
}

func TestForegroundViewerRequiresFragmentToken(t *testing.T) {
	viewer, err := StartViewer(Viewer{SourcePath: filepath.Join("..", "..", "fixtures", "canonical", "linear.ndjson")})
	if err != nil {
		t.Fatal(err)
	}
	serveDone := make(chan error, 1)
	go func() { serveDone <- viewer.Serve() }()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = viewer.Shutdown(ctx)
		<-serveDone
	}()

	viewerURL, err := url.Parse(viewer.URL)
	if err != nil {
		t.Fatal(err)
	}
	token := viewerURL.Fragment
	values, err := url.ParseQuery(token)
	if err != nil || values.Get("token") == "" {
		t.Fatalf("viewer URL has no token fragment: %s", viewer.URL)
	}
	endpoint := "http://" + viewerURL.Host + "/api/v1/trajectory?" + viewerURL.RawQuery
	response, err := http.Get(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d", response.StatusCode)
	}
	request, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+values.Get("token"))
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("authenticated status = %d", response.StatusCode)
	}
}
