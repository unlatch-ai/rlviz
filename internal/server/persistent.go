package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/TheSnakeFang/rlviz/internal/presentation"
)

type PersistentRegistrar func(context.Context, string, string, json.RawMessage) (Registration, error)

// NewPersistentHandler serves daemon lifecycle, source registration, indexed
// reads, and the embedded viewer over one authenticated loopback origin.
func NewPersistentHandler(reader IndexedReader, token string, registrar PersistentRegistrar, stop func()) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/api/v1/indexed/", NewIndexedHandler(reader, token))
	mux.HandleFunc("GET /api/v1/health", func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(response, `{"status":"ok"}`+"\n")
	})
	mux.HandleFunc("GET /api/v1/daemon/status", func(response http.ResponseWriter, request *http.Request) {
		if !authorized(request, token) {
			writeJSONError(response, http.StatusUnauthorized, "unauthorized", errors.New("valid daemon token required"))
			return
		}
		writeJSON(response, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("POST /api/v1/sources", func(response http.ResponseWriter, request *http.Request) {
		if !authorized(request, token) {
			writeJSONError(response, http.StatusUnauthorized, "unauthorized", errors.New("valid daemon token required"))
			return
		}
		if registrar == nil {
			writeJSONError(response, http.StatusNotImplemented, "registration_unavailable", errors.New("source registration is unavailable"))
			return
		}
		request.Body = http.MaxBytesReader(response, request.Body, 1<<20)
		decoder := json.NewDecoder(request.Body)
		decoder.DisallowUnknownFields()
		var input struct {
			Path         string          `json:"path"`
			Adapter      string          `json:"adapter,omitempty"`
			Presentation json.RawMessage `json:"presentation"`
		}
		if err := decoder.Decode(&input); err != nil {
			writeJSONError(response, http.StatusBadRequest, "invalid_request", err)
			return
		}
		if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
			if err == nil {
				err = errors.New("request contains multiple JSON values")
			}
			writeJSONError(response, http.StatusBadRequest, "invalid_request", err)
			return
		}
		normalizedPresentation, err := presentation.NormalizeJSON(input.Presentation)
		if err != nil {
			writeJSONError(response, http.StatusBadRequest, "invalid_presentation", err)
			return
		}
		registration, err := registrar(request.Context(), input.Path, input.Adapter, normalizedPresentation)
		if err != nil {
			writeSourceError(response, err)
			return
		}
		writeJSON(response, http.StatusCreated, registration)
	})
	mux.HandleFunc("POST /api/v1/daemon/stop", func(response http.ResponseWriter, request *http.Request) {
		if !authorized(request, token) {
			writeJSONError(response, http.StatusUnauthorized, "unauthorized", errors.New("valid daemon token required"))
			return
		}
		if stop == nil {
			writeJSONError(response, http.StatusNotImplemented, "stop_unavailable", errors.New("daemon stop is unavailable"))
			return
		}
		writeJSON(response, http.StatusOK, map[string]string{"status": "stopping"})
		go stop()
	})
	mux.Handle("/", viewerHandler())
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("X-Content-Type-Options", "nosniff")
		response.Header().Set("Referrer-Policy", "no-referrer")
		response.Header().Set("Content-Security-Policy", "default-src 'self'; connect-src 'self'; img-src 'self' data: blob:; style-src 'self' 'unsafe-inline'; script-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'")
		mux.ServeHTTP(response, request)
	})
}
