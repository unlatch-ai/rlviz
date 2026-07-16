package daemon

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func testMetadata(t *testing.T, address string) Metadata {
	t.Helper()
	token, err := GenerateToken()
	if err != nil {
		t.Fatal(err)
	}
	return Metadata{PID: 1234, Address: address, Token: token, Version: "test"}
}

func TestWriteReadMetadataPermissions(t *testing.T) {
	paths := PathsAt(filepath.Join(t.TempDir(), "runtime"))
	want := testMetadata(t, "127.0.0.1:7317")
	if err := WriteMetadata(paths, want); err != nil {
		t.Fatal(err)
	}
	got, err := ReadMetadata(paths)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("metadata = %#v, want %#v", got, want)
	}
	if runtime.GOOS != "windows" {
		directory, err := os.Stat(paths.RuntimeDir)
		if err != nil {
			t.Fatal(err)
		}
		if got := directory.Mode().Perm(); got != 0o700 {
			t.Fatalf("runtime mode = %o, want 700", got)
		}
		file, err := os.Stat(paths.MetadataFile)
		if err != nil {
			t.Fatal(err)
		}
		if got := file.Mode().Perm(); got != 0o600 {
			t.Fatalf("metadata mode = %o, want 600", got)
		}
	}
	matches, err := filepath.Glob(filepath.Join(paths.RuntimeDir, ".daemon-*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("temporary metadata files remain: %v", matches)
	}
}

func TestReadMetadataRejectsBroadPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("mode bits do not represent Windows ACLs")
	}
	paths := PathsAt(filepath.Join(t.TempDir(), "runtime"))
	if err := WriteMetadata(paths, testMetadata(t, "127.0.0.1:7317")); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(paths.MetadataFile, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadMetadata(paths); !errors.Is(err, ErrInsecureMetadata) {
		t.Fatalf("ReadMetadata error = %v, want ErrInsecureMetadata", err)
	}
}

func TestEnsureRuntimeDirRejectsSymlink(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "runtime")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := PathsAt(link).EnsureRuntimeDir(); err == nil {
		t.Fatal("EnsureRuntimeDir accepted a symlink")
	}
}

func TestValidateLoopbackAddress(t *testing.T) {
	for _, address := range []string{"127.0.0.1:7317", "127.19.2.3:1", "[::1]:7317"} {
		if err := ValidateLoopbackAddress(address); err != nil {
			t.Errorf("ValidateLoopbackAddress(%q) = %v", address, err)
		}
	}
	for _, address := range []string{
		"localhost:7317",
		"0.0.0.0:7317",
		"192.168.1.2:7317",
		"8.8.8.8:443",
		"example.com:443",
		"127.0.0.1",
		"127.0.0.1:http",
		"127.0.0.1:0",
		"127.0.0.1:65536",
	} {
		if err := ValidateLoopbackAddress(address); err == nil {
			t.Errorf("ValidateLoopbackAddress(%q) accepted non-loopback or malformed address", address)
		}
	}
}

func TestGenerateTokenHasRequiredEntropy(t *testing.T) {
	first, err := GenerateToken()
	if err != nil {
		t.Fatal(err)
	}
	second, err := GenerateToken()
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("two generated tokens are equal")
	}
	metadata := Metadata{PID: 1, Address: "127.0.0.1:1", Token: first, Version: "test"}
	if err := metadata.Validate(); err != nil {
		t.Fatalf("generated token failed validation: %v", err)
	}
}
