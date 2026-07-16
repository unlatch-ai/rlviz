package main

import (
	"reflect"
	"testing"

	"github.com/unlatch-ai/rolloutviz/internal/daemon"
)

func TestNormalizeViewerArgumentsAllowsFlagsAfterPath(t *testing.T) {
	got := normalizeViewerArguments([]string{"trace.ndjson", "--no-open", "--port", "7317", "--json"})
	want := []string{"--no-open", "--port", "7317", "--json", "trace.ndjson"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalizeViewerArguments() = %#v, want %#v", got, want)
	}
}

func TestResolveViewerURLStaysOnDaemonOrigin(t *testing.T) {
	metadata := daemon.Metadata{Address: "127.0.0.1:7317", Token: "secret"}
	got, err := resolveViewerURL(metadata, "/?trajectory=trace-1")
	if err != nil || got != "http://127.0.0.1:7317/?trajectory=trace-1#token=secret" {
		t.Fatalf("resolveViewerURL() = %q, %v", got, err)
	}
	if _, err := resolveViewerURL(metadata, "https://example.com/steal"); err == nil {
		t.Fatal("resolveViewerURL() accepted another origin")
	}
}

func TestSafePluginName(t *testing.T) {
	if got := safePluginName("Customer Trace V2"); got != "customer-trace-v2" {
		t.Fatalf("safePluginName() = %q", got)
	}
}

func TestNormalizeViewerArgumentsPreservesEqualsFlag(t *testing.T) {
	got := normalizeViewerArguments([]string{"trace.ndjson", "--port=7317"})
	want := []string{"--port=7317", "trace.ndjson"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalizeViewerArguments() = %#v, want %#v", got, want)
	}
}
