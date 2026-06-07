package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
)

func (s *server) notConfigured(w http.ResponseWriter) bool {
	if s.adminScenarios == nil {
		jsonError(w, "scenario authoring not configured (DB_PATH unset)", http.StatusNotImplemented)
		return true
	}
	return false
}

func (s *server) adminListScenarios(w http.ResponseWriter, r *http.Request) {
	if s.notConfigured(w) {
		return
	}
	rows, err := s.adminScenarios.AdminList(r.Context())
	if err != nil {
		jsonError(w, "list scenarios: "+err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, rows)
}

func (s *server) adminCreateScenario(w http.ResponseWriter, r *http.Request) {
	if s.notConfigured(w) {
		return
	}
	var req struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ID == "" {
		jsonError(w, "id required", http.StatusBadRequest)
		return
	}
	if strings.ContainsAny(req.ID, "/ .") {
		jsonError(w, "id must not contain / . or spaces", http.StatusBadRequest)
		return
	}
	if err := s.adminScenarios.CreateScenario(r.Context(), req.ID); err != nil {
		jsonError(w, err.Error(), http.StatusConflict)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func (s *server) adminGetScenario(w http.ResponseWriter, r *http.Request) {
	if s.notConfigured(w) {
		return
	}
	sc, err := s.adminScenarios.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		jsonError(w, err.Error(), http.StatusNotFound)
		return
	}
	jsonOK(w, sc)
}

func (s *server) adminPublishScenario(w http.ResponseWriter, r *http.Request) {
	if s.notConfigured(w) {
		return
	}
	if err := s.adminScenarios.SetStatus(r.Context(), r.PathValue("id"), "published"); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *server) adminUnpublishScenario(w http.ResponseWriter, r *http.Request) {
	if s.notConfigured(w) {
		return
	}
	if err := s.adminScenarios.SetStatus(r.Context(), r.PathValue("id"), "draft"); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *server) adminListVersions(w http.ResponseWriter, r *http.Request) {
	if s.notConfigured(w) {
		return
	}
	versions, err := s.adminScenarios.Versions(r.Context(), r.PathValue("id"))
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, versions)
}

func (s *server) adminRestoreVersion(w http.ResponseWriter, r *http.Request) {
	if s.notConfigured(w) {
		return
	}
	versionID, err := strconv.ParseInt(r.PathValue("versionId"), 10, 64)
	if err != nil {
		jsonError(w, "invalid version id", http.StatusBadRequest)
		return
	}
	if err := s.adminScenarios.Restore(r.Context(), r.PathValue("id"), versionID); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *server) adminListFiles(w http.ResponseWriter, r *http.Request) {
	if s.notConfigured(w) {
		return
	}
	paths, err := s.adminScenarios.ListFiles(r.Context(), r.PathValue("id"))
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, paths)
}

func (s *server) adminGetFile(w http.ResponseWriter, r *http.Request) {
	if s.notConfigured(w) {
		return
	}
	content, err := s.adminScenarios.GetFile(r.Context(), r.PathValue("id"), r.PathValue("path"))
	if err != nil {
		jsonError(w, err.Error(), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = io.WriteString(w, content)
}

func (s *server) adminPutFile(w http.ResponseWriter, r *http.Request) {
	if s.notConfigured(w) {
		return
	}
	id := r.PathValue("id")
	path := r.PathValue("path")

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // 1 MB limit
	if err != nil {
		jsonError(w, "read body: "+err.Error(), http.StatusBadRequest)
		return
	}
	content := string(body)

	// shellcheck gate for shell scripts
	if strings.HasSuffix(path, ".sh") {
		if diags, err := shellcheck(content); err != nil {
			jsonError(w, fmt.Sprintf("shellcheck: %v", err), http.StatusInternalServerError)
			return
		} else if len(diags) > 0 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnprocessableEntity)
			_ = json.NewEncoder(w).Encode(map[string]any{"shellcheck": diags})
			return
		}
	}

	if err := s.adminScenarios.PutFile(r.Context(), id, path, content); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// adminLintShell accepts raw shell script in the request body and returns
// shellcheck diagnostics as JSON. Used by the editor for real-time feedback.
func (s *server) adminLintShell(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		jsonError(w, "read body: "+err.Error(), http.StatusBadRequest)
		return
	}
	diags, err := shellcheck(string(body))
	if err != nil {
		// shellcheck not installed or crashed — return empty diagnostics
		// so the editor doesn't block saves in environments without shellcheck.
		jsonOK(w, []any{})
		return
	}
	jsonOK(w, diags)
}

func (s *server) adminScenarioEditorPage(w http.ResponseWriter, r *http.Request) {
	f, err := s.staticFS.Open("scenario-editor.html")
	if err != nil {
		http.Error(w, "editor not found", http.StatusNotFound)
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.Copy(w, f)
}

// shellcheck runs shellcheck on content via stdin and returns structured
// diagnostics. Uses stdin (not a temp file) to avoid path injection risks.
// Returns nil error + empty slice when the script is clean.
func shellcheck(content string) ([]json.RawMessage, error) {
	cmd := exec.Command("shellcheck", "--format=json", "--shell=bash", "-")
	cmd.Stdin = bytes.NewBufferString(content)
	out, err := cmd.Output()
	if err != nil {
		// Exit 1 = issues found (normal). Exit 2+ = tool error.
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			var diags []json.RawMessage
			if jsonErr := json.Unmarshal(out, &diags); jsonErr != nil {
				return nil, jsonErr
			}
			return diags, nil
		}
		return nil, err
	}
	// Exit 0 = clean.
	return nil, nil
}
