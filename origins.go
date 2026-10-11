package main

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"sync"
)

// What the agent knows of a site that calls it.
const (
	OriginAllowed = "allowed"
	OriginDenied  = "denied"
	OriginPending = "pending" // the question is on the screen, waiting for the answer
)

// Origins are the sites the owner answered about: true for yes, false for no. They are kept in a
// file in the settings folder of the user, so that the same agent serves the local Directus and
// the one in production without an address written in it.
type Origins struct {
	mu      sync.Mutex
	path    string
	answers map[string]bool
	asking  map[string]bool
	ask     func(origin string) bool
}

func configDir() string {
	base, err := os.UserConfigDir()
	if err != nil {
		base = os.TempDir()
	}
	dir := filepath.Join(base, "FamilyManagerAgent")
	os.MkdirAll(dir, 0o700)
	return dir
}

func LoadOrigins(ask func(origin string) bool) *Origins {
	origins := &Origins{
		path:    filepath.Join(configDir(), "origins.json"),
		answers: map[string]bool{},
		asking:  map[string]bool{},
		ask:     ask,
	}
	if content, err := os.ReadFile(origins.path); err == nil {
		json.Unmarshal(content, &origins.answers)
	}
	return origins
}

// validOrigin tells a site (http or https, with its host) from anything else a browser may send,
// like "null" for a page opened from a file.
func validOrigin(origin string) bool {
	parsed, err := url.Parse(origin)
	if err != nil {
		return false
	}
	return (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != "" &&
		parsed.Scheme+"://"+parsed.Host == origin
}

// Check answers at once. For a site never seen the question opens on the computer and the answer
// is OriginPending until the owner chooses: the page asks again meanwhile.
func (o *Origins) Check(origin string) string {
	o.mu.Lock()
	defer o.mu.Unlock()
	if allowed, known := o.answers[origin]; known {
		if allowed {
			return OriginAllowed
		}
		return OriginDenied
	}
	if !o.asking[origin] {
		o.asking[origin] = true
		go func() {
			allowed := o.ask(origin)
			o.mu.Lock()
			defer o.mu.Unlock()
			delete(o.asking, origin)
			o.answers[origin] = allowed
			o.save()
			logf("%s: allowed %v", origin, allowed)
		}()
	}
	return OriginPending
}

// Forget deletes every answer: each site will be asked about again.
func (o *Origins) Forget() {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.answers = map[string]bool{}
	o.save()
}

func (o *Origins) save() {
	content, err := json.MarshalIndent(o.answers, "", "  ")
	if err != nil {
		return
	}
	if err := os.WriteFile(o.path, content, 0o600); err != nil {
		logf("origins: %v", err)
	}
}
