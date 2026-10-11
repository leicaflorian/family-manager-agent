package main

import (
	"encoding/json"
	"net"
	"net/http"
	"runtime"
	"strings"
	"sync/atomic"
	"time"
)

// The address the page calls. The port is fixed: the page and the security rule of Directus
// (connect-src) know only this one. Only the local interface: nothing outside this computer reaches it.
const (
	Port    = "47115"
	Address = "127.0.0.1:" + Port
)

// After this long without a request from an accepted site the agent quits.
const idleTime = 10 * time.Minute

// The header that tells an agent from another program on the same port.
const agentHeader = "X-Family-Manager-Agent"

type Server struct {
	session *Session
	origins *Origins
	last    atomic.Int64
}

func NewServer(session *Session, origins *Origins) *Server {
	server := &Server{session: session, origins: origins}
	server.touch()
	return server
}

func (s *Server) touch() {
	s.last.Store(time.Now().UnixNano())
}

// Idle is true when no accepted site has asked anything for idleTime and no scan is running.
func (s *Server) Idle() bool {
	return !s.session.Scanning() && time.Since(time.Unix(0, s.last.Load())) > idleTime
}

func (s *Server) Serve(listener net.Listener) error {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /status", s.status)
	mux.HandleFunc("POST /scan", s.scan)
	mux.HandleFunc("GET /pages", s.pages)
	mux.HandleFunc("GET /pages/{id}", s.page)
	mux.HandleFunc("DELETE /pages", s.clear)
	server := &http.Server{Handler: s.guard(mux), ReadHeaderTimeout: 10 * time.Second}
	return server.Serve(listener)
}

func answer(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(body)
}

func refuse(w http.ResponseWriter, status int, reason string) {
	answer(w, status, map[string]string{"error": reason})
}

// guard lets through only the calls of a browser, made to this very address, from a site the owner
// accepted. No Origin means not a page (or a page of the agent's own address, which does not exist);
// another Host means a name pointed at 127.0.0.1 to get around the browser.
func (s *Server) guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := w.Header()
		header.Set(agentHeader, Version)
		header.Set("Cache-Control", "no-store")

		origin := r.Header.Get("Origin")
		if r.Host != Address || !validOrigin(origin) {
			refuse(w, http.StatusForbidden, "forbidden")
			return
		}
		// the browser shows the answer to the page only with these; a site not accepted reads
		// nothing but the refusal
		header.Set("Access-Control-Allow-Origin", origin)
		header.Set("Vary", "Origin")

		if r.Method == http.MethodOptions {
			header.Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
			if asked := r.Header.Get("Access-Control-Request-Headers"); asked != "" {
				header.Set("Access-Control-Allow-Headers", asked)
			}
			// what Chrome asks before letting a public site call this computer
			header.Set("Access-Control-Allow-Private-Network", "true")
			header.Set("Access-Control-Max-Age", "600")
			w.WriteHeader(http.StatusNoContent)
			return
		}

		switch s.origins.Check(origin) {
		case OriginAllowed:
			s.touch()
			next.ServeHTTP(w, r)
		case OriginDenied:
			refuse(w, http.StatusForbidden, "origin_denied")
		default:
			// the question is open on the computer: the agent must not quit under it
			s.touch()
			refuse(w, http.StatusForbidden, "origin_pending")
		}
	})
}

func (s *Server) status(w http.ResponseWriter, r *http.Request) {
	scanner := ScannerReady
	// during a scan the scanner is busy: it is there, and asking it would only wait
	if !s.session.Scanning() {
		scanner = s.session.driver.Status()
	}
	answer(w, http.StatusOK, map[string]string{"version": Version, "os": runtime.GOOS, "scanner": scanner})
}

func (s *Server) scan(w http.ResponseWriter, r *http.Request) {
	if !s.session.Start() {
		refuse(w, http.StatusConflict, "scanning")
		return
	}
	answer(w, http.StatusAccepted, map[string]string{"state": StateScanning})
}

func (s *Server) pages(w http.ResponseWriter, r *http.Request) {
	state, failure, pages := s.session.Snapshot()
	body := map[string]any{"session": s.session.ID, "state": state, "error": nil, "message": "", "pages": pages}
	if failure != nil {
		body["error"] = failure.Code
		body["message"] = failure.Message
	}
	answer(w, http.StatusOK, body)
}

func (s *Server) page(w http.ResponseWriter, r *http.Request) {
	path := s.session.Path(strings.TrimSuffix(r.PathValue("id"), ".jpg"))
	if path == "" {
		refuse(w, http.StatusNotFound, "not_found")
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	http.ServeFile(w, r, path)
}

func (s *Server) clear(w http.ResponseWriter, r *http.Request) {
	if !s.session.Clear() {
		refuse(w, http.StatusConflict, "scanning")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// otherAgent is true when the port is held by another copy of the agent.
func otherAgent() bool {
	client := &http.Client{Timeout: 2 * time.Second}
	response, err := client.Get("http://" + Address + "/status")
	if err != nil {
		return false
	}
	defer response.Body.Close()
	return response.Header.Get(agentHeader) != ""
}
