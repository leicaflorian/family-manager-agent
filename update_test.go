package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
)

const testZip = "FamilyManagerAgent-test.zip"

// received is a request as it got to the releases.
type received struct {
	method, path, query string
	header              http.Header
}

// releases stands for the releases on GitHub: version.json of the last one, and the files of each.
type releases struct {
	*httptest.Server
	mu       sync.Mutex
	manifest []byte            // what latest/download/version.json answers, nil for a 404
	files    map[string][]byte // by path, like "/download/v1.2.4/FamilyManagerAgent-test.zip"
	calls    []received
}

func newReleases(t *testing.T) *releases {
	t.Helper()
	r := &releases{files: map[string][]byte{}}
	r.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.calls = append(r.calls, received{request.Method, request.URL.Path, request.URL.RawQuery, request.Header.Clone()})
		content := r.files[request.URL.Path]
		if request.URL.Path == "/latest/download/version.json" {
			content = r.manifest
		}
		if content == nil {
			http.NotFound(w, request)
			return
		}
		w.Write(content)
	}))
	t.Cleanup(r.Close)
	return r
}

// publish makes a version the last one, as release.sh does: the zip, and version.json with its
// fingerprint.
func (r *releases) publish(version string, zip []byte) {
	r.describe(manifest{
		Version: version,
		Files:   map[string]string{systemName(): testZip},
		SHA256:  map[string]string{systemName(): fingerprintOfBytes(zip)},
	})
	r.mu.Lock()
	defer r.mu.Unlock()
	r.files["/download/v"+version+"/"+testZip] = zip
}

// describe writes version.json alone, for the releases that are not as they should be.
func (r *releases) describe(latest manifest) {
	content, _ := json.Marshal(latest)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.manifest = content
}

// asked lists the paths requested, in order.
func (r *releases) asked() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	paths := []string{}
	for _, call := range r.calls {
		paths = append(paths, call.path)
	}
	return paths
}

func fingerprintOfBytes(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

// runAs makes the tests an agent of the version given.
func runAs(t *testing.T, version string) {
	t.Helper()
	before := Version
	Version = version
	t.Cleanup(func() { Version = before })
}

// found collects the versions an updater said it downloaded.
type found struct {
	mu       sync.Mutex
	versions []string
}

func (f *found) note(version string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.versions = append(f.versions, version)
}

// updaterFor is an updater that asks the releases of the test and keeps the zip in a folder of the
// test, not yet made. Its program is a path that Check only needs to be there.
func updaterFor(t *testing.T, source *releases) (*Updater, *found) {
	t.Helper()
	noted := &found{}
	return &Updater{
		found:  noted.note,
		source: source.URL,
		dir:    filepath.Join(t.TempDir(), "update"),
		target: func() string { return filepath.Join(t.TempDir(), "agent") },
	}, noted
}

// waiting lists what is in the update folder; nothing when the folder was never made.
func waiting(t *testing.T, updater *Updater) []string {
	t.Helper()
	entries, err := os.ReadDir(updater.dir)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	names := []string{}
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

func TestNumbers(t *testing.T) {
	cases := map[string][]int{
		"1.2.3":      {1, 2, 3},
		"0.0.0":      {0, 0, 0},
		"1.10.0":     {1, 10, 0},
		"":           nil,
		"1":          nil,
		"1.2":        nil,
		"1.2.3.4":    nil,
		"v1.2.3":     nil,
		"1.2.x":      nil,
		"1.2.":       nil,
		"1.-2.3":     nil,
		"1.2.3-beta": nil,
		"1.2.3\n":    nil,
		" 1.2.3":     nil,
	}
	for version, want := range cases {
		if got := numbers(version); !reflect.DeepEqual(got, want) {
			t.Errorf("numbers(%q) = %v, want %v", version, got, want)
		}
	}
}

func TestAbove(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"1.2.4", "1.2.3", true},
		{"1.3.0", "1.2.9", true},
		{"2.0.0", "1.9.9", true},
		{"1.10.0", "1.9.0", true}, // numbers, not text
		{"1.2.3", "1.2.3", false},
		{"1.2.3", "1.2.4", false},
		{"1.2.9", "1.3.0", false},
		{"1.9.9", "2.0.0", false},
	}
	for _, c := range cases {
		if got := above(numbers(c.a), numbers(c.b)); got != c.want {
			t.Errorf("above(%s, %s) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestFingerprintOf(t *testing.T) {
	path := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(path, []byte("abc"), 0o600); err != nil {
		t.Fatal(err)
	}
	// the sha256 of "abc", from the standard that defines it
	if got, want := fingerprintOf(path), "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"; got != want {
		t.Errorf("fingerprint %s, want %s", got, want)
	}
	if got := fingerprintOf(path + ".missing"); got != "" {
		t.Errorf("fingerprint of a missing file: %q", got)
	}
}

func TestCheckFindsNothingNewer(t *testing.T) {
	runAs(t, "1.2.3")
	for _, published := range []string{"1.2.3", "1.2.2", "1.1.9", "0.9.9"} {
		source := newReleases(t)
		source.publish(published, []byte("zip"))
		updater, noted := updaterFor(t, source)

		state, version, err := updater.Check()
		if state != updateNone || version != "" || err != nil {
			t.Errorf("%s published: %d, %q, %v", published, state, version, err)
		}
		if asked := source.asked(); !reflect.DeepEqual(asked, []string{"/latest/download/version.json"}) {
			t.Errorf("%s published: asked %q", published, asked)
		}
		if left := waiting(t, updater); len(left) != 0 || len(noted.versions) != 0 {
			t.Errorf("%s published: kept %q, said %q", published, left, noted.versions)
		}
	}
}

// A new first number comes with the page: it is said, not downloaded.
func TestCheckLeavesANewFirstNumberToThePage(t *testing.T) {
	runAs(t, "1.2.3")
	source := newReleases(t)
	source.publish("2.0.0", []byte("zip"))
	updater, noted := updaterFor(t, source)

	state, version, err := updater.Check()
	if state != updateLater || version != "2.0.0" || err != nil {
		t.Errorf("%d, %q, %v", state, version, err)
	}
	if asked := source.asked(); len(asked) != 1 {
		t.Errorf("asked %q", asked)
	}
	if left := waiting(t, updater); len(left) != 0 || len(noted.versions) != 0 {
		t.Errorf("kept %q, said %q", left, noted.versions)
	}
}

func TestCheckDownloadsANewerVersion(t *testing.T) {
	cases := []struct{ here, published string }{
		{"1.2.3", "1.2.4"},
		{"1.2.3", "1.3.0"},
		{"1.9.9", "1.10.0"},
	}
	for _, c := range cases {
		runAs(t, c.here)
		source := newReleases(t)
		source.publish(c.published, []byte("the zip of "+c.published))
		updater, noted := updaterFor(t, source)

		state, version, err := updater.Check()
		if state != updateReady || version != c.published || err != nil {
			t.Fatalf("%s here, %s published: %d, %q, %v", c.here, c.published, state, version, err)
		}
		want := []string{"/latest/download/version.json", "/download/v" + c.published + "/" + testZip}
		if asked := source.asked(); !reflect.DeepEqual(asked, want) {
			t.Errorf("%s published: asked %q, want %q", c.published, asked, want)
		}
		if !reflect.DeepEqual(noted.versions, []string{c.published}) {
			t.Errorf("%s published: said %q", c.published, noted.versions)
		}
		name := c.published + "-" + testZip
		if left := waiting(t, updater); !reflect.DeepEqual(left, []string{name}) {
			t.Fatalf("%s published: kept %q, want %q", c.published, left, name)
		}
		if content, _ := os.ReadFile(filepath.Join(updater.dir, name)); string(content) != "the zip of "+c.published {
			t.Errorf("%s published: the zip kept is %q", c.published, content)
		}
	}
}

func TestCheckDoesNotDownloadTheSameZipTwice(t *testing.T) {
	runAs(t, "1.2.3")
	source := newReleases(t)
	source.publish("1.2.4", []byte("zip"))
	updater, _ := updaterFor(t, source)

	for i := 0; i < 2; i++ {
		if state, _, err := updater.Check(); state != updateReady || err != nil {
			t.Fatalf("check %d: %d, %v", i+1, state, err)
		}
	}
	want := []string{"/latest/download/version.json", "/download/v1.2.4/" + testZip, "/latest/download/version.json"}
	if asked := source.asked(); !reflect.DeepEqual(asked, want) {
		t.Errorf("asked %q, want %q", asked, want)
	}
}

// A zip of the right name with something else inside (a file gone bad) is downloaded again.
func TestCheckDownloadsAgainAZipThatChanged(t *testing.T) {
	runAs(t, "1.2.3")
	source := newReleases(t)
	source.publish("1.2.4", []byte("zip"))
	updater, _ := updaterFor(t, source)
	zip := filepath.Join(updater.dir, "1.2.4-"+testZip)
	os.MkdirAll(updater.dir, 0o700)
	if err := os.WriteFile(zip, []byte("half a z"), 0o600); err != nil {
		t.Fatal(err)
	}

	if state, _, err := updater.Check(); state != updateReady || err != nil {
		t.Fatalf("%d, %v", state, err)
	}
	if content, _ := os.ReadFile(zip); string(content) != "zip" {
		t.Errorf("the zip kept is %q", content)
	}
}

func TestCheckClearsWhatOtherDownloadsLeft(t *testing.T) {
	runAs(t, "1.2.3")
	source := newReleases(t)
	source.publish("1.2.4", []byte("zip"))
	updater, _ := updaterFor(t, source)
	os.MkdirAll(filepath.Join(updater.dir, "a folder"), 0o700)
	for _, name := range []string{"1.2.2-" + testZip, "1.2.4-" + testZip + ".part"} {
		if err := os.WriteFile(filepath.Join(updater.dir, name), []byte("old"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	if state, _, err := updater.Check(); state != updateReady || err != nil {
		t.Fatalf("%d, %v", state, err)
	}
	if left := waiting(t, updater); !reflect.DeepEqual(left, []string{"1.2.4-" + testZip}) {
		t.Errorf("kept %q", left)
	}
}

// A release that is not as release.sh makes it is never kept, and so never installed.
func TestCheckRefusesAReleaseItCannotTrust(t *testing.T) {
	runAs(t, "1.2.3")
	zip := []byte("zip")
	good := fingerprintOfBytes(zip)
	system := systemName()
	cases := []struct {
		name     string
		latest   manifest
		download bool // the zip is asked for before the release is refused
	}{
		{"no fingerprint", manifest{Version: "1.2.4", Files: map[string]string{system: testZip}}, false},
		{"a fingerprint cut short", manifest{Version: "1.2.4", Files: map[string]string{system: testZip}, SHA256: map[string]string{system: good[:40]}}, false},
		{"the fingerprint of another system", manifest{Version: "1.2.4", Files: map[string]string{system: testZip}, SHA256: map[string]string{"another": good}}, false},
		{"the fingerprint of another zip", manifest{Version: "1.2.4", Files: map[string]string{system: testZip}, SHA256: map[string]string{system: fingerprintOfBytes([]byte("another"))}}, true},
		{"no zip for this system", manifest{Version: "1.2.4", Files: map[string]string{"another": testZip}, SHA256: map[string]string{system: good}}, false},
		{"a zip in a folder", manifest{Version: "1.2.4", Files: map[string]string{system: "folder/" + testZip}, SHA256: map[string]string{system: good}}, false},
		{"a zip outside the folder", manifest{Version: "1.2.4", Files: map[string]string{system: "../" + testZip}, SHA256: map[string]string{system: good}}, false},
		{"a file that is not a zip", manifest{Version: "1.2.4", Files: map[string]string{system: "agent.exe"}, SHA256: map[string]string{system: good}}, false},
	}
	for _, c := range cases {
		source := newReleases(t)
		source.publish("1.2.4", zip)
		source.describe(c.latest)
		updater, noted := updaterFor(t, source)

		state, version, err := updater.Check()
		if state != updateNone || version != "1.2.4" || err == nil {
			t.Errorf("%s: %d, %q, %v", c.name, state, version, err)
		}
		if asked := source.asked(); len(asked) != 1 && !c.download || len(asked) != 2 && c.download {
			t.Errorf("%s: asked %q", c.name, asked)
		}
		if left := waiting(t, updater); len(left) != 0 || len(noted.versions) != 0 {
			t.Errorf("%s: kept %q, said %q", c.name, left, noted.versions)
		}
		if installed, err := updater.Install(); installed != "" || err != nil {
			t.Errorf("%s: installed %q, %v", c.name, installed, err)
		}
	}
}

func TestCheckFailsWhenTheReleasesAnswerBadly(t *testing.T) {
	runAs(t, "1.2.3")
	cases := map[string][]byte{
		"nothing published":      nil,
		"not a version.json":     []byte("<html>"),
		"a version not in X.Y.Z": []byte(`{"version": "1.3"}`),
		"no version":             []byte(`{}`),
		"a version.json too big": []byte(`{"version": "1.2.4"}` + strings.Repeat(" ", manifestLimit)),
	}
	for name, content := range cases {
		source := newReleases(t)
		source.manifest = content
		updater, noted := updaterFor(t, source)

		state, version, err := updater.Check()
		if state != updateNone || version != "" || err == nil {
			t.Errorf("%s: %d, %q, %v", name, state, version, err)
		}
		if left := waiting(t, updater); len(left) != 0 || len(noted.versions) != 0 {
			t.Errorf("%s: kept %q, said %q", name, left, noted.versions)
		}
	}
}

func TestCheckFailsWhenTheZipIsNotThere(t *testing.T) {
	runAs(t, "1.2.3")
	source := newReleases(t)
	source.describe(manifest{
		Version: "1.2.4",
		Files:   map[string]string{systemName(): testZip},
		SHA256:  map[string]string{systemName(): fingerprintOfBytes([]byte("zip"))},
	})
	updater, noted := updaterFor(t, source)

	state, version, err := updater.Check()
	if state != updateNone || version != "1.2.4" || err == nil {
		t.Errorf("%d, %q, %v", state, version, err)
	}
	if left := waiting(t, updater); len(left) != 0 || len(noted.versions) != 0 {
		t.Errorf("kept %q, said %q", left, noted.versions)
	}
}

// A copy that cannot be replaced asks nothing: it would download a version it cannot install.
func TestCheckOfACopyThatCannotBeReplaced(t *testing.T) {
	runAs(t, "1.2.3")
	source := newReleases(t)
	source.publish("1.2.4", []byte("zip"))
	updater, _ := updaterFor(t, source)
	updater.target = func() string { return "" }

	if state, version, err := updater.Check(); state != updateNone || version != "" || !errors.Is(err, errFixed) {
		t.Errorf("%d, %q, %v", state, version, err)
	}
	if asked := source.asked(); len(asked) != 0 {
		t.Errorf("asked %q", asked)
	}
}

// The releases are the only address the agent calls, and the request says the name and the version
// of the agent: nothing of the computer or of who uses it.
func TestRequestsSayOnlyTheVersion(t *testing.T) {
	runAs(t, "1.2.3")
	source := newReleases(t)
	source.publish("1.2.4", []byte("zip"))
	updater, _ := updaterFor(t, source)
	if state, _, err := updater.Check(); state != updateReady || err != nil {
		t.Fatalf("%d, %v", state, err)
	}

	source.mu.Lock()
	defer source.mu.Unlock()
	if len(source.calls) != 2 {
		t.Fatalf("%d requests, want 2", len(source.calls))
	}
	for _, call := range source.calls {
		if call.method != http.MethodGet || call.query != "" {
			t.Errorf("%s %s?%s", call.method, call.path, call.query)
		}
		for name, values := range call.header {
			switch name {
			case "User-Agent":
				if !reflect.DeepEqual(values, []string{"FamilyManagerAgent/1.2.3"}) {
					t.Errorf("%s: User-Agent %q", call.path, values)
				}
			case "Accept-Encoding": // added by Go: how the answer may be packed
			default:
				t.Errorf("%s: the request carries %s: %q", call.path, name, values)
			}
		}
	}
}

// GitHub answers a download with the address of the file: the agent follows it, up to five times.
func TestFetchFollowsAFewRedirects(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/file":
			w.Write([]byte("content"))
		case "/moved":
			http.Redirect(w, request, server.URL+"/file", http.StatusFound)
		default:
			http.Redirect(w, request, server.URL+"/around", http.StatusFound)
		}
	}))
	defer server.Close()

	if content, err := fetch(server.URL+"/moved", 100); string(content) != "content" || err != nil {
		t.Errorf("a redirect: %q, %v", content, err)
	}
	if _, err := fetch(server.URL+"/around", 100); err == nil {
		t.Error("redirects without an end: no error")
	}
	if _, err := fetch(server.URL+"/file", 6); err == nil {
		t.Error("a file larger than the limit: no error")
	}
	if content, err := fetch(server.URL+"/file", 7); string(content) != "content" || err != nil {
		t.Errorf("a file as large as the limit: %q, %v", content, err)
	}
}

func TestInstallWithNothingDownloaded(t *testing.T) {
	updater, _ := updaterFor(t, newReleases(t))
	if version, err := updater.Install(); version != "" || err != nil {
		t.Errorf("%q, %v", version, err)
	}
}
