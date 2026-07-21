package server

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const MaxArtifactContentBytes int64 = 16 << 20

var safeArtifactTypes = map[string]string{
	"text/plain":           "text/plain; charset=utf-8",
	"text/x-log":           "text/plain; charset=utf-8",
	"text/x-diff":          "text/plain; charset=utf-8",
	"text/x-patch":         "text/plain; charset=utf-8",
	"application/json":     "application/json; charset=utf-8",
	"application/x-ndjson": "application/x-ndjson; charset=utf-8",
	"image/png":            "image/png",
	"image/jpeg":           "image/jpeg",
	"image/gif":            "image/gif",
	"image/webp":           "image/webp",
}

func (api *indexedAPI) artifactContent(response http.ResponseWriter, request *http.Request) {
	params, ok := api.trajectoryParams(response, request, map[string]bool{"trajectory": true, "trajectory_id": true, "artifact_id": true})
	if !ok {
		return
	}
	artifactID, err := requiredSingle(request.URL.Query(), "artifact_id")
	if err != nil {
		writeJSONError(response, http.StatusBadRequest, "invalid_query", err)
		return
	}
	item, err := api.reader.Artifact(request.Context(), params.sourceID, params.trajectoryID, artifactID)
	if err != nil {
		api.writeReadError(response, "artifact_not_found", err)
		return
	}
	artifact := item.Value
	if artifact == nil {
		writeJSONError(response, http.StatusNotFound, "artifact_not_found", fmt.Errorf("artifact %q was not found", artifactID))
		return
	}
	if artifact.Path == "" {
		writeJSONError(response, http.StatusBadRequest, "artifact_not_path_backed", errors.New("artifact content is already inline"))
		return
	}
	contentType, err := safeArtifactContentType(artifact.MediaType)
	if err != nil {
		writeJSONError(response, http.StatusUnsupportedMediaType, "artifact_type_unsupported", err)
		return
	}
	path, err := resolveArtifactPath(params.source.Path, artifact.Path)
	if err != nil {
		writeJSONError(response, http.StatusForbidden, "artifact_path_forbidden", err)
		return
	}
	expectedInfo, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeJSONError(response, http.StatusNotFound, "artifact_content_not_found", err)
		} else {
			writeJSONError(response, http.StatusForbidden, "artifact_read_forbidden", err)
		}
		return
	}
	file, info, err := openVerifiedArtifact(path, expectedInfo)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeJSONError(response, http.StatusNotFound, "artifact_content_not_found", err)
		} else {
			writeJSONError(response, http.StatusForbidden, "artifact_read_forbidden", err)
		}
		return
	}
	defer file.Close()
	if !info.Mode().IsRegular() {
		err = errors.New("artifact must be a regular file")
		writeJSONError(response, http.StatusForbidden, "artifact_read_forbidden", err)
		return
	}
	if info.Size() > MaxArtifactContentBytes {
		writeJSONError(response, http.StatusRequestEntityTooLarge, "artifact_too_large", fmt.Errorf("artifact is %d bytes; maximum is %d", info.Size(), MaxArtifactContentBytes))
		return
	}
	content, err := io.ReadAll(io.LimitReader(file, MaxArtifactContentBytes+1))
	if err != nil {
		writeJSONError(response, http.StatusInternalServerError, "artifact_read_failed", err)
		return
	}
	if int64(len(content)) > MaxArtifactContentBytes {
		writeJSONError(response, http.StatusRequestEntityTooLarge, "artifact_too_large", fmt.Errorf("artifact exceeds maximum of %d bytes", MaxArtifactContentBytes))
		return
	}
	if err := verifyArtifactHash(content, artifact.SHA256); err != nil {
		writeJSONError(response, http.StatusUnprocessableEntity, "artifact_hash_mismatch", err)
		return
	}
	filename := artifact.Name
	if filename == "" {
		filename = filepath.Base(path)
	}
	response.Header().Set("Content-Type", contentType)
	response.Header().Set("Content-Length", fmt.Sprint(len(content)))
	response.Header().Set("Content-Disposition", mime.FormatMediaType("inline", map[string]string{"filename": filename}))
	response.WriteHeader(http.StatusOK)
	_, _ = response.Write(content)
}

func openVerifiedArtifact(path string, expected os.FileInfo) (*os.File, os.FileInfo, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	actual, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, nil, err
	}
	if expected == nil || !os.SameFile(expected, actual) {
		_ = file.Close()
		return nil, nil, errors.New("artifact changed while opening")
	}
	return file, actual, nil
}

func safeArtifactContentType(value string) (string, error) {
	base, _, err := mime.ParseMediaType(value)
	if err != nil {
		return "", fmt.Errorf("invalid artifact media type: %w", err)
	}
	if contentType, ok := safeArtifactTypes[strings.ToLower(base)]; ok {
		return contentType, nil
	}
	return "", fmt.Errorf("artifact media type %q is not safe for inline display", base)
}

func resolveArtifactPath(sourcePath, artifactPath string) (string, error) {
	if sourcePath == "" {
		return "", errors.New("indexed source path is unavailable")
	}
	source, err := filepath.Abs(sourcePath)
	if err != nil {
		return "", err
	}
	source, err = filepath.EvalSymlinks(source)
	if err != nil {
		return "", fmt.Errorf("resolve source root: %w", err)
	}
	info, err := os.Stat(source)
	if err != nil {
		return "", fmt.Errorf("inspect source root: %w", err)
	}
	root := source
	if !info.IsDir() {
		root = filepath.Dir(source)
	}
	candidate := artifactPath
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(root, candidate)
	}
	candidate, err = filepath.Abs(candidate)
	if err != nil {
		return "", err
	}
	candidate, err = filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve artifact: %w", err)
	}
	relative, err := filepath.Rel(root, candidate)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return "", fmt.Errorf("artifact path escapes source root")
	}
	return candidate, nil
}

func verifyArtifactHash(content []byte, expected string) error {
	if expected == "" {
		return nil
	}
	if len(expected) != 64 || strings.Trim(expected, "0123456789abcdef") != "" {
		return errors.New("artifact sha256 is invalid")
	}
	digest := sha256.Sum256(content)
	actual := hex.EncodeToString(digest[:])
	if actual != expected {
		return fmt.Errorf("artifact sha256 mismatch: expected %s, got %s", expected, actual)
	}
	return nil
}
