package plugins

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// ContentDigest hashes every regular source file under the plugin root, not
// just the manifest and entrypoint: imported helpers are executable code too.
// VCS metadata is excluded. Python bytecode is included because interpreters
// may execute it; the host disables writing new bytecode during execution.
// Symlinked directories are rejected; symlinked files are accepted only when
// their targets remain inside root and are hashed under the symlink's name.
func ContentDigest(root, manifestPath string, command []string) (string, error) {
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	manifestPath, err = filepath.EvalSymlinks(manifestPath)
	if err != nil {
		return "", err
	}
	if !within(root, manifestPath) {
		return "", errorsPath("manifest", manifestPath, root)
	}
	files := map[string]string{}
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if rel != "." && entry.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Name() == ".DS_Store" {
			return nil
		}
		resolved := path
		if entry.Type()&os.ModeSymlink != 0 {
			resolved, err = filepath.EvalSymlinks(path)
			if err != nil {
				return err
			}
			if !within(root, resolved) {
				return fmt.Errorf("plugin symlink %s escapes plugin root", path)
			}
			info, err := os.Stat(resolved)
			if err != nil {
				return err
			}
			if info.IsDir() {
				return fmt.Errorf("symlinked plugin directories are not supported: %s", path)
			}
			if !info.Mode().IsRegular() {
				return fmt.Errorf("plugin symlink target is not a regular file: %s", path)
			}
		} else if !entry.Type().IsRegular() {
			return fmt.Errorf("plugin contains unsupported non-regular file: %s", path)
		}
		files[filepath.ToSlash(rel)] = resolved
		return nil
	})
	if err != nil {
		return "", err
	}
	if len(command) == 0 {
		return "", fmt.Errorf("plugin command is empty")
	}
	for index, argument := range command {
		candidate := argument
		if index == 0 && !strings.ContainsRune(candidate, filepath.Separator) {
			candidate, err = exec.LookPath(candidate)
			if err != nil {
				return "", fmt.Errorf("locate plugin executable %q: %w", argument, err)
			}
		} else if !filepath.IsAbs(candidate) {
			candidate = filepath.Join(root, candidate)
		}
		info, statErr := os.Stat(candidate)
		if statErr != nil || !info.Mode().IsRegular() {
			continue
		}
		resolved, resolveErr := filepath.EvalSymlinks(candidate)
		if resolveErr != nil {
			return "", resolveErr
		}
		if within(root, resolved) {
			continue
		}
		files[fmt.Sprintf("@command/%d/%s", index, filepath.ToSlash(resolved))] = resolved
	}
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	h := sha256.New()
	for _, name := range names {
		fmt.Fprintf(h, "%d:%s:", len(name), name)
		file, err := os.Open(files[name])
		if err != nil {
			return "", err
		}
		_, copyErr := io.Copy(h, file)
		closeErr := file.Close()
		if copyErr != nil {
			return "", copyErr
		}
		if closeErr != nil {
			return "", closeErr
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func errorsPath(kind, path, root string) error {
	return fmt.Errorf("%s %s escapes plugin root %s", kind, path, root)
}

func within(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
