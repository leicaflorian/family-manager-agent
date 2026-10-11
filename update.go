package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Where the published versions are: the releases of the agent's own repository, each with the two
// zips and version.json. It is the only address the agent calls, and it sends nothing but the
// request. A test build points it at another server (-ldflags "-X main.updateSource=...").
var updateSource = "https://github.com/leicaflorian/family-manager-agent/releases"

const (
	// the first check waits for the scanner to open; an agent kept open checks again once a day
	updateDelay = 15 * time.Second
	updateEvery = 24 * time.Hour
	// more than this is not a file of the agent: the zips are about 3 MB
	manifestLimit = 64 << 10
	zipLimit      = 64 << 20
)

// What version.json of a release says: the version, the name of the zip of each system ("mac",
// "windows") and its fingerprint. A release without fingerprints is never installed.
type manifest struct {
	Version string            `json:"version"`
	Files   map[string]string `json:"files"`
	SHA256  map[string]string `json:"sha256"`
}

// What a check found.
const (
	updateNone  = iota // this is the last version
	updateLater        // a new first number: that version comes with the page, not by itself
	updateReady        // a newer version is downloaded, and waits to be installed
)

// errFixed is the error of a copy of the agent that cannot be replaced: see updateTarget.
var errFixed = errors.New("this copy cannot be replaced (not in its folder): no check")

// Updater keeps the agent at the last published version: while the agent is open it downloads the
// zip of a newer version and checks its fingerprint, and when the agent quits it puts the new
// program in the place of the old one. The next start is the new version: nothing is replaced
// under a scan, and nothing asks the owner. The owner can also ask for it from the menu (updateNow
// in main.go): the same check, and the new version at once.
type Updater struct {
	checking sync.Mutex // one check at a time: the one of Run and the one asked from the menu
	mu       sync.Mutex
	version  string // the version waiting in the zip, "" when there is none
	zip      string
	found    func(version string)
	// where the releases are, the folder the zip waits in and the program to replace: the ones of
	// this agent, others in a test
	source string
	dir    string
	target func() string
}

func NewUpdater(found func(version string)) *Updater {
	return &Updater{found: found, source: updateSource, dir: updateDir(), target: updateTarget}
}

func updateDir() string {
	base, err := os.UserCacheDir()
	if err != nil {
		base = os.TempDir()
	}
	return filepath.Join(base, "FamilyManagerAgent", "update")
}

// systemName is the name of this system in version.json.
func systemName() string {
	if runtime.GOOS == "darwin" {
		return "mac"
	}
	return runtime.GOOS
}

// Run checks now and then, until the agent quits.
func (u *Updater) Run() {
	time.Sleep(updateDelay)
	for {
		if _, _, err := u.Check(); err != nil {
			logf("update: %v", err)
		}
		time.Sleep(updateEvery)
	}
}

// Check asks the releases for a newer version and downloads it: what it found, and the published
// version when it is not this one.
func (u *Updater) Check() (int, string, error) {
	u.checking.Lock()
	defer u.checking.Unlock()
	if u.target() == "" {
		return updateNone, "", errFixed
	}
	var latest manifest
	body, err := fetch(u.source+"/latest/download/version.json", manifestLimit)
	if err != nil {
		return updateNone, "", err
	}
	if err := json.Unmarshal(body, &latest); err != nil {
		return updateNone, "", fmt.Errorf("version.json: %v", err)
	}
	current, published := numbers(Version), numbers(latest.Version)
	if current == nil || published == nil {
		return updateNone, "", fmt.Errorf("versions not understood: %q here, %q published", Version, latest.Version)
	}
	if !above(published, current) {
		logf("update: %s is the last version", Version)
		return updateNone, "", nil
	}
	// a new first number means the page of before cannot talk to that agent: it comes with the page
	if published[0] != current[0] {
		logf("update: %s is published, not installed by itself: it needs the new page", latest.Version)
		return updateLater, latest.Version, nil
	}
	if err := u.download(latest); err != nil {
		return updateNone, latest.Version, err
	}
	logf("update: %s downloaded, installed when the agent quits", latest.Version)
	if u.found != nil {
		u.found(latest.Version)
	}
	return updateReady, latest.Version, nil
}

// download keeps the zip of a published version in the update folder, once its fingerprint is the
// published one, and remembers it as the version that waits.
func (u *Updater) download(latest manifest) error {
	name, fingerprint := latest.Files[systemName()], strings.ToLower(latest.SHA256[systemName()])
	if name == "" || name != filepath.Base(name) || !strings.HasSuffix(name, ".zip") {
		return fmt.Errorf("%s: no zip for %s", latest.Version, systemName())
	}
	if len(fingerprint) != sha256.Size*2 {
		return fmt.Errorf("%s: no fingerprint for %s, not installed", latest.Version, systemName())
	}

	dir := u.dir
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	zip := filepath.Join(dir, latest.Version+"-"+name)
	// what is left of other versions, and of a download cut short
	if entries, err := os.ReadDir(dir); err == nil {
		for _, entry := range entries {
			if entry.Name() != filepath.Base(zip) {
				os.RemoveAll(filepath.Join(dir, entry.Name()))
			}
		}
	}
	if fingerprintOf(zip) != fingerprint {
		content, err := fetch(u.source+"/download/v"+latest.Version+"/"+name, zipLimit)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(content)
		if hex.EncodeToString(sum[:]) != fingerprint {
			return fmt.Errorf("%s: the zip downloaded is not the one that was published", latest.Version)
		}
		// written beside and renamed: a download cut short never leaves half a file with the right name
		if err := os.WriteFile(zip+".part", content, 0o600); err != nil {
			return err
		}
		if err := os.Rename(zip+".part", zip); err != nil {
			return err
		}
	}

	u.mu.Lock()
	u.version, u.zip = latest.Version, zip
	u.mu.Unlock()
	return nil
}

// Install puts the downloaded version in the place of this one: the version, "" when none was
// waiting. Called when the agent quits, and before a restart asked from the menu. A version that
// does not get there is tried again from the check of the next start: its zip stays.
func (u *Updater) Install() (string, error) {
	u.mu.Lock()
	version, zip := u.version, u.zip
	u.version, u.zip = "", ""
	u.mu.Unlock()
	if version == "" {
		return "", nil
	}
	target := u.target()
	if err := replaceProgram(zip, target); err != nil {
		logf("update: %s not installed: %v", version, err)
		return version, err
	}
	os.Remove(zip)
	logf("update: %s installed in %s", version, target)
	return version, nil
}

func fetch(address string, limit int64) ([]byte, error) {
	secure := strings.HasPrefix(address, "https://")
	client := &http.Client{
		Timeout: 5 * time.Minute,
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if len(via) >= 5 || (secure && request.URL.Scheme != "https") {
				return errors.New("redirect refused")
			}
			return nil
		},
	}
	request, err := http.NewRequest(http.MethodGet, address, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", "FamilyManagerAgent/"+Version)
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s answered %d", address, response.StatusCode)
	}
	content, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(content)) > limit {
		return nil, fmt.Errorf("%s is larger than expected", address)
	}
	return content, nil
}

// fingerprintOf is the sha256 of a file, "" when it cannot be read.
func fingerprintOf(path string) string {
	content, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

// numbers reads "X.Y.Z"; nil for anything else.
func numbers(version string) []int {
	parts := strings.Split(version, ".")
	if len(parts) != 3 {
		return nil
	}
	found := make([]int, 3)
	for i, part := range parts {
		value, err := strconv.Atoi(part)
		if err != nil || value < 0 {
			return nil
		}
		found[i] = value
	}
	return found
}

func above(a, b []int) bool {
	for i := range a {
		if a[i] != b[i] {
			return a[i] > b[i]
		}
	}
	return false
}
