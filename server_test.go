package main

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

// agent is a server of the agent with the scanner and the owner of the test. It listens on a port
// of its own, not on the one of the agent: an agent open on this computer is left alone.
type agent struct {
	server  *Server
	session *Session
	scanner *feeder
	person  *owner
	url     string
}

// startAgent opens the server with the answers given already in origins.json.
func startAgent(t *testing.T, answers map[string]bool) *agent {
	t.Helper()
	path := originsFile(t)
	if answers != nil {
		content, _ := json.Marshal(answers)
		if err := os.WriteFile(path, content, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	a := &agent{scanner: newFeeder(), person: &owner{}}
	a.session = sessionWith(t, a.scanner)
	a.server = NewServer(a.session, loadOrigins(path, a.person.ask))
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	a.url = "http://" + listener.Addr().String()
	go a.server.Serve(listener)
	t.Cleanup(func() { listener.Close() })
	return a
}

// reply is what the agent answered to a call.
type reply struct {
	status int
	header http.Header
	body   []byte
}

// reason is the "error" of a refusal.
func (r reply) reason() string {
	return r.text("error")
}

func (r reply) text(field string) string {
	fields := map[string]any{}
	json.Unmarshal(r.body, &fields)
	text, _ := fields[field].(string)
	return text
}

var browser = &http.Client{Transport: &http.Transport{DisableKeepAlives: true}, Timeout: 10 * time.Second}

// call asks the agent as the browser does for a page of the site given: with the address of the
// agent as Host and the site as Origin. No site is a call without Origin. A change, when given,
// touches the request before it leaves.
func (a *agent) call(t *testing.T, method, path, site string, change ...func(*http.Request)) reply {
	t.Helper()
	request, err := http.NewRequest(method, a.url+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Host = Address
	if site != "" {
		request.Header.Set("Origin", site)
	}
	for _, apply := range change {
		apply(request)
	}
	response, err := browser.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return reply{response.StatusCode, response.Header, body}
}

// scan feeds the sides given through POST /scan and waits for the end.
func (a *agent) scan(t *testing.T, site string, failure *ScanError, sides ...string) {
	t.Helper()
	if got := a.call(t, "POST", "/scan", site); got.status != http.StatusAccepted {
		t.Fatalf("POST /scan: %d %s", got.status, got.body)
	}
	for _, side := range sides {
		a.scanner.feed(side)
	}
	a.scanner.finish(failure)
	until(t, "the end of the scan", func() bool { return !a.session.Scanning() })
}

// The requests of the page, one for each line of the table in the README.
var requests = [][2]string{
	{"GET", "/scanners"}, {"GET", "/status"}, {"POST", "/scan"}, {"GET", "/pages"}, {"GET", "/pages/1"}, {"DELETE", "/pages"},
}

// Without a site as Origin, or to another Host, nothing is answered and nothing is asked: not the
// scanner, not the owner.
func TestGuardRefusesWhatIsNotAPageCallingTheAgent(t *testing.T) {
	a := startAgent(t, map[string]bool{siteHome: true})
	host := func(name string) func(*http.Request) {
		return func(request *http.Request) { request.Host = name }
	}
	cases := []struct {
		name   string
		site   string
		change func(*http.Request)
	}{
		{"no Origin", "", func(*http.Request) {}},
		{"a page opened from a file", "null", func(*http.Request) {}},
		{"an Origin with a path", siteHome + "/admin", func(*http.Request) {}},
		{"an Origin that is not a site", "file:///Users/me/page.html", func(*http.Request) {}},
		{"a name pointed at this computer", siteHome, host("agent.example:" + Port)},
		{"localhost", siteHome, host("localhost:" + Port)},
		{"no port", siteHome, host("127.0.0.1")},
		{"another port", siteHome, host("127.0.0.1:8080")},
		{"the site as Host", siteHome, host("family.example")},
	}
	for _, c := range cases {
		for _, request := range requests {
			got := a.call(t, request[0], request[1], c.site, c.change)
			if got.status != http.StatusForbidden || got.reason() != "forbidden" {
				t.Errorf("%s, %s %s: %d %s", c.name, request[0], request[1], got.status, got.body)
			}
			if allowed := got.header.Get("Access-Control-Allow-Origin"); allowed != "" {
				t.Errorf("%s, %s %s: the answer can be read by %s", c.name, request[0], request[1], allowed)
			}
		}
		if got := a.call(t, "OPTIONS", "/status", c.site, c.change); got.status != http.StatusForbidden {
			t.Errorf("%s, OPTIONS: %d", c.name, got.status)
		}
	}
	if asked, scans := a.scanner.count(); asked != 0 || scans != 0 {
		t.Errorf("the scanner was asked: %d times its status, %d scans", asked, scans)
	}
	if questions := a.person.questions(); len(questions) != 0 {
		t.Errorf("the owner was asked about %q", questions)
	}
}

func TestEveryAnswerSaysItIsTheAgent(t *testing.T) {
	runAs(t, "1.2.3")
	a := startAgent(t, map[string]bool{siteHome: true, siteLocal: false})
	for _, site := range []string{"", siteHome, siteLocal, siteMoved} {
		got := a.call(t, "GET", "/status", site)
		if version := got.header.Get(agentHeader); version != "1.2.3" {
			t.Errorf("from %q: %s is %q", site, agentHeader, version)
		}
		if cache := got.header.Get("Cache-Control"); cache != "no-store" {
			t.Errorf("from %q: Cache-Control is %q", site, cache)
		}
	}
	a.person.answer(siteMoved, false)
	until(t, "the answer", func() bool { return a.call(t, "GET", "/status", siteMoved).reason() == "origin_denied" })
}

func TestAcceptedSiteIsAnswered(t *testing.T) {
	runAs(t, "1.2.3")
	a := startAgent(t, map[string]bool{siteHome: true, siteLocal: true})
	for _, site := range []string{siteHome, siteLocal} {
		got := a.call(t, "GET", "/status", site)
		if got.status != http.StatusOK {
			t.Fatalf("%s: %d %s", site, got.status, got.body)
		}
		// the browser shows the answer only to the site that asked
		if allowed := got.header.Get("Access-Control-Allow-Origin"); allowed != site {
			t.Errorf("%s: the answer can be read by %q", site, allowed)
		}
		if vary := got.header.Get("Vary"); vary != "Origin" {
			t.Errorf("%s: Vary is %q", site, vary)
		}
		if kind := got.header.Get("Content-Type"); !strings.HasPrefix(kind, "application/json") {
			t.Errorf("%s: Content-Type is %q", site, kind)
		}
	}
	if questions := a.person.questions(); len(questions) != 0 {
		t.Errorf("the owner was asked about %q", questions)
	}
}

func TestRefusedSiteGetsNothing(t *testing.T) {
	a := startAgent(t, map[string]bool{siteLocal: false})
	for _, request := range requests {
		got := a.call(t, request[0], request[1], siteLocal)
		if got.status != http.StatusForbidden || got.reason() != "origin_denied" {
			t.Errorf("%s %s: %d %s", request[0], request[1], got.status, got.body)
		}
		// the page reads the refusal, to say that the site was refused
		if allowed := got.header.Get("Access-Control-Allow-Origin"); allowed != siteLocal {
			t.Errorf("%s %s: the refusal can be read by %q", request[0], request[1], allowed)
		}
	}
	if asked, scans := a.scanner.count(); asked != 0 || scans != 0 {
		t.Errorf("the scanner was asked: %d times its status, %d scans", asked, scans)
	}
	if questions := a.person.questions(); len(questions) != 0 {
		t.Errorf("the owner was asked about %q", questions)
	}
}

// A site never seen waits for the owner: one question, however many times the page asks, and
// nothing reaches the scanner meanwhile.
func TestSiteNeverSeenWaitsForTheOwner(t *testing.T) {
	for _, allowed := range []bool{true, false} {
		a := startAgent(t, map[string]bool{siteHome: true})
		for i := 0; i < 3; i++ {
			for _, request := range requests {
				got := a.call(t, request[0], request[1], siteMoved)
				if got.status != http.StatusForbidden || got.reason() != "origin_pending" {
					t.Errorf("%s %s: %d %s", request[0], request[1], got.status, got.body)
				}
				if reader := got.header.Get("Access-Control-Allow-Origin"); reader != siteMoved {
					t.Errorf("%s %s: the refusal can be read by %q", request[0], request[1], reader)
				}
			}
		}
		until(t, "the question", func() bool { return len(a.person.questions()) > 0 })
		if asked, scans := a.scanner.count(); asked != 0 || scans != 0 {
			t.Errorf("before the answer the scanner was asked: %d times its status, %d scans", asked, scans)
		}
		// the site already accepted goes on working under the question
		if got := a.call(t, "GET", "/status", siteHome); got.status != http.StatusOK {
			t.Errorf("%s under the question: %d %s", siteHome, got.status, got.body)
		}

		a.person.answer(siteMoved, allowed)
		want, reason := http.StatusOK, ""
		if !allowed {
			want, reason = http.StatusForbidden, "origin_denied"
		}
		until(t, "the answer", func() bool { return a.call(t, "GET", "/pages", siteMoved).reason() != "origin_pending" })
		if got := a.call(t, "GET", "/pages", siteMoved); got.status != want || got.reason() != reason {
			t.Errorf("answered %v: %d %s", allowed, got.status, got.body)
		}
		if questions := a.person.questions(); !reflect.DeepEqual(questions, []string{siteMoved}) {
			t.Errorf("answered %v: questions %q", allowed, questions)
		}
	}
}

// What the browser asks before the real request gets the headers it needs, also for a site never
// seen or refused, and opens no question.
func TestOptionsAnswersTheBrowser(t *testing.T) {
	a := startAgent(t, map[string]bool{siteHome: true, siteLocal: false})
	for _, site := range []string{siteHome, siteLocal, siteMoved} {
		got := a.call(t, "OPTIONS", "/scan", site, func(request *http.Request) {
			request.Header.Set("Access-Control-Request-Method", "POST")
			request.Header.Set("Access-Control-Request-Headers", "content-type, x-other")
			request.Header.Set("Access-Control-Request-Private-Network", "true")
		})
		if got.status != http.StatusNoContent || len(got.body) != 0 {
			t.Errorf("%s: %d %q", site, got.status, got.body)
		}
		want := map[string]string{
			"Access-Control-Allow-Origin":          site,
			"Access-Control-Allow-Methods":         "GET, POST, DELETE, OPTIONS",
			"Access-Control-Allow-Headers":         "content-type, x-other",
			"Access-Control-Allow-Private-Network": "true",
			"Access-Control-Max-Age":               "600",
			"Vary":                                 "Origin",
		}
		for name, value := range want {
			if got := got.header.Get(name); got != value {
				t.Errorf("%s: %s is %q, want %q", site, name, got, value)
			}
		}
	}
	if got := a.call(t, "OPTIONS", "/status", siteHome); got.header.Get("Access-Control-Allow-Headers") != "" {
		t.Errorf("headers allowed that nobody asked for: %q", got.header.Get("Access-Control-Allow-Headers"))
	}
	if questions := a.person.questions(); len(questions) != 0 {
		t.Errorf("the owner was asked about %q", questions)
	}
	if asked, scans := a.scanner.count(); asked != 0 || scans != 0 {
		t.Errorf("the scanner was asked: %d times its status, %d scans", asked, scans)
	}
}

func TestStatusSaysWhatTheScannerSays(t *testing.T) {
	runAs(t, "1.2.3")
	a := startAgent(t, map[string]bool{siteHome: true})
	for _, state := range []string{ScannerReady, ScannerEmpty, ScannerOff} {
		a.scanner.mu.Lock()
		a.scanner.status = state
		a.scanner.mu.Unlock()

		got := a.call(t, "GET", "/status", siteHome)
		fields := map[string]string{}
		if err := json.Unmarshal(got.body, &fields); err != nil {
			t.Fatalf("%s: %v", got.body, err)
		}
		if want := map[string]string{"version": "1.2.3", "os": runtime.GOOS, "scanner": state}; !reflect.DeepEqual(fields, want) {
			t.Errorf("scanner %s: %v, want %v", state, fields, want)
		}
	}
}

// During a scan the scanner is busy: the status is "ready" without asking it.
func TestStatusDuringAScanDoesNotAskTheScanner(t *testing.T) {
	a := startAgent(t, map[string]bool{siteHome: true})
	a.scanner.status = ScannerOff
	if got := a.call(t, "POST", "/scan", siteHome); got.status != http.StatusAccepted || got.text("state") != StateScanning {
		t.Fatalf("POST /scan: %d %s", got.status, got.body)
	}

	if got := a.call(t, "GET", "/status", siteHome); got.text("scanner") != ScannerReady {
		t.Errorf("during the scan: %s", got.body)
	}
	if asked, _ := a.scanner.count(); asked != 0 {
		t.Errorf("the scanner was asked its status %d times", asked)
	}
	a.scanner.finish(nil)
	until(t, "the end of the scan", func() bool { return !a.session.Scanning() })
	if got := a.call(t, "GET", "/status", siteHome); got.text("scanner") != ScannerOff {
		t.Errorf("after the scan: %s", got.body)
	}
}

func TestScanAndClearAreRefusedDuringAScan(t *testing.T) {
	a := startAgent(t, map[string]bool{siteHome: true})
	if got := a.call(t, "POST", "/scan", siteHome); got.status != http.StatusAccepted {
		t.Fatalf("POST /scan: %d %s", got.status, got.body)
	}
	a.scanner.feed("side")
	until(t, "the first side", func() bool { return len(sidesOf(a.session)) == 1 })

	for _, request := range [][2]string{{"POST", "/scan"}, {"DELETE", "/pages"}} {
		if got := a.call(t, request[0], request[1], siteHome); got.status != http.StatusConflict || got.reason() != "scanning" {
			t.Errorf("%s %s during a scan: %d %s", request[0], request[1], got.status, got.body)
		}
	}
	// the sides already arrived can be read while the others pass
	if got := a.call(t, "GET", "/pages/1", siteHome); got.status != http.StatusOK || string(got.body) != "side" {
		t.Errorf("GET /pages/1 during a scan: %d %q", got.status, got.body)
	}
	if got := a.call(t, "GET", "/pages", siteHome); got.text("state") != StateScanning {
		t.Errorf("GET /pages during a scan: %s", got.body)
	}
	a.scanner.finish(nil)
	until(t, "the end of the scan", func() bool { return !a.session.Scanning() })
	if _, scans := a.scanner.count(); scans != 1 {
		t.Errorf("%d scans, want 1", scans)
	}
}

// GET /pages, field by field: it is what the page reads every half second.
func TestPagesListsTheSides(t *testing.T) {
	a := startAgent(t, map[string]bool{siteHome: true})
	a.session.ID = "a1b2"
	pages := func() string {
		t.Helper()
		got := a.call(t, "GET", "/pages", siteHome)
		if got.status != http.StatusOK {
			t.Fatalf("GET /pages: %d %s", got.status, got.body)
		}
		return strings.TrimSpace(string(got.body))
	}

	if got, want := pages(), `{"error":null,"message":"","pages":[],"session":"a1b2","state":"idle"}`; got != want {
		t.Errorf("before a scan:\n%s\nwant\n%s", got, want)
	}
	a.scan(t, siteHome, nil, "front", "the back")
	// of a side the page gets the id and the size, not where the file is
	if got, want := pages(), `{"error":null,"message":"","pages":[{"id":"1","size":5},{"id":"2","size":8}],"session":"a1b2","state":"done"}`; got != want {
		t.Errorf("after a scan:\n%s\nwant\n%s", got, want)
	}
	a.scan(t, siteHome, &ScanError{Code: ErrorJam, Message: "a sheet is stuck"}, "3")
	if got, want := pages(), `{"error":"jam","message":"a sheet is stuck","pages":[{"id":"1","size":5},{"id":"2","size":8},{"id":"3","size":1}],"session":"a1b2","state":"error"}`; got != want {
		t.Errorf("after a scan that stopped:\n%s\nwant\n%s", got, want)
	}
}

func TestPageGivesTheSide(t *testing.T) {
	a := startAgent(t, map[string]bool{siteHome: true, siteLocal: false})
	a.scan(t, siteHome, nil, "front", "the back")

	for _, path := range []string{"/pages/2", "/pages/2.jpg"} {
		got := a.call(t, "GET", path, siteHome)
		if got.status != http.StatusOK || string(got.body) != "the back" {
			t.Errorf("%s: %d %q", path, got.status, got.body)
		}
		if kind := got.header.Get("Content-Type"); kind != "image/jpeg" {
			t.Errorf("%s: Content-Type is %q", path, kind)
		}
		if cache := got.header.Get("Cache-Control"); cache != "no-store" {
			t.Errorf("%s: Cache-Control is %q", path, cache)
		}
	}
	for _, path := range []string{"/pages/3", "/pages/0", "/pages/x", "/pages/..%2F2", "/pages/scan-1-side-2.tmp"} {
		if got := a.call(t, "GET", path, siteHome); got.status != http.StatusNotFound || got.reason() != "not_found" {
			t.Errorf("%s: %d %s", path, got.status, got.body)
		}
	}
	if got := a.call(t, "GET", "/pages/2", siteLocal); got.status != http.StatusForbidden || strings.Contains(string(got.body), "the back") {
		t.Errorf("a refused site: %d %q", got.status, got.body)
	}
}

func TestDeleteClearsTheSides(t *testing.T) {
	a := startAgent(t, map[string]bool{siteHome: true})
	a.scan(t, siteHome, nil, "front", "the back")

	if got := a.call(t, "DELETE", "/pages", siteHome); got.status != http.StatusNoContent || len(got.body) != 0 {
		t.Errorf("DELETE /pages: %d %q", got.status, got.body)
	}
	if got := a.call(t, "GET", "/pages", siteHome); got.text("state") != StateIdle || !strings.Contains(string(got.body), `"pages":[]`) {
		t.Errorf("GET /pages: %s", got.body)
	}
	if got := a.call(t, "GET", "/pages/1", siteHome); got.status != http.StatusNotFound {
		t.Errorf("GET /pages/1: %d", got.status)
	}
	if left := files(t, a.session); len(left) != 0 {
		t.Errorf("still in the folder: %q", left)
	}
}

func TestOtherRequestsAreNotThere(t *testing.T) {
	a := startAgent(t, map[string]bool{siteHome: true})
	cases := map[[2]string]int{
		{"GET", "/"}:           http.StatusNotFound,
		{"GET", "/scan"}:       http.StatusMethodNotAllowed,
		{"PUT", "/scan"}:       http.StatusMethodNotAllowed,
		{"POST", "/status"}:    http.StatusMethodNotAllowed,
		{"POST", "/scanners"}:  http.StatusMethodNotAllowed,
		{"DELETE", "/status"}:  http.StatusMethodNotAllowed,
		{"DELETE", "/pages/1"}: http.StatusMethodNotAllowed,
		{"GET", "/origins"}:    http.StatusNotFound,
		{"GET", "/pages/1/2"}:  http.StatusNotFound,
	}
	for request, want := range cases {
		if got := a.call(t, request[0], request[1], siteHome); got.status != want {
			t.Errorf("%s %s: %d, want %d", request[0], request[1], got.status, want)
		}
	}
	if _, scans := a.scanner.count(); scans != 0 {
		t.Errorf("%d scans started", scans)
	}
}

// The agent quits by itself after idleTime without a request of an accepted site, never under a
// scan or under the question.
func TestIdle(t *testing.T) {
	a := startAgent(t, map[string]bool{siteHome: true, siteLocal: false})
	longAgo := func() { a.server.last.Store(time.Now().Add(-idleTime - time.Second).UnixNano()) }

	if a.server.Idle() {
		t.Error("idle at the start")
	}
	longAgo()
	if !a.server.Idle() {
		t.Error("not idle after the time without requests")
	}
	// who is not let through does not keep the agent open
	a.call(t, "GET", "/status", "")
	a.call(t, "GET", "/status", siteLocal)
	a.call(t, "OPTIONS", "/status", siteHome)
	if !a.server.Idle() {
		t.Error("a call that was refused, or the one before the request, kept the agent open")
	}
	a.call(t, "GET", "/status", siteHome)
	if a.server.Idle() {
		t.Error("idle after a request of an accepted site")
	}

	longAgo()
	a.call(t, "GET", "/status", siteMoved)
	if a.server.Idle() {
		t.Error("idle with the question open")
	}
	a.person.answer(siteMoved, false)
	until(t, "the answer", func() bool { return a.call(t, "GET", "/status", siteMoved).reason() == "origin_denied" })

	a.call(t, "POST", "/scan", siteHome)
	longAgo()
	if a.server.Idle() {
		t.Error("idle during a scan")
	}
	a.scanner.finish(nil)
	until(t, "the end of the scan", func() bool { return !a.session.Scanning() })
	if !a.server.Idle() {
		t.Error("not idle after the scan and the time without requests")
	}
}

// A call refused as forbidden leaves a line in the log with what it carried: the names of the
// headers, never what they hold.
func TestRefusedCallsAreInTheLog(t *testing.T) {
	book := logged(t)
	a := startAgent(t, map[string]bool{siteHome: true})
	secret := func(request *http.Request) {
		request.Header.Set("Cookie", "session=a-secret")
		request.Header.Set("Authorization", "Bearer another-secret")
		request.Header.Set("User-Agent", "a browser")
	}

	a.call(t, "POST", "/scan", "null", secret)
	lines := book.lines("refused:")
	if len(lines) != 1 {
		t.Fatalf("%d lines, want 1: %q", len(lines), lines)
	}
	for _, part := range []string{`POST "/scan"`, `Host "` + Address + `"`, `Origin ["null"]`, `from "a browser"`, "Authorization", "Cookie", "Origin"} {
		if !strings.Contains(lines[0], part) {
			t.Errorf("the line does not say %s: %s", part, lines[0])
		}
	}
	for _, part := range []string{"a-secret", "another-secret", "Bearer"} {
		if strings.Contains(lines[0], part) {
			t.Errorf("the line says %s: %s", part, lines[0])
		}
	}

	// the same call again is not written again; a different one is
	a.call(t, "POST", "/scan", "null", secret)
	a.call(t, "GET", "/status", siteHome, func(request *http.Request) { request.Host = "agent.example:" + Port })
	if lines := book.lines("refused:"); len(lines) != 2 || !strings.Contains(lines[1], `Host "agent.example:`+Port+`"`) {
		t.Errorf("lines: %q", lines)
	}
	// what is let through, or refused by the owner, is not there
	a.call(t, "GET", "/status", siteHome)
	a.call(t, "OPTIONS", "/status", siteHome)
	if lines := book.lines("refused:"); len(lines) != 2 {
		t.Errorf("lines: %q", lines)
	}
}

// A page that keeps asking, each time something else, does not fill the log, and neither does a
// call with a text without end.
func TestLogOfRefusedCallsHasALimit(t *testing.T) {
	book := logged(t)
	a := startAgent(t, nil)

	a.call(t, "GET", "/"+strings.Repeat("a", 5000), "")
	lines := book.lines("refused:")
	if len(lines) != 1 || len(lines[0]) > 1500 || !strings.Contains(lines[0], strings.Repeat("a", 99)+`..."`) {
		t.Fatalf("a long call: %q", lines)
	}
	for i := 0; i < 2*refusedKept; i++ {
		a.call(t, "GET", "/status", "", func(request *http.Request) {
			request.Header.Set("User-Agent", strings.Repeat("x", i+1))
		})
	}
	if lines := book.lines("refused:"); len(lines) != refusedKept {
		t.Errorf("%d lines, want %d", len(lines), refusedKept)
	}
}

// fields are the fields of an answer, whatever they hold.
func (r reply) fields(t *testing.T) map[string]any {
	t.Helper()
	fields := map[string]any{}
	if err := json.Unmarshal(r.body, &fields); err != nil {
		t.Fatalf("%s: %v", r.body, err)
	}
	return fields
}

// asked is the scanner and the source of every status the scanner of the test was asked.
func (a *agent) asked() []string {
	a.scanner.mu.Lock()
	defer a.scanner.mu.Unlock()
	return append([]string{}, a.scanner.opened...)
}

var twoScanners = []Scanner{
	{ID: "one", Name: "The one with the feeder", Connection: ConnectionUSB},
	{ID: "two", Name: "The one with the glass", Connection: ConnectionNetwork},
}

var feederOnly = &Capabilities{Source: SourceFeeder, Sources: []string{SourceFeeder}, Resolutions: []int{150, 200, 300},
	Colours: []string{ColourColour, ColourGrey}}

// The list says the scanners the system sees, and opens none: nothing is asked of a scanner.
func TestScannersListsWithoutOpeningAny(t *testing.T) {
	a := startAgent(t, map[string]bool{siteHome: true})
	if got := a.call(t, "GET", "/scanners", siteHome); got.status != http.StatusOK || strings.TrimSpace(string(got.body)) != `{"scanners":[]}` {
		t.Errorf("with no scanner: %d %s", got.status, got.body)
	}
	a.scanner.mu.Lock()
	a.scanner.list = twoScanners
	a.scanner.mu.Unlock()

	got := a.call(t, "GET", "/scanners", siteHome)
	want := `{"scanners":[{"id":"one","name":"The one with the feeder","connection":"usb"},` +
		`{"id":"two","name":"The one with the glass","connection":"network"}]}`
	if strings.TrimSpace(string(got.body)) != want {
		t.Errorf("the list: %s, want %s", got.body, want)
	}
	if asked := a.asked(); len(asked) != 0 {
		t.Errorf("a scanner was opened for the list: %v", asked)
	}
}

// During a scan the scanner is busy: the list is the last one, without asking again.
func TestScannersDuringAScanIsTheLastList(t *testing.T) {
	a := startAgent(t, map[string]bool{siteHome: true})
	a.scanner.list = twoScanners
	a.call(t, "GET", "/scanners", siteHome)
	if got := a.call(t, "POST", "/scan", siteHome); got.status != http.StatusAccepted {
		t.Fatalf("POST /scan: %d %s", got.status, got.body)
	}
	got := a.call(t, "GET", "/scanners", siteHome)
	if list, _ := got.fields(t)["scanners"].([]any); len(list) != 2 {
		t.Errorf("during the scan: %s", got.body)
	}
	a.scanner.mu.Lock()
	listed := a.scanner.listed
	a.scanner.mu.Unlock()
	if listed != 1 {
		t.Errorf("the list was asked %d times, want 1", listed)
	}
	a.scanner.finish(nil)
	until(t, "the end of the scan", func() bool { return !a.session.Scanning() })
}

// The page that chooses the scanner sends api=2. Until it has chosen one, none is opened.
func TestStatusWithoutAChosenScannerOpensNone(t *testing.T) {
	a := startAgent(t, map[string]bool{siteHome: true})
	a.scanner.has = feederOnly
	got := a.call(t, "GET", "/status?api=2", siteHome).fields(t)
	if got["api"] != float64(API) || got["scanner"] != ScannerOff {
		t.Errorf("without a scanner: %v", got)
	}
	if _, there := got["resolutions"]; there {
		t.Errorf("without a scanner the answer says what a scanner has: %v", got)
	}
	if asked := a.asked(); len(asked) != 0 {
		t.Errorf("a scanner was opened: %v", asked)
	}
}

func TestStatusOfTheChosenScannerSaysWhatItHas(t *testing.T) {
	runAs(t, "1.2.3")
	a := startAgent(t, map[string]bool{siteHome: true})
	a.scanner.has = feederOnly
	a.scanner.status = ScannerEmpty

	got := a.call(t, "GET", "/status?api=2&scanner=one", siteHome).fields(t)
	want := map[string]any{"version": "1.2.3", "os": runtime.GOOS, "api": float64(API), "scanner": ScannerEmpty,
		"source": SourceFeeder, "sources": []any{SourceFeeder}, "resolutions": []any{150.0, 200.0, 300.0},
		"colours": []any{ColourColour, ColourGrey}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("the chosen scanner: %v, want %v", got, want)
	}
	a.call(t, "GET", "/status?api=2&scanner=two&source=flatbed", siteHome)
	if asked := a.asked(); !reflect.DeepEqual(asked, []string{"one/", "two/flatbed"}) {
		t.Errorf("the scanner was asked for %v", asked)
	}
}

// While the scanner asked opens, and when it is off, the answer says only how it is.
func TestStatusOfAScannerNotOpenSaysOnlyHowItIs(t *testing.T) {
	a := startAgent(t, map[string]bool{siteHome: true})
	a.scanner.has = feederOnly
	for _, state := range []string{ScannerOpening, ScannerOff} {
		a.scanner.mu.Lock()
		a.scanner.status = state
		a.scanner.mu.Unlock()
		got := a.call(t, "GET", "/status?api=2&scanner=one", siteHome).fields(t)
		if got["scanner"] != state || got["api"] != float64(API) {
			t.Errorf("scanner %s: %v", state, got)
		}
		if _, there := got["sources"]; there {
			t.Errorf("scanner %s: the answer says what it has: %v", state, got)
		}
	}
}

func TestStatusRefusesASourceThatDoesNotExist(t *testing.T) {
	a := startAgent(t, map[string]bool{siteHome: true})
	if got := a.call(t, "GET", "/status?api=2&scanner=one&source=tray", siteHome); got.status != http.StatusBadRequest || got.reason() != "settings" {
		t.Errorf("an unknown source: %d %s", got.status, got.body)
	}
	if asked := a.asked(); len(asked) != 0 {
		t.Errorf("the scanner was asked: %v", asked)
	}
}

// The page of before sends nothing: it gets the answer it always got, of the first scanner on the
// cable with a feeder, whatever its address carries besides.
func TestStatusOfThePageOfBeforeChoosesNoScanner(t *testing.T) {
	a := startAgent(t, map[string]bool{siteHome: true})
	a.scanner.has = feederOnly
	got := a.call(t, "GET", "/status?scanner=one&source=flatbed", siteHome).fields(t)
	if len(got) != 3 || got["scanner"] != ScannerReady {
		t.Errorf("the page of before: %v", got)
	}
	if asked := a.asked(); !reflect.DeepEqual(asked, []string{"/"}) {
		t.Errorf("the scanner was asked for %v, want the one of the page of before", asked)
	}
}

func TestScanWithTheSettingsOfThePage(t *testing.T) {
	a := startAgent(t, map[string]bool{siteHome: true})
	a.scanner.has = feederOnly
	if got := a.call(t, "POST", "/scan?api=2&scanner=one&source=feeder&resolution=150&colour=grey", siteHome); got.status != http.StatusAccepted {
		t.Fatalf("POST /scan: %d %s", got.status, got.body)
	}
	a.scanner.finish(nil)
	until(t, "the end of the scan", func() bool { return !a.session.Scanning() })
	// the page of before, with settings in its address that it never sent
	if got := a.call(t, "POST", "/scan?scanner=one&resolution=300", siteHome); got.status != http.StatusAccepted {
		t.Fatalf("POST /scan of the page of before: %d %s", got.status, got.body)
	}
	a.scanner.finish(nil)
	until(t, "the end of the scan", func() bool { return !a.session.Scanning() })

	a.scanner.mu.Lock()
	defer a.scanner.mu.Unlock()
	want := []Settings{{Scanner: "one", Source: SourceFeeder, Resolution: 150, Colour: ColourGrey}, Before}
	if !reflect.DeepEqual(a.scanner.settings, want) {
		t.Errorf("the scanner got %+v, want %+v", a.scanner.settings, want)
	}
}

// Settings the scanner does not have, or that are not settings, start no scan.
func TestScanRefusesSettingsTheScannerDoesNotHave(t *testing.T) {
	a := startAgent(t, map[string]bool{siteHome: true})
	a.scanner.has = feederOnly
	for name, query := range map[string]string{
		"no scanner":                    "source=feeder&resolution=150&colour=grey",
		"no resolution":                 "scanner=one&source=feeder&colour=grey",
		"a resolution it does not have": "scanner=one&source=feeder&resolution=175&colour=grey",
		"a source it does not have":     "scanner=one&source=flatbed&resolution=150&colour=grey",
		"a source that does not exist":  "scanner=one&source=tray&resolution=150&colour=grey",
		"a colour that does not exist":  "scanner=one&source=feeder&resolution=150&colour=blue",
	} {
		if got := a.call(t, "POST", "/scan?api=2&"+query, siteHome); got.status != http.StatusBadRequest || got.reason() != "settings" {
			t.Errorf("%s: %d %s", name, got.status, got.body)
		}
	}
	if _, scans := a.scanner.count(); scans != 0 {
		t.Errorf("%d scans started", scans)
	}
	if a.session.Scanning() {
		t.Error("a scan is running")
	}
}

// What a scanner that is off has is not known: the scan starts, and says itself that it is off.
func TestScanOfAScannerThatIsOffStartsAndSaysSo(t *testing.T) {
	a := startAgent(t, map[string]bool{siteHome: true})
	a.scanner.has = feederOnly
	a.scanner.status = ScannerOff
	if got := a.call(t, "POST", "/scan?api=2&scanner=one&source=feeder&resolution=1200&colour=grey", siteHome); got.status != http.StatusAccepted {
		t.Fatalf("POST /scan: %d %s", got.status, got.body)
	}
	a.scanner.finish(&ScanError{Code: ErrorOff})
	until(t, "the end of the scan", func() bool { return !a.session.Scanning() })
	if got := a.call(t, "GET", "/pages", siteHome); got.text("error") != ErrorOff {
		t.Errorf("GET /pages: %s", got.body)
	}
}
