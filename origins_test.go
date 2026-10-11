package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sync"
	"testing"
	"time"
)

// The sites of the tests. None is a real one, and no test depends on which they are: the site
// that is allowed is whatever the owner answered about, and it can change.
const (
	siteHome  = "https://family.example"
	siteMoved = "https://famiglia.example"
	siteLocal = "http://localhost:8056"
)

// owner stands for the person in front of the question: it notes every question the agent opens,
// and each one stays open until the test answers it.
type owner struct {
	mu      sync.Mutex
	asked   []string
	replies map[string]chan bool
}

func (o *owner) reply(origin string) chan bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.replies == nil {
		o.replies = map[string]chan bool{}
	}
	if o.replies[origin] == nil {
		o.replies[origin] = make(chan bool, 1)
	}
	return o.replies[origin]
}

func (o *owner) ask(origin string) bool {
	o.mu.Lock()
	o.asked = append(o.asked, origin)
	o.mu.Unlock()
	return <-o.reply(origin)
}

func (o *owner) answer(origin string, allowed bool) {
	o.reply(origin) <- allowed
}

func (o *owner) questions() []string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]string{}, o.asked...)
}

// until waits for something that happens in another goroutine, like the answer to a question.
func until(t *testing.T, what string, done func() bool) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(time.Millisecond) {
		if done() {
			return
		}
	}
	t.Fatalf("still waiting for %s", what)
}

// decide asks about a site never seen, answers the question, and waits for the answer to be kept.
func decide(t *testing.T, origins *Origins, person *owner, origin string, allowed bool) {
	t.Helper()
	if state := origins.Check(origin); state != OriginPending {
		t.Fatalf("%s: %s before the question, want %s", origin, state, OriginPending)
	}
	person.answer(origin, allowed)
	want := OriginDenied
	if allowed {
		want = OriginAllowed
	}
	until(t, "the answer about "+origin, func() bool { return origins.Check(origin) == want })
}

// kept reads the file of the answers as the next start of the agent will.
func kept(t *testing.T, path string) map[string]bool {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	answers := map[string]bool{}
	if err := json.Unmarshal(content, &answers); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return answers
}

func originsFile(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "origins.json")
}

func TestValidOrigin(t *testing.T) {
	cases := map[string]bool{
		siteHome:                     true,
		siteLocal:                    true,
		"http://127.0.0.1:8055":      true,
		"https://family.example:444": true,
		"":                           false,
		"null":                       false, // a page opened from a file
		"family.example":             false,
		"https://":                   false,
		"https://family.example/":    false,
		"https://family.example/a":   false,
		"https://family.example?a=b": false,
		"https://family.example#a":   false,
		"https://me@family.example":  false,
		"HTTPS://family.example":     false,
		"ftp://family.example":       false,
		"file:///Users/me/page.html": false,
		"chrome-extension://abcdefg": false,
		"https://family.example\n":   false,
	}
	for origin, want := range cases {
		if got := validOrigin(origin); got != want {
			t.Errorf("validOrigin(%q) = %v, want %v", origin, got, want)
		}
	}
}

func TestCheckAsksOnceAndKeepsTheAnswer(t *testing.T) {
	path := originsFile(t)
	person := &owner{}
	origins := loadOrigins(path, person.ask)

	// the page asks again while the question is open: no second question
	for i := 0; i < 5; i++ {
		if state := origins.Check(siteHome); state != OriginPending {
			t.Fatalf("with the question open: %s, want %s", state, OriginPending)
		}
	}
	until(t, "the question", func() bool { return len(person.questions()) > 0 })
	if _, err := os.Stat(path); err == nil {
		t.Error("the file is written before the answer")
	}

	person.answer(siteHome, true)
	until(t, "the answer", func() bool { return origins.Check(siteHome) == OriginAllowed })
	origins.Check(siteHome)
	if asked := person.questions(); !reflect.DeepEqual(asked, []string{siteHome}) {
		t.Errorf("questions: %q, want one about %s", asked, siteHome)
	}
	if answers := kept(t, path); !reflect.DeepEqual(answers, map[string]bool{siteHome: true}) {
		t.Errorf("kept: %v", answers)
	}
}

func TestCheckFromManyRequestsAsksOnce(t *testing.T) {
	person := &owner{}
	origins := loadOrigins(originsFile(t), person.ask)

	var requests sync.WaitGroup
	for i := 0; i < 50; i++ {
		requests.Add(1)
		go func() {
			defer requests.Done()
			origins.Check(siteHome)
		}()
	}
	requests.Wait()
	person.answer(siteHome, true)
	until(t, "the answer", func() bool { return origins.Check(siteHome) == OriginAllowed })
	if asked := person.questions(); len(asked) != 1 {
		t.Errorf("%d questions, want 1", len(asked))
	}
}

func TestRefusedSiteIsNotAskedAgain(t *testing.T) {
	path := originsFile(t)
	person := &owner{}
	origins := loadOrigins(path, person.ask)
	decide(t, origins, person, siteHome, false)

	if state := origins.Check(siteHome); state != OriginDenied {
		t.Errorf("%s, want %s", state, OriginDenied)
	}
	// the next start of the agent
	again := &owner{}
	if state := loadOrigins(path, again.ask).Check(siteHome); state != OriginDenied {
		t.Errorf("after a restart: %s, want %s", state, OriginDenied)
	}
	if asked := append(person.questions(), again.questions()...); len(asked) != 1 {
		t.Errorf("%d questions, want 1", len(asked))
	}
}

func TestAnswersSurviveARestart(t *testing.T) {
	path := originsFile(t)
	person := &owner{}
	origins := loadOrigins(path, person.ask)
	decide(t, origins, person, siteHome, true)
	decide(t, origins, person, siteLocal, false)

	again := &owner{}
	restarted := loadOrigins(path, again.ask)
	if state := restarted.Check(siteHome); state != OriginAllowed {
		t.Errorf("%s: %s, want %s", siteHome, state, OriginAllowed)
	}
	if state := restarted.Check(siteLocal); state != OriginDenied {
		t.Errorf("%s: %s, want %s", siteLocal, state, OriginDenied)
	}
	if asked := again.questions(); len(asked) != 0 {
		t.Errorf("asked again about %q", asked)
	}
}

// Each site has its own question, and one waiting does not hold the other.
func TestSitesAreAskedAboutSeparately(t *testing.T) {
	person := &owner{}
	origins := loadOrigins(originsFile(t), person.ask)
	origins.Check(siteHome)
	origins.Check(siteLocal)
	until(t, "the two questions", func() bool { return len(person.questions()) == 2 })

	person.answer(siteLocal, false)
	until(t, "the answer about "+siteLocal, func() bool { return origins.Check(siteLocal) == OriginDenied })
	if state := origins.Check(siteHome); state != OriginPending {
		t.Errorf("%s: %s, want %s", siteHome, state, OriginPending)
	}
	person.answer(siteHome, true)
	until(t, "the answer about "+siteHome, func() bool { return origins.Check(siteHome) == OriginAllowed })
}

// Family Manager moves to another address: for the agent it is a site never seen. The answer
// given to the old address is not passed on, and stays until the owner forgets the sites.
func TestSiteAtANewAddressIsAskedAbout(t *testing.T) {
	path := originsFile(t)
	person := &owner{}
	origins := loadOrigins(path, person.ask)
	decide(t, origins, person, siteHome, true)

	if state := origins.Check(siteMoved); state != OriginPending {
		t.Fatalf("%s: %s, want %s", siteMoved, state, OriginPending)
	}
	if state := origins.Check(siteHome); state != OriginAllowed {
		t.Errorf("%s while the new address waits: %s, want %s", siteHome, state, OriginAllowed)
	}
	person.answer(siteMoved, true)
	until(t, "the answer about "+siteMoved, func() bool { return origins.Check(siteMoved) == OriginAllowed })

	allowed, denied := origins.Answers()
	if want := []string{siteMoved, siteHome}; !reflect.DeepEqual(allowed, want) || len(denied) != 0 {
		t.Errorf("allowed %q, denied %q; want allowed %q", allowed, denied, want)
	}
	if answers := kept(t, path); !reflect.DeepEqual(answers, map[string]bool{siteHome: true, siteMoved: true}) {
		t.Errorf("kept: %v", answers)
	}
}

// The same host is another site with another scheme or another port.
func TestSchemeAndPortMakeAnotherSite(t *testing.T) {
	person := &owner{}
	origins := loadOrigins(originsFile(t), person.ask)
	decide(t, origins, person, siteHome, true)

	for _, other := range []string{"http://family.example", "https://family.example:8443", "https://www.family.example"} {
		if state := origins.Check(other); state != OriginPending {
			t.Errorf("%s: %s, want %s", other, state, OriginPending)
		}
		person.answer(other, false)
		until(t, "the answer about "+other, func() bool { return origins.Check(other) == OriginDenied })
	}
	if state := origins.Check(siteHome); state != OriginAllowed {
		t.Errorf("%s: %s, want %s", siteHome, state, OriginAllowed)
	}
}

// An answer changes only through Forget: the site is then asked about again, and the new answer
// takes the place of the old one, in the file too.
func TestAnswerChangesAfterForget(t *testing.T) {
	for _, first := range []bool{true, false} {
		path := originsFile(t)
		person := &owner{}
		origins := loadOrigins(path, person.ask)
		decide(t, origins, person, siteHome, first)

		origins.Forget()
		decide(t, origins, person, siteHome, !first)

		if asked := person.questions(); len(asked) != 2 {
			t.Errorf("first answer %v: %d questions, want 2", first, len(asked))
		}
		if answers := kept(t, path); !reflect.DeepEqual(answers, map[string]bool{siteHome: !first}) {
			t.Errorf("first answer %v: kept %v", first, answers)
		}
	}
}

func TestForgetDeletesEveryAnswer(t *testing.T) {
	path := originsFile(t)
	person := &owner{}
	origins := loadOrigins(path, person.ask)
	decide(t, origins, person, siteHome, true)
	decide(t, origins, person, siteLocal, false)

	origins.Forget()

	if allowed, denied := origins.Answers(); len(allowed) != 0 || len(denied) != 0 {
		t.Errorf("still allowed %q, denied %q", allowed, denied)
	}
	if answers := kept(t, path); len(answers) != 0 {
		t.Errorf("kept: %v", answers)
	}
	for _, origin := range []string{siteHome, siteLocal} {
		if state := origins.Check(origin); state != OriginPending {
			t.Errorf("%s: %s, want %s", origin, state, OriginPending)
		}
		person.answer(origin, true)
		until(t, "the answer about "+origin, func() bool { return origins.Check(origin) == OriginAllowed })
	}
}

func TestAnswersAreInAlphabeticalOrder(t *testing.T) {
	sites := map[string]bool{
		"https://c.example": true, "https://a.example": true, "https://b.example": true,
		"https://z.example": false, "http://y.example": false,
	}
	path := originsFile(t)
	content, _ := json.Marshal(sites)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	allowed, denied := loadOrigins(path, nil).Answers()
	if want := []string{"https://a.example", "https://b.example", "https://c.example"}; !reflect.DeepEqual(allowed, want) {
		t.Errorf("allowed %q, want %q", allowed, want)
	}
	if want := []string{"http://y.example", "https://z.example"}; !reflect.DeepEqual(denied, want) {
		t.Errorf("denied %q, want %q", denied, want)
	}
}

// The file is read at the start: a site written there by hand before opening the agent (a trial
// site) is not asked about, and what is written there the next time is what counts.
func TestFileWrittenByHandCountsAtTheNextStart(t *testing.T) {
	path := originsFile(t)
	for _, allowed := range []bool{true, false, true} {
		content, _ := json.Marshal(map[string]bool{siteLocal: allowed})
		if err := os.WriteFile(path, content, 0o600); err != nil {
			t.Fatal(err)
		}
		want := OriginDenied
		if allowed {
			want = OriginAllowed
		}
		person := &owner{}
		if state := loadOrigins(path, person.ask).Check(siteLocal); state != want {
			t.Errorf("written %v: %s, want %s", allowed, state, want)
		}
		if asked := person.questions(); len(asked) != 0 {
			t.Errorf("written %v: asked about %q", allowed, asked)
		}
	}
}

func TestMissingOrBrokenFileMeansNoAnswers(t *testing.T) {
	broken := originsFile(t)
	if err := os.WriteFile(broken, []byte(`{"https://family.example": tr`), 0o600); err != nil {
		t.Fatal(err)
	}
	for name, path := range map[string]string{"missing": originsFile(t), "broken": broken} {
		person := &owner{}
		origins := loadOrigins(path, person.ask)
		if allowed, denied := origins.Answers(); len(allowed) != 0 || len(denied) != 0 {
			t.Errorf("%s file: allowed %q, denied %q", name, allowed, denied)
		}
		decide(t, origins, person, siteHome, true)
		if answers := kept(t, path); !reflect.DeepEqual(answers, map[string]bool{siteHome: true}) {
			t.Errorf("%s file: kept %v", name, answers)
		}
	}
}

func TestFileIsReadableOnlyByTheUser(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no such permissions")
	}
	path := originsFile(t)
	person := &owner{}
	decide(t, loadOrigins(path, person.ask), person, siteHome, true)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("permissions %o, want 600", mode)
	}
}
