package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/defenseunicorns/uds-lab-platform/internal/hetzner"
	"github.com/defenseunicorns/uds-lab-platform/internal/proxy"
	"github.com/defenseunicorns/uds-lab-platform/internal/session"
)

type server struct {
	mgr          *session.Manager
	scenariosDir string
}

func main() {
	hcloudToken := requireEnv("HCLOUD_TOKEN")
	scenariosDir := envOr("SCENARIOS_DIR", "scenarios")
	ttlMinutes, _ := strconv.Atoi(envOr("SESSION_TTL_MINUTES", "60"))
	serverType := envOr("VM_SERVER_TYPE", "ccx13")
	location := envOr("VM_LOCATION", "hil")
	port := envOr("PORT", "8080")

	udTmpl, err := template.ParseFiles(filepath.Join("vm", "user-data.sh.gotmpl"))
	if err != nil {
		log.Fatalf("load user-data template: %v", err)
	}

	mgr := session.NewManager(
		hetzner.New(hcloudToken),
		time.Duration(ttlMinutes)*time.Minute,
		session.VMConfig{
			ServerType:   serverType,
			Location:     location,
			SSHKeyNames:  []string{"local"},
			UserDataTmpl: udTmpl,
			ScenariosDir: scenariosDir,
		},
	)

	srv := &server{mgr: mgr, scenariosDir: scenariosDir}

	mux := http.NewServeMux()

	// API
	mux.HandleFunc("POST /api/sessions", srv.createSession)
	mux.HandleFunc("GET /api/sessions/{id}", srv.getSession)
	mux.HandleFunc("DELETE /api/sessions/{id}", srv.deleteSession)
	mux.HandleFunc("GET /api/scenarios", srv.listScenarios)

	// Terminal proxy: /t/{sessionID}/* → VM ttyd
	mux.HandleFunc("/t/{id}/", srv.terminalProxy)

	// Static frontend
	mux.Handle("/", http.FileServer(http.Dir("web/static")))

	log.Printf("labserver listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}

func (s *server) createSession(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Scenario string `json:"scenario"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Scenario == "" {
		jsonError(w, "scenario required", http.StatusBadRequest)
		return
	}

	sess, err := s.mgr.Create(r.Context(), req.Scenario)
	if err != nil {
		log.Printf("create session: %v", err)
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(sess)
}

func (s *server) getSession(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.mgr.Get(r.PathValue("id"))
	if !ok {
		jsonError(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sess)
}

func (s *server) deleteSession(w http.ResponseWriter, r *http.Request) {
	if err := s.mgr.Delete(r.Context(), r.PathValue("id")); err != nil {
		jsonError(w, err.Error(), http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) listScenarios(w http.ResponseWriter, r *http.Request) {
	entries, err := os.ReadDir(s.scenariosDir)
	if err != nil {
		jsonError(w, "cannot read scenarios", http.StatusInternalServerError)
		return
	}

	type ScenarioMeta struct {
		ID    string `json:"id"`
		Title string `json:"title,omitempty"`
	}

	out := []ScenarioMeta{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		meta := ScenarioMeta{ID: e.Name()}
		// Try to read title from scenario.yaml (simple grep, no YAML lib dep)
		if b, err := os.ReadFile(filepath.Join(s.scenariosDir, e.Name(), "scenario.yaml")); err == nil {
			for _, line := range splitLines(string(b)) {
				if len(line) > 7 && line[:6] == "title:" {
					meta.Title = trimQuotes(line[7:])
					break
				}
			}
		}
		out = append(out, meta)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}

func (s *server) terminalProxy(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sess, ok := s.mgr.Get(id)
	if !ok {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	if sess.Status != session.StatusReady {
		http.Error(w, "terminal not ready", http.StatusServiceUnavailable)
		return
	}

	stripPrefix := fmt.Sprintf("/t/%s", id)
	target := fmt.Sprintf("http://%s:7681", sess.VMIP)
	proxy.Handler(target, stripPrefix).ServeHTTP(w, r)
}

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func requireEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("required env var %s not set", key)
	}
	return v
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i, c := range s {
		if c == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	return append(lines, s[start:])
}

func trimQuotes(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return s
}
