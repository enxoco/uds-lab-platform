package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"text/template"
	"time"

	"github.com/defenseunicorns/uds-lab-platform/internal/hetzner"
	"github.com/defenseunicorns/uds-lab-platform/internal/proxy"
	"github.com/defenseunicorns/uds-lab-platform/internal/scenario"
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

	mux.HandleFunc("GET /api/scenarios", srv.listScenarios)
	mux.HandleFunc("GET /api/scenarios/{id}", srv.getScenario)
	mux.HandleFunc("POST /api/sessions", srv.createSession)
	mux.HandleFunc("GET /api/sessions/{id}", srv.getSession)
	mux.HandleFunc("DELETE /api/sessions/{id}", srv.deleteSession)
	mux.HandleFunc("/t/{id}/", srv.terminalProxy)
	mux.HandleFunc("/t/{id}/shell/", srv.shellProxy)
	mux.Handle("/", http.FileServer(http.Dir("web/static")))

	log.Printf("labserver listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}

func (s *server) listScenarios(w http.ResponseWriter, r *http.Request) {
	summaries, err := scenario.ListSummaries(s.scenariosDir)
	if err != nil {
		jsonError(w, "cannot read scenarios", http.StatusInternalServerError)
		return
	}
	jsonOK(w, summaries)
}

func (s *server) getScenario(w http.ResponseWriter, r *http.Request) {
	sc, err := scenario.Load(s.scenariosDir, r.PathValue("id"))
	if err != nil {
		jsonError(w, err.Error(), http.StatusNotFound)
		return
	}
	jsonOK(w, sc)
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
	jsonOK(w, sess)
}

func (s *server) deleteSession(w http.ResponseWriter, r *http.Request) {
	if err := s.mgr.Delete(r.Context(), r.PathValue("id")); err != nil {
		jsonError(w, err.Error(), http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) terminalProxy(w http.ResponseWriter, r *http.Request) {
	s.proxyToVM(w, r, r.PathValue("id"), 7681, fmt.Sprintf("/t/%s", r.PathValue("id")))
}

func (s *server) shellProxy(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	s.proxyToVM(w, r, id, 7682, fmt.Sprintf("/t/%s/shell", id))
}

func (s *server) proxyToVM(w http.ResponseWriter, r *http.Request, id string, port int, stripPrefix string) {
	sess, ok := s.mgr.Get(id)
	if !ok {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	if sess.Status != session.StatusReady {
		http.Error(w, "terminal not ready", http.StatusServiceUnavailable)
		return
	}
	proxy.Handler(fmt.Sprintf("http://%s:%d", sess.VMIP, port), stripPrefix).ServeHTTP(w, r)
}

func jsonOK(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
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
