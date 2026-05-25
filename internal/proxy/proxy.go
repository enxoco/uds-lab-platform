package proxy

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// Handler proxies HTTP and WebSocket traffic to target, stripping prefix.
func Handler(target string, stripPrefix string) http.Handler {
	targetURL, _ := url.Parse(target)
	rp := httputil.NewSingleHostReverseProxy(targetURL)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.URL.Path = strings.TrimPrefix(r.URL.Path, stripPrefix)
		if r.URL.Path == "" {
			r.URL.Path = "/"
		}

		if isWebSocket(r) {
			proxyWS(w, r, fmt.Sprintf("ws://%s%s", targetURL.Host, r.URL.Path))
			return
		}
		rp.ServeHTTP(w, r)
	})
}

func proxyWS(w http.ResponseWriter, r *http.Request, targetWS string) {
	subprotocols := websocket.Subprotocols(r)
	dialHeader := http.Header{}
	if len(subprotocols) > 0 {
		dialHeader.Set("Sec-WebSocket-Protocol", strings.Join(subprotocols, ", "))
	}
	upstream, upstreamResp, err := websocket.DefaultDialer.Dial(targetWS, dialHeader)
	if err != nil {
		http.Error(w, "terminal not ready", http.StatusBadGateway)
		return
	}
	defer upstream.Close()

	var respHeader http.Header
	if proto := upstreamResp.Header.Get("Sec-WebSocket-Protocol"); proto != "" {
		respHeader = http.Header{"Sec-WebSocket-Protocol": []string{proto}}
	}
	u := websocket.Upgrader{
		CheckOrigin:  func(r *http.Request) bool { return true },
		Subprotocols: subprotocols,
	}
	client, err := u.Upgrade(w, r, respHeader)
	if err != nil {
		return
	}
	defer client.Close()

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
