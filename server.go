package main

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
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

// What the requests of the page that chooses the scanner carry (api=2), and the agent answers with.
// A request without it is of the page of before, which gets what it always got.
const API = 2

// How many different calls refused as forbidden the log keeps: a page that keeps asking does not fill it.
const refusedKept = 20

type Server struct {
	session *Session
	origins *Origins
	last    atomic.Int64
	// the calls refused as forbidden already written in the log
	refusedMu sync.Mutex
	refused   map[string]bool
	// the scanners last listed: what GET /scanners answers during a scan
	listMu sync.Mutex
	list   []Scanner
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
	mux.HandleFunc("GET /scanners", s.scanners)
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
			s.noteRefused(r)
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

// noteRefused writes in the log a call refused as forbidden, with what it carried: the page takes
// such a refusal for an agent that does not answer, and nothing else tells why. The names of the
// headers are there to show a program between the browser and the agent. Each different call is
// written once.
func (s *Server) noteRefused(r *http.Request) {
	names := make([]string, 0, len(r.Header))
	for name := range r.Header {
		names = append(names, name)
	}
	sort.Strings(names)
	call := fmt.Sprintf("%s %q, Host %q, Origin %s, from %q, headers %q", r.Method, clip(r.URL.Path, 100),
		clip(r.Host, 100), clip(fmt.Sprintf("%q", r.Header.Values("Origin")), 200), clip(r.UserAgent(), 200),
		clip(strings.Join(names, " "), 500))

	s.refusedMu.Lock()
	defer s.refusedMu.Unlock()
	if s.refused[call] || len(s.refused) >= refusedKept {
		return
	}
	if s.refused == nil {
		s.refused = map[string]bool{}
	}
	s.refused[call] = true
	logf("refused: %s", call)
}

// clip keeps the start of a text that came with a call.
func clip(text string, most int) string {
	if len(text) > most {
		return text[:most] + "..."
	}
	return text
}

// chooses is true for a request of the page that chooses the scanner.
func chooses(r *http.Request) bool {
	return r.URL.Query().Get("api") == strconv.Itoa(API)
}

func (s *Server) scanners(w http.ResponseWriter, r *http.Request) {
	s.listMu.Lock()
	defer s.listMu.Unlock()
	// during a scan the scanner is busy, and on Windows the list would wait for it
	if !s.session.Scanning() {
		s.list = s.session.driver.List()
	}
	list := s.list
	if list == nil {
		list = []Scanner{}
	}
	answer(w, http.StatusOK, map[string]any{"scanners": list})
}

func (s *Server) status(w http.ResponseWriter, r *http.Request) {
	body := map[string]any{"version": Version, "os": runtime.GOOS, "scanner": ScannerReady}
	query := r.URL.Query()
	scanner, source := query.Get("scanner"), query.Get("source")
	if chooses(r) {
		body["api"] = API
		if source != "" && source != SourceFeeder && source != SourceFlatbed {
			refuse(w, http.StatusBadRequest, "settings")
			return
		}
	} else {
		scanner, source = "", ""
	}
	switch {
	case s.session.Scanning():
		// during a scan the scanner is busy: it is there, and asking it would only wait
	case chooses(r) && scanner == "":
		// the page has not chosen a scanner yet: none is opened
		body["scanner"] = ScannerOff
	default:
		state, has := s.session.driver.Status(scanner, source)
		body["scanner"] = state
		if has != nil && chooses(r) {
			body["source"], body["sources"] = has.Source, has.Sources
			body["resolutions"], body["colours"] = has.Resolutions, has.Colours
		}
	}
	answer(w, http.StatusOK, body)
}

func (s *Server) scan(w http.ResponseWriter, r *http.Request) {
	settings := Before
	if chooses(r) {
		query := r.URL.Query()
		resolution, _ := strconv.Atoi(query.Get("resolution"))
		settings = Settings{Scanner: query.Get("scanner"), Source: query.Get("source"), Resolution: resolution, Colour: query.Get("colour")}
		valid := settings.Scanner != "" && resolution > 0 &&
			(settings.Source == SourceFeeder || settings.Source == SourceFlatbed) &&
			(settings.Colour == ColourColour || settings.Colour == ColourGrey)
		// what the scanner has is known only with the scanner open: of one that is off the scan says so
		if valid && !s.session.Scanning() {
			if _, has := s.session.driver.Status(settings.Scanner, settings.Source); has != nil {
				valid = has.Has(settings)
			}
		}
		if !valid {
			refuse(w, http.StatusBadRequest, "settings")
			return
		}
	}
	if !s.session.Start(settings) {
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
