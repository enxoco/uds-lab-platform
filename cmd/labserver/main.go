package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strings"
	"strconv"
	"text/template"
	"time"

	labplatform "github.com/defenseunicorns/uds-lab-platform"
	"github.com/defenseunicorns/uds-lab-platform/internal/hetzner"
	"github.com/defenseunicorns/uds-lab-platform/internal/proxy"
	"github.com/defenseunicorns/uds-lab-platform/internal/scenario"
	"github.com/defenseunicorns/uds-lab-platform/internal/session"
	"github.com/google/uuid"
)

type server struct {
	mgr         *session.Manager
	scenariosFS fs.FS
}

func main() {
	hcloudToken := os.Getenv("HCLOUD_TOKEN")
	if hcloudToken == "" {
		fmt.Print("Hetzner API token: ")
		scanner := bufio.NewScanner(os.Stdin)
		if scanner.Scan() {
			hcloudToken = strings.TrimSpace(scanner.Text())
		}
		if hcloudToken == "" {
			log.Fatal("HCLOUD_TOKEN is required")
		}
	}

	ttlMinutes, _ := strconv.Atoi(envOr("SESSION_TTL_MINUTES", "60"))
	serverType := envOr("VM_SERVER_TYPE", "ccx13")
	location := envOr("VM_LOCATION", "hil")
	vmImage := envOr("VM_IMAGE", "ubuntu-24.04")
	port := envOr("PORT", "8080")

	// Scenarios FS: OS override for development, embedded otherwise
	var scenariosFS fs.FS
	if dir := os.Getenv("SCENARIOS_DIR"); dir != "" {
		scenariosFS = os.DirFS(dir)
		log.Printf("using scenarios from %s", dir)
	} else {
		sub, err := fs.Sub(labplatform.ScenariosFS, "scenarios")
		if err != nil {
			log.Fatalf("embedded scenarios: %v", err)
		}
		scenariosFS = sub
	}

	// Static files FS: OS override for development
	var staticFS fs.FS
	if dir := os.Getenv("STATIC_DIR"); dir != "" {
		staticFS = os.DirFS(dir)
		log.Printf("using static files from %s", dir)
	} else {
		sub, err := fs.Sub(labplatform.StaticFiles, "web/static")
		if err != nil {
			log.Fatalf("embedded static: %v", err)
		}
		staticFS = sub
	}

	// VM user-data template: embedded
	vmFS, err := fs.Sub(labplatform.VMFiles, "vm")
	if err != nil {
		log.Fatalf("embedded vm: %v", err)
	}
	tmplData, err := fs.ReadFile(vmFS, "user-data.sh.gotmpl")
	if err != nil {
		log.Fatalf("load user-data template: %v", err)
	}
	udTmpl, err := template.New("user-data.sh.gotmpl").Parse(string(tmplData))
	if err != nil {
		log.Fatalf("parse user-data template: %v", err)
	}

	mgr := session.NewManager(
		hetzner.New(hcloudToken),
		time.Duration(ttlMinutes)*time.Minute,
		session.VMConfig{
			ServerType:   serverType,
			Location:     location,
			Image:        vmImage,
			SSHKeyNames:  []string{"local"},
			UserDataTmpl: udTmpl,
			ScenariosFS:  scenariosFS,
		},
	)

	srv := &server{mgr: mgr, scenariosFS: scenariosFS}
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/scenarios", srv.listScenarios)
	mux.HandleFunc("GET /api/scenarios/{id}", srv.getScenario)
	mux.HandleFunc("POST /api/sessions", srv.createSession)
	mux.HandleFunc("GET /api/sessions/{id}", srv.getSession)
	mux.HandleFunc("DELETE /api/sessions/{id}", srv.deleteSession)
	mux.HandleFunc("POST /t/{id}/cmd", srv.injectCmd)
	mux.HandleFunc("POST /api/sessions/{id}/verify/{step}", srv.verifyStep)
	mux.HandleFunc("/t/{id}/", srv.terminalProxy)
	mux.HandleFunc("/t/{id}/shell/", srv.shellProxy)
	mux.HandleFunc("/vnc/{id}/", srv.browserProxy)
	mux.Handle("/", http.FileServerFS(staticFS))

	log.Printf("labserver listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}

func (s *server) listScenarios(w http.ResponseWriter, r *http.Request) {
	summaries, err := scenario.ListSummaries(s.scenariosFS)
	if err != nil {
		jsonError(w, "cannot read scenarios", http.StatusInternalServerError)
		return
	}
	jsonOK(w, summaries)
}

func (s *server) getScenario(w http.ResponseWriter, r *http.Request) {
	sc, err := scenario.Load(s.scenariosFS, r.PathValue("id"))
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

	cid := clientID(w, r)
	sess, err := s.mgr.Create(r.Context(), cid, req.Scenario)
	if err != nil {
		if err == session.ErrSessionExists {
			jsonError(w, "you already have an active lab session", http.StatusConflict)
			return
		}
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

func (s *server) verifyStep(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.mgr.Get(r.PathValue("id"))
	if !ok {
		jsonError(w, "not found", http.StatusNotFound)
		return
	}
	if sess.Status != session.StatusReady {
		jsonError(w, "terminal not ready", http.StatusServiceUnavailable)
		return
	}
	step := r.PathValue("step")
	body, _ := json.Marshal(map[string]string{"step": step})
	client := &http.Client{Timeout: 35 * time.Second}
	resp, err := client.Post(
		fmt.Sprintf("http://%s:7680/verify", sess.VMIP),
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		jsonError(w, "verify failed", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	w.Header().Set("Content-Type", "application/json")
	io.Copy(w, resp.Body)
}

func (s *server) injectCmd(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.mgr.Get(r.PathValue("id"))
	if !ok {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	if sess.Status != session.StatusReady {
		http.Error(w, "terminal not ready", http.StatusServiceUnavailable)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read body failed", http.StatusBadRequest)
		return
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Post(
		fmt.Sprintf("http://%s:7680/cmd", sess.VMIP),
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		http.Error(w, "injection failed", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	w.WriteHeader(resp.StatusCode)
}

func (s *server) browserProxy(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sess, ok := s.mgr.Get(id)
	if !ok {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	if !sess.BrowserEnabled {
		http.Error(w, "browser not available for this scenario", http.StatusNotFound)
		return
	}
	if sess.Status != session.StatusReady {
		http.Error(w, "terminal not ready", http.StatusServiceUnavailable)
		return
	}
	proxy.Handler(fmt.Sprintf("http://%s:6080", sess.VMIP), fmt.Sprintf("/vnc/%s", id)).ServeHTTP(w, r)
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

// clientID returns a stable client identifier from the lab_client_id cookie,
// setting a new one if absent. Swap this for a GitHub user ID when auth lands.
func clientID(w http.ResponseWriter, r *http.Request) string {
	const cookieName = "lab_client_id"
	if c, err := r.Cookie(cookieName); err == nil && c.Value != "" {
		return c.Value
	}
	id := uuid.New().String()
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    id,
		Path:     "/",
		MaxAge:   86400 * 30,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	return id
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
