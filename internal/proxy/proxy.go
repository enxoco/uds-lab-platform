package proxy

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/gorilla/websocket"
)

// Handler proxies HTTP and WebSocket traffic to target, stripping prefix.
// Returns an error if target is not a valid URL.
func Handler(target string, stripPrefix string) (http.Handler, error) {
	targetURL, err := url.Parse(target)
	if err != nil {
		return nil, fmt.Errorf("proxy: invalid target URL %q: %w", target, err)
	}
	rp := httputil.NewSingleHostReverseProxy(targetURL)

	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.URL.Path = strings.TrimPrefix(r.URL.Path, stripPrefix)
		if r.URL.Path == "" {
			r.URL.Path = "/"
		}

		if isWebSocket(r) {
			proxyWS(w, r, fmt.Sprintf("ws://%s%s", targetURL.Host, r.URL.Path)) // nosemgrep: javascript.lang.security.detect-insecure-websocket.detect-insecure-websocket -- in-cluster pod-to-pod; encrypted by Istio mTLS
			return
		}
		rp.ServeHTTP(w, r)
	})
	return h, nil
}

func proxyWS(w http.ResponseWriter, r *http.Request, targetWS string) {
	subprotocols := websocket.Subprotocols(r)
	dialHeader := http.Header{}
	if len(subprotocols) > 0 {
		dialHeader.Set("Sec-WebSocket-Protocol", strings.Join(subprotocols, ", "))
	}
	upstream, _, err := websocket.DefaultDialer.Dial(targetWS, dialHeader)
	if err != nil {
		http.Error(w, "terminal not ready", http.StatusBadGateway)
		return
	}
	defer func() { _ = upstream.Close() }()

	u := websocket.Upgrader{
		// Only allow connections from the same host to prevent cross-origin
		// WebSocket hijacking of terminal sessions. Normalize both sides by
		// stripping the port before comparing: browsers omit default ports in
		// the Origin header (e.g. "example.com" not "example.com:443") while
		// r.Host may include an explicit port from the upstream proxy.
		CheckOrigin: func(r *http.Request) bool {
			origin := r.Header.Get("Origin")
			if origin == "" {
				return true
			}
			ou, err := url.Parse(origin)
			if err != nil {
				return false
			}
			ouHost := ou.Hostname() // strips port
			rHost := r.Host
			if h, _, err := net.SplitHostPort(r.Host); err == nil {
				rHost = h
			}
			return ouHost == rHost
		},
		Subprotocols: subprotocols,
	}
	client, err := u.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer func() { _ = client.Close() }()

	errc := make(chan error, 2)
	cp := func(dst, src *websocket.Conn) {
		for {
			mt, msg, err := src.ReadMessage()
			if err != nil {
				errc <- err
				return
			}
			if err := dst.WriteMessage(mt, msg); err != nil {
				errc <- err
				return
			}
		}
	}

	go cp(upstream, client)
	go cp(client, upstream)
	<-errc
}

func isWebSocket(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("Upgrade"), "websocket")
}
