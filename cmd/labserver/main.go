package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	labplatform "github.com/enxoco/uds-lab-platform"
	labv1 "github.com/enxoco/uds-lab-platform/api/v1alpha1"
	"github.com/enxoco/uds-lab-platform/internal/proxy"
	"github.com/enxoco/uds-lab-platform/internal/scenario"
	"github.com/enxoco/uds-lab-platform/internal/session"
	"github.com/google/uuid"
	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type customerInfo struct {
	DisplayName string `yaml:"display_name"`
	CSE         string `yaml:"cse"`
}

type server struct {
	mgr         *session.Manager
	scenariosFS fs.FS
	staticFS    fs.FS
	ttlMinutes  int
	customers   map[string]customerInfo
}

func main() {
	ttlMinutes, _ := strconv.Atoi(envOr("SESSION_TTL_MINUTES", "60"))
	vmNamespace := envOr("VM_NAMESPACE", "uds-lab-vms")
	port := envOr("PORT", "8080")

	// Scenarios FS: OS override for development, embedded otherwise.
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

	// Static files FS: OS override for development.
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

	// Build k8s client (in-cluster or from KUBECONFIG for local dev).
	scheme := runtime.NewScheme()
	if err := labv1.AddToScheme(scheme); err != nil {
		log.Fatalf("register LabSession scheme: %v", err)
	}
	cfg, err := ctrl.GetConfig()
	if err != nil {
		log.Fatalf("get kubeconfig: %v", err)
	}
	k8s, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		log.Fatalf("build k8s client: %v", err)
	}

	mgr := session.NewManager(k8s, vmNamespace, time.Duration(ttlMinutes)*time.Minute, scenariosFS)

	srv := &server{
		mgr:         mgr,
		scenariosFS: scenariosFS,
		staticFS:    staticFS,
		ttlMinutes:  ttlMinutes,
		customers:   loadCustomers(),
	}

	mux := http.NewServeMux()

	// Health check (always public)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })

	// Protected API routes
	mux.HandleFunc("GET /api/config", srv.getConfig)
	mux.HandleFunc("GET /api/scenarios", srv.listScenarios)
	mux.HandleFunc("GET /api/scenarios/{id}", srv.getScenario)
	mux.HandleFunc("POST /api/sessions", srv.createSession)
	mux.HandleFunc("GET /api/sessions/me", srv.getMySession)
	mux.HandleFunc("GET /api/sessions/{id}", srv.getSession)
	mux.HandleFunc("DELETE /api/sessions/{id}", srv.deleteSession)
	mux.HandleFunc("POST /t/{id}/cmd", srv.injectCmd)
	mux.HandleFunc("POST /t/{id}/navigate", srv.navigateBrowser)
	mux.HandleFunc("POST /api/sessions/{id}/verify/{step}", srv.verifyStep)
	mux.HandleFunc("GET /api/sessions/{id}/services", srv.sessionServices)
	mux.HandleFunc("/t/{id}/", srv.terminalProxy)
	mux.HandleFunc("/t/{id}/shell/", srv.shellProxy)
	mux.HandleFunc("/vnc/{id}/", srv.browserProxy)
	mux.HandleFunc("GET /ide/{id}", srv.idePage)
	mux.HandleFunc("/ide-api/{id}/", srv.ideFileProxy)

	// Admin routes
	mux.HandleFunc("GET /api/admin/sessions", srv.adminListSessions)
	mux.HandleFunc("DELETE /api/admin/sessions/{id}", srv.adminDeleteSession)
	mux.HandleFunc("GET /api/admin/csm", srv.adminCSM)
	mux.HandleFunc("GET /admin", srv.adminPage)
	mux.HandleFunc("GET /admin/csm", srv.csmDashboard)

	// Static file server (catch-all)
	mux.Handle("/", http.FileServerFS(staticFS))

	log.Printf("labserver listening on :%s (vm-namespace=%s)", port, vmNamespace)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}


// ── Admin handlers ────────────────────────────────────────────────────────────

func (s *server) adminPage(w http.ResponseWriter, r *http.Request) {
	http.ServeFileFS(w, r, s.staticFS, "admin.html")
}

func (s *server) csmDashboard(w http.ResponseWriter, r *http.Request) {
	http.ServeFileFS(w, r, s.staticFS, "csm.html")
}

func (s *server) adminListSessions(w http.ResponseWriter, r *http.Request) {
	all := s.mgr.All()

	type adminSession struct {
		SessionID  string    `json:"session_id"`
		UserEmail  string    `json:"user_email"`
		Scenario   string    `json:"scenario"`
		ServiceDNS string    `json:"service_dns"`
		Status     string    `json:"status"`
		ExpiresAt  time.Time `json:"expires_at"`
	}

	result := make([]adminSession, 0, len(all))
	for _, sess := range all {
		result = append(result, adminSession{
			SessionID:  sess.ID,
			UserEmail:  sess.UserEmail,
			Scenario:   sess.Scenario,
			ServiceDNS: sess.ServiceDNS,
			Status:     string(sess.Status),
			ExpiresAt:  sess.ExpiresAt,
		})
	}
	jsonOK(w, result)
}

func (s *server) adminDeleteSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.mgr.Delete(r.Context(), id); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) adminCSM(w http.ResponseWriter, r *http.Request) {
	type csmStep struct {
		Status      string     `json:"status"`
		StartedAt   *time.Time `json:"startedAt,omitempty"`
		CompletedAt *time.Time `json:"completedAt,omitempty"`
		DurationH   float64    `json:"durationH,omitempty"`
	}
	type csmUser struct {
		Email     string `json:"email"`
		Completed int    `json:"completed"`
		Active    bool   `json:"active"`
	}
	type csmCustomer struct {
		ID         string      `json:"id"`
		Name       string      `json:"name"`
		Scenario   string      `json:"scenario"`
		CSE        string      `json:"cse"`
		UserCount  int         `json:"user_count"`
		Users      []csmUser   `json:"users"`
		StepTitles []string    `json:"step_titles"`
		Steps      []csmStep   `json:"steps"`
	}

	type groupKey struct{ domain, scenario string }
	type group struct {
		domain  string
		best    *session.Session
		users   []csmUser
	}

	cutoff := time.Now().Add(-30 * 24 * time.Hour)
	groups := map[groupKey]*group{}
	for _, sess := range s.mgr.All() {
		// Skip sessions that expired more than 30 days ago.
		if sess.Status == session.StatusExpired && sess.ExpiresAt.Before(cutoff) {
			continue
		}
		domain := emailDomain(sess.UserEmail)
		if domain == "" {
			continue
		}
		k := groupKey{domain, sess.Scenario}
		g, ok := groups[k]
		if !ok {
			g = &group{domain: domain}
			groups[k] = g
		}
		isActive := sess.Status != session.StatusExpired
		g.users = append(g.users, csmUser{
			Email:     sess.UserEmail,
			Completed: len(sess.CompletedSteps),
			Active:    isActive,
		})
		// Best = most completed steps; tie-break: active beats expired.
		if g.best == nil ||
			len(sess.CompletedSteps) > len(g.best.CompletedSteps) ||
			(len(sess.CompletedSteps) == len(g.best.CompletedSteps) &&
				isActive && g.best.Status == session.StatusExpired) {
			g.best = sess
		}
	}

	result := make([]csmCustomer, 0, len(groups))
	for k, g := range groups {
		sess := g.best
		sc, err := scenario.Load(s.scenariosFS, k.scenario)
		if err != nil {
			continue
		}

		completed := len(sess.CompletedSteps)
		isActive := sess.Status != session.StatusExpired

		steps := make([]csmStep, len(sc.Steps))
		for i := range sc.Steps {
			switch {
			case i < completed:
				rec := sess.CompletedSteps[i]
				t := rec.CompletedAt
				prev := sess.CreatedAt
				if i > 0 {
					prev = sess.CompletedSteps[i-1].CompletedAt
				}
				steps[i] = csmStep{
					Status:      "passed",
					CompletedAt: &t,
					DurationH:   rec.CompletedAt.Sub(prev).Hours(),
				}
			case i == completed && isActive:
				var t time.Time
				if completed > 0 {
					t = sess.CompletedSteps[completed-1].CompletedAt
				} else {
					t = sess.CreatedAt
				}
				steps[i] = csmStep{Status: "active", StartedAt: &t}
			default:
				steps[i] = csmStep{Status: "pending"}
			}
		}

		titles := make([]string, len(sc.Steps))
		for i, step := range sc.Steps {
			titles[i] = step.Title
		}

		info := s.customers[g.domain]
		name := info.DisplayName
		if name == "" {
			name = g.domain
		}
		result = append(result, csmCustomer{
			ID:         g.domain,
			Name:       name,
			Scenario:   sc.Title,
			CSE:        info.CSE,
			UserCount:  len(g.users),
			Users:      g.users,
			StepTitles: titles,
			Steps:      steps,
		})
	}

	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	jsonOK(w, result)
}

func loadCustomers() map[string]customerInfo {
	data, err := labplatform.ConfigFiles.ReadFile("config/customers.yaml")
	if err != nil {
		log.Printf("customers.yaml not found, CSE assignments disabled: %v", err)
		return map[string]customerInfo{}
	}
	var cfg struct {
		Customers map[string]customerInfo `yaml:"customers"`
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		log.Printf("customers.yaml parse error: %v", err)
		return map[string]customerInfo{}
	}
	if cfg.Customers == nil {
		return map[string]customerInfo{}
	}
	return cfg.Customers
}

func emailDomain(email string) string {
	at := strings.LastIndex(email, "@")
	if at < 0 || at == len(email)-1 {
		return ""
	}
	return strings.ToLower(email[at+1:])
}

// ── Existing handlers ─────────────────────────────────────────────────────────

func (s *server) getConfig(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, map[string]any{"session_ttl_minutes": s.ttlMinutes})
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

	userEmail := r.Header.Get("X-Auth-Request-Email")
	cid := clientID(w, r)
	sess, err := s.mgr.Create(r.Context(), cid, req.Scenario, userEmail)
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
	_ = json.NewEncoder(w).Encode(sess)
}

func (s *server) getMySession(w http.ResponseWriter, r *http.Request) {
	cid := clientID(w, r)
	sess, ok := s.mgr.GetActive(r.Context(), cid)
	if !ok {
		jsonError(w, "no active session", http.StatusNotFound)
		return
	}
	jsonOK(w, sess)
}

func (s *server) getSession(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.mgr.Get(r.PathValue("id"))
	if !ok {
		jsonError(w, "not found", http.StatusNotFound)
		return
	}
	if !ownsSession(r, sess) {
		jsonError(w, "forbidden", http.StatusForbidden)
		return
	}
	jsonOK(w, sess)
}

func (s *server) deleteSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sess, ok := s.mgr.Get(id)
	if !ok {
		jsonError(w, "not found", http.StatusNotFound)
		return
	}
	if !ownsSession(r, sess) {
		jsonError(w, "forbidden", http.StatusForbidden)
		return
	}
	if err := s.mgr.Delete(r.Context(), id); err != nil {
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
	if !ownsSession(r, sess) {
		jsonError(w, "forbidden", http.StatusForbidden)
		return
	}
	if sess.Status != session.StatusReady || sess.ServiceDNS == "" {
		jsonError(w, "terminal not ready", http.StatusServiceUnavailable)
		return
	}
	step := r.PathValue("step")
	body, _ := json.Marshal(map[string]string{"step": step})
	httpClient := &http.Client{Timeout: 35 * time.Second}
	resp, err := httpClient.Post(
		fmt.Sprintf("http://%s:7680/verify", sess.ServiceDNS),
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		jsonError(w, "verify failed", http.StatusBadGateway)
		return
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		jsonError(w, "verify: read response", http.StatusBadGateway)
		return
	}
	var result struct {
		Passed bool `json:"passed"`
	}
	if json.Unmarshal(respBody, &result) == nil && result.Passed {
		if err := s.mgr.MarkStepComplete(r.Context(), r.PathValue("id"), step); err != nil {
			log.Printf("mark step complete: %v", err)
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(respBody)
}

func (s *server) injectCmd(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.mgr.Get(r.PathValue("id"))
	if !ok {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	if !ownsSession(r, sess) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if sess.Status != session.StatusReady || sess.ServiceDNS == "" {
		http.Error(w, "terminal not ready", http.StatusServiceUnavailable)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read body failed", http.StatusBadRequest)
		return
	}
	httpClient := &http.Client{Timeout: 5 * time.Second}
	resp, err := httpClient.Post(
		fmt.Sprintf("http://%s:7680/cmd", sess.ServiceDNS),
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		http.Error(w, "injection failed", http.StatusBadGateway)
		return
	}
	defer func() { _ = resp.Body.Close() }()
	w.WriteHeader(resp.StatusCode)
}

func (s *server) navigateBrowser(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.mgr.Get(r.PathValue("id"))
	if !ok {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	if !ownsSession(r, sess) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if !sess.BrowserEnabled || sess.Status != session.StatusReady || sess.ServiceDNS == "" {
		http.Error(w, "browser not available", http.StatusServiceUnavailable)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read body failed", http.StatusBadRequest)
		return
	}
	httpClient := &http.Client{Timeout: 5 * time.Second}
	resp, err := httpClient.Post(
		fmt.Sprintf("http://%s:7680/navigate", sess.ServiceDNS),
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		http.Error(w, "navigate failed", http.StatusBadGateway)
		return
	}
	defer func() { _ = resp.Body.Close() }()
	w.WriteHeader(resp.StatusCode)
}

func (s *server) sessionServices(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.mgr.Get(r.PathValue("id"))
	if !ok {
		jsonError(w, "not found", http.StatusNotFound)
		return
	}
	if !ownsSession(r, sess) {
		jsonError(w, "forbidden", http.StatusForbidden)
		return
	}

	sc, err := scenario.Load(s.scenariosFS, sess.Scenario)
	var services []scenario.ServiceLink
	if err == nil && len(sc.Services) > 0 {
		services = sc.Services
	}

	if sess.BrowserEnabled && sess.Status == session.StatusReady && sess.ServiceDNS != "" {
		httpClient := &http.Client{Timeout: 10 * time.Second}
		resp, err := httpClient.Get(fmt.Sprintf("http://%s:7680/services", sess.ServiceDNS))
		if err == nil {
			defer func() { _ = resp.Body.Close() }()
			var detected []scenario.ServiceLink
			if json.NewDecoder(resp.Body).Decode(&detected) == nil {
				existing := make(map[string]bool, len(services))
				for _, svc := range services {
					existing[svc.URL] = true
				}
				for _, d := range detected {
					if !existing[d.URL] {
						services = append(services, d)
					}
				}
			}
		}
	}

	if services == nil {
		services = []scenario.ServiceLink{}
	}
	jsonOK(w, services)
}

func (s *server) browserProxy(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sess, ok := s.mgr.Get(id)
	if !ok {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	if !ownsSession(r, sess) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if !sess.BrowserEnabled {
		http.Error(w, "browser not available for this scenario", http.StatusNotFound)
		return
	}
	if sess.Status != session.StatusReady || sess.ServiceDNS == "" {
		http.Error(w, "terminal not ready", http.StatusServiceUnavailable)
		return
	}
	proxy.Handler(fmt.Sprintf("http://%s:6080", sess.ServiceDNS), fmt.Sprintf("/vnc/%s", id)).ServeHTTP(w, r)
}

func (s *server) idePage(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, ok := s.mgr.Get(id); !ok {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	http.ServeFileFS(w, r, s.staticFS, "ide.html")
}

func (s *server) ideFileProxy(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sess, ok := s.mgr.Get(id)
	if !ok {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	if !ownsSession(r, sess) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if sess.Status != session.StatusReady {
		http.Error(w, "IDE not ready", http.StatusServiceUnavailable)
		return
	}
	proxy.Handler(fmt.Sprintf("http://%s:7680", sess.ServiceDNS), fmt.Sprintf("/ide-api/%s", id)).ServeHTTP(w, r)
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
	if !ownsSession(r, sess) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if sess.Status != session.StatusReady || sess.ServiceDNS == "" {
		http.Error(w, "terminal not ready", http.StatusServiceUnavailable)
		return
	}
	proxy.Handler(fmt.Sprintf("http://%s:%d", sess.ServiceDNS, port), stripPrefix).ServeHTTP(w, r)
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func jsonOK(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// ownsSession checks whether the requesting user owns the session.
// In production, authservice injects X-Auth-Request-Email and we compare it
// against the email recorded at session create time. In dev (no authservice),
// we fall back to the UUID cookie so local testing still works.
func ownsSession(r *http.Request, sess *session.Session) bool {
	if email := r.Header.Get("X-Auth-Request-Email"); email != "" {
		return strings.EqualFold(email, sess.UserEmail)
	}
	c, err := r.Cookie("lab_client_id")
	return err == nil && c.Value != "" && c.Value == sess.ClientID
}

// clientID returns a stable client identifier for the requesting user.
// In production, we derive a deterministic label-safe key from the Keycloak
// email so that one user always maps to one client regardless of browser or
// device. In dev (no authservice), we fall back to a UUID cookie.
func clientID(w http.ResponseWriter, r *http.Request) string {
	if email := r.Header.Get("X-Auth-Request-Email"); email != "" {
		return emailClientID(email)
	}
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

// emailClientID derives a Kubernetes label-safe client key from an email.
// Uses the first 32 hex chars of SHA-256(lower(email)).
func emailClientID(email string) string {
	h := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(email))))
	return hex.EncodeToString(h[:16])
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
