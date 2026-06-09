package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"text/template"
	"time"

	labplatform "github.com/enxoco/uds-lab-platform"
	"github.com/enxoco/uds-lab-platform/internal/auth"
	"github.com/enxoco/uds-lab-platform/internal/hetzner"
	"github.com/enxoco/uds-lab-platform/internal/proxy"
	"github.com/enxoco/uds-lab-platform/internal/scenario"
	"github.com/enxoco/uds-lab-platform/internal/session"
	"github.com/google/uuid"
)

type server struct {
	mgr                *session.Manager
	scenariosFS        fs.FS
	staticFS           fs.FS
	ttlMinutes         int
	authStore          *auth.Store
	workshopCode       string
	githubClientID     string
	githubClientSecret string
	githubCallbackURL  string
	adminUsers         map[string]bool
	authEnabled        bool
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

	workshopCode := os.Getenv("WORKSHOP_CODE")
	githubClientID := os.Getenv("GITHUB_CLIENT_ID")
	githubClientSecret := os.Getenv("GITHUB_CLIENT_SECRET")
	githubCallbackURL := envOr("GITHUB_CALLBACK_URL", "http://localhost:"+port+"/auth/callback")
	authEnabled := workshopCode != "" && githubClientID != ""

	adminUsers := map[string]bool{}
	for _, u := range strings.Split(os.Getenv("ADMIN_USERS"), ",") {
		u = strings.TrimSpace(u)
		if u != "" {
			adminUsers[u] = true
		}
	}

	if authEnabled {
		log.Printf("auth enabled: workshop code set, GitHub OAuth configured")
	} else {
		log.Printf("auth disabled: set WORKSHOP_CODE and GITHUB_CLIENT_ID to enable")
	}

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
	injectPy, err := fs.ReadFile(vmFS, "lab-inject.py")
	if err != nil {
		log.Fatalf("load lab-inject.py: %v", err)
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
			InjectPy:     string(injectPy),
		},
	)

	srv := &server{
		mgr:                mgr,
		scenariosFS:        scenariosFS,
		staticFS:           staticFS,
		ttlMinutes:         ttlMinutes,
		authStore:          auth.NewStore(),
		workshopCode:       workshopCode,
		githubClientID:     githubClientID,
		githubClientSecret: githubClientSecret,
		githubCallbackURL:  githubCallbackURL,
		adminUsers:         adminUsers,
		authEnabled:        authEnabled,
	}

	mux := http.NewServeMux()

	// Auth routes (always public)
	mux.HandleFunc("GET /login", srv.loginPage)
	mux.HandleFunc("POST /login", srv.loginSubmit)
	mux.HandleFunc("GET /auth/github", srv.authGitHub)
	mux.HandleFunc("GET /auth/callback", srv.authCallback)

	// Health check (always public)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })

	// Protected API routes
	mux.HandleFunc("GET /api/config", srv.getConfig)
	mux.HandleFunc("GET /api/scenarios", srv.listScenarios)
	mux.HandleFunc("GET /api/scenarios/{id}", srv.getScenario)
	mux.HandleFunc("POST /api/sessions", srv.createSession)
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
	mux.HandleFunc("GET /admin", srv.adminPage)

	// Static file server (catch-all)
	mux.Handle("/", http.FileServerFS(staticFS))

	log.Printf("labserver listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, srv.authMiddleware(mux)))
}

// ── Auth middleware ───────────────────────────────────────────────────────────

func (s *server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.authEnabled {
			next.ServeHTTP(w, r)
			return
		}

		p := r.URL.Path
		// Public paths: auth flow, health, and CSS needed by login page
		if p == "/login" || p == "/auth/github" || p == "/auth/callback" ||
			p == "/healthz" || p == "/style.css" {
			next.ServeHTTP(w, r)
			return
		}

		cid, err := r.Cookie("lab_client_id")
		if err != nil || cid.Value == "" {
			s.unauthResponse(w, r)
			return
		}
		if _, ok := s.authStore.Get(cid.Value); !ok {
			s.unauthResponse(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *server) unauthResponse(w http.ResponseWriter, r *http.Request) {
	p := r.URL.Path
	if strings.HasPrefix(p, "/api/") || strings.HasPrefix(p, "/t/") || strings.HasPrefix(p, "/vnc/") || strings.HasPrefix(p, "/ide-api/") {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
	} else {
		http.Redirect(w, r, "/login", http.StatusFound)
	}
}

func (s *server) requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.authEnabled {
			cid, err := r.Cookie("lab_client_id")
			if err != nil {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			user, ok := s.authStore.Get(cid.Value)
			if !ok || !s.adminUsers[user] {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
		}
		next(w, r)
	}
}

// ── Auth handlers ─────────────────────────────────────────────────────────────

func (s *server) loginPage(w http.ResponseWriter, r *http.Request) {
	// Ensure client ID exists before login so the callback can bind to it.
	clientID(w, r)
	http.ServeFileFS(w, r, s.staticFS, "login.html")
}

func (s *server) loginSubmit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/login?error=invalid", http.StatusFound)
		return
	}
	if r.FormValue("code") != s.workshopCode {
		http.Redirect(w, r, "/login?error=invalid", http.StatusFound)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "lab_code_ok",
		Value:    "1",
		Path:     "/",
		MaxAge:   600,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, "/auth/github", http.StatusFound)
}

func (s *server) authGitHub(w http.ResponseWriter, r *http.Request) {
	if _, err := r.Cookie("lab_code_ok"); err != nil {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	state := uuid.New().String()
	http.SetCookie(w, &http.Cookie{
		Name:     "lab_oauth_state",
		Value:    state,
		Path:     "/",
		MaxAge:   600,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	u := fmt.Sprintf(
		"https://github.com/login/oauth/authorize?client_id=%s&redirect_uri=%s&state=%s&scope=read:user",
		s.githubClientID, s.githubCallbackURL, state,
	)
	http.Redirect(w, r, u, http.StatusFound)
}

func (s *server) authCallback(w http.ResponseWriter, r *http.Request) {
	stateCookie, err := r.Cookie("lab_oauth_state")
	if err != nil || stateCookie.Value != r.URL.Query().Get("state") {
		http.Error(w, "invalid OAuth state", http.StatusBadRequest)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "lab_oauth_state", MaxAge: -1, Path: "/"})

	token, err := s.exchangeGitHubCode(r.Context(), r.URL.Query().Get("code"))
	if err != nil {
		log.Printf("github token exchange: %v", err)
		http.Error(w, "OAuth token exchange failed", http.StatusInternalServerError)
		return
	}

	username, err := s.getGitHubUser(r.Context(), token)
	if err != nil {
		log.Printf("github user fetch: %v", err)
		http.Error(w, "failed to fetch GitHub user", http.StatusInternalServerError)
		return
	}

	cid := clientID(w, r)
	s.authStore.Set(cid, username)
	log.Printf("auth: client %s authenticated as github:%s", cid[:8], username)

	http.SetCookie(w, &http.Cookie{Name: "lab_code_ok", MaxAge: -1, Path: "/"})
	http.Redirect(w, r, "/", http.StatusFound)
}

func (s *server) exchangeGitHubCode(ctx context.Context, code string) (string, error) {
	body, _ := json.Marshal(map[string]string{
		"client_id":     s.githubClientID,
		"client_secret": s.githubClientSecret,
		"code":          code,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://github.com/login/oauth/access_token", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	var result struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	if result.Error != "" {
		return "", fmt.Errorf("github: %s", result.Error)
	}
	return result.AccessToken, nil
}

func (s *server) getGitHubUser(ctx context.Context, token string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/user", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	var user struct {
		Login string `json:"login"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return "", err
	}
	if user.Login == "" {
		return "", fmt.Errorf("empty GitHub username in response")
	}
	return user.Login, nil
}

// ── Admin handlers ────────────────────────────────────────────────────────────

func (s *server) adminPage(w http.ResponseWriter, r *http.Request) {
	s.requireAdmin(func(w http.ResponseWriter, r *http.Request) {
		http.ServeFileFS(w, r, s.staticFS, "admin.html")
	})(w, r)
}

func (s *server) adminListSessions(w http.ResponseWriter, r *http.Request) {
	s.requireAdmin(func(w http.ResponseWriter, r *http.Request) {
		all := s.mgr.All()
		entries := s.authStore.All()
		userMap := make(map[string]string, len(entries))
		for _, e := range entries {
			userMap[e.ClientID] = e.GitHubUser
		}

		type adminSession struct {
			SessionID  string    `json:"session_id"`
			GitHubUser string    `json:"github_user"`
			Scenario   string    `json:"scenario"`
			VMIP       string    `json:"vm_ip"`
			Status     string    `json:"status"`
			ExpiresAt  time.Time `json:"expires_at"`
		}

		result := make([]adminSession, 0, len(all))
		for _, sess := range all {
			result = append(result, adminSession{
				SessionID:  sess.ID,
				GitHubUser: userMap[sess.ClientID],
				Scenario:   sess.Scenario,
				VMIP:       sess.VMIP,
				Status:     string(sess.Status),
				ExpiresAt:  sess.ExpiresAt,
			})
		}
		jsonOK(w, result)
	})(w, r)
}

func (s *server) adminDeleteSession(w http.ResponseWriter, r *http.Request) {
	s.requireAdmin(func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		sess, ok := s.mgr.Get(id)
		if !ok {
			jsonError(w, "not found", http.StatusNotFound)
			return
		}
		clientID := sess.ClientID
		if err := s.mgr.Delete(r.Context(), id); err != nil {
			jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		s.authStore.Delete(clientID)
		w.WriteHeader(http.StatusNoContent)
	})(w, r)
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
	_ = json.NewEncoder(w).Encode(sess)
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
	defer func() { _ = resp.Body.Close() }()
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.Copy(w, resp.Body)
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
	if !sess.BrowserEnabled || sess.Status != session.StatusReady {
		http.Error(w, "browser not available", http.StatusServiceUnavailable)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read body failed", http.StatusBadRequest)
		return
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Post(
		fmt.Sprintf("http://%s:7680/navigate", sess.VMIP),
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

	if sess.BrowserEnabled && sess.Status == session.StatusReady {
		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Get(fmt.Sprintf("http://%s:7680/services", sess.VMIP))
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
	if sess.Status != session.StatusReady {
		http.Error(w, "terminal not ready", http.StatusServiceUnavailable)
		return
	}
	proxy.Handler(fmt.Sprintf("http://%s:6080", sess.VMIP), fmt.Sprintf("/vnc/%s", id)).ServeHTTP(w, r)
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
	proxy.Handler(fmt.Sprintf("http://%s:7680", sess.VMIP), fmt.Sprintf("/ide-api/%s", id)).ServeHTTP(w, r)
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
	if sess.Status != session.StatusReady {
		http.Error(w, "terminal not ready", http.StatusServiceUnavailable)
		return
	}
	proxy.Handler(fmt.Sprintf("http://%s:%d", sess.VMIP, port), stripPrefix).ServeHTTP(w, r)
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

func ownsSession(r *http.Request, sess *session.Session) bool {
	c, err := r.Cookie("lab_client_id")
	return err == nil && c.Value != "" && c.Value == sess.ClientID
}

// clientID returns the stable client identifier from lab_client_id cookie,
// setting a new one if absent.
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
