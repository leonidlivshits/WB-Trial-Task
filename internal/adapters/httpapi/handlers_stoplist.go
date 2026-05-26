package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"wbtrialtask/internal/domain"
)

func (a *API) ListStopWords(w http.ResponseWriter, r *http.Request) {
	if !a.authorizeAdmin(w, r) {
		return
	}
	if !a.ensureStopListService(w) {
		return
	}

	words, version, err := a.stop.List(r.Context())
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, codeUnavailable, msgStopListUnavailable)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"version":       version,
		"words":         words,
		"updated_at_ms": a.now().UnixMilli(),
	})
}

func (a *API) AddStopWord(w http.ResponseWriter, r *http.Request) {
	if !a.authorizeAdmin(w, r) {
		return
	}
	if !a.ensureStopListService(w) {
		return
	}

	var req struct {
		Word string `json:"word"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, codeInvalidArgument, msgInvalidJSON)
		return
	}

	version, err := a.stop.Add(r.Context(), req.Word)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrInvalidArgument):
			writeError(w, http.StatusBadRequest, codeInvalidArgument, msgWordEmpty)
		case errors.Is(err, domain.ErrAlreadyExists):
			writeError(w, http.StatusConflict, codeConflict, msgWordAlreadyExist)
		default:
			writeError(w, http.StatusServiceUnavailable, codeUnavailable, msgStopListUnavailable)
		}
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"status":  "ok",
		"version": version,
	})
}

func (a *API) DeleteStopWord(w http.ResponseWriter, r *http.Request) {
	if !a.authorizeAdmin(w, r) {
		return
	}
	if !a.ensureStopListService(w) {
		return
	}

	word := strings.TrimSpace(r.PathValue("word"))
	version, err := a.stop.Remove(r.Context(), word)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrInvalidArgument), word == "":
			writeError(w, http.StatusBadRequest, codeInvalidArgument, msgWordEmpty)
		case errors.Is(err, domain.ErrNotFound):
			writeError(w, http.StatusNotFound, codeNotFound, msgWordNotFound)
		default:
			writeError(w, http.StatusServiceUnavailable, codeUnavailable, msgStopListUnavailable)
		}
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":  "ok",
		"version": version,
	})
}

func (a *API) authorizeAdmin(w http.ResponseWriter, r *http.Request) bool {
	token := strings.TrimSpace(r.Header.Get("X-Admin-Token"))
	if a.adminToken != "" && token == a.adminToken {
		return true
	}

	writeError(w, http.StatusUnauthorized, codeUnauthorized, msgInvalidAdminAuth)
	return false
}

func (a *API) ensureStopListService(w http.ResponseWriter) bool {
	if a.stop != nil {
		return true
	}
	writeError(w, http.StatusServiceUnavailable, codeUnavailable, msgStopListUnavailable)
	return false
}
