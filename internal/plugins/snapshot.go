package plugins

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// snapshotPlugin copies all digest-covered plugin code into a private
// directory and reloads it there. Execution never reads the mutable trusted
// source tree after this snapshot has been verified against its trusted digest.
func snapshotPlugin(plugin *Plugin) (*Plugin, func(), error) {
	destination, err := os.MkdirTemp("", "rolloutviz-plugin-*")
	if err != nil {
		return nil, func() {}, err
	}
	cleanup := func() { _ = os.RemoveAll(destination) }
	err = filepath.WalkDir(plugin.Path, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(plugin.Path, path)
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if relative != "." && entry.Name() == ".git" {
				return filepath.SkipDir
			}
			if relative == "." {
				return nil
			}
			return os.Mkdir(filepath.Join(destination, relative), 0o700)
		}
		if entry.Name() == ".DS_Store" {
			return nil
		}
		source := path
		if entry.Type()&os.ModeSymlink != 0 {
			source, err = filepath.EvalSymlinks(path)
			if err != nil {
				return err
			}
		}
		info, err := os.Stat(source)
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("plugin contains unsupported non-regular file: %s", path)
		}
		return copySnapshotFile(source, filepath.Join(destination, relative), info.Mode().Perm())
	})
	if err != nil {
		cleanup()
		return nil, func() {}, err
	}
	snapshot, err := Load(destination)
	if err != nil {
		cleanup()
		return nil, func() {}, err
	}
	return snapshot, cleanup, nil
}

func copySnapshotFile(source, destination string, mode fs.FileMode) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, input)
	closeErr := output.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}
