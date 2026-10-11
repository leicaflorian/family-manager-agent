package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
)

// writeApp makes a folder shaped like the app, with a text in the place of the program.
func writeApp(t *testing.T, app, program string) {
	t.Helper()
	path := filepath.Join(app, bundleProgram)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(program), 0o755); err != nil {
		t.Fatal(err)
	}
}

// zipOf packs an app as build.sh does.
func zipOf(t *testing.T, app string) []byte {
	t.Helper()
	zip := filepath.Join(t.TempDir(), "app.zip")
	if output, err := exec.Command("/usr/bin/ditto", "-c", "-k", "--keepParent", app, zip).CombinedOutput(); err != nil {
		t.Fatalf("ditto: %v: %s", err, output)
	}
	content, err := os.ReadFile(zip)
	if err != nil {
		t.Fatal(err)
	}
	return content
}

// installed is an app of version 1.2.3 in a folder of its own, and the updater of that app, with
// the zip given published as 1.2.4 and already downloaded.
func installed(t *testing.T, zip []byte) (app string, updater *Updater) {
	t.Helper()
	runAs(t, "1.2.3")
	// not the name build.sh gives it: the app keeps the name the owner gave it
	app = filepath.Join(t.TempDir(), "Agente di casa.app")
	writeApp(t, app, "1.2.3")
	source := newReleases(t)
	source.publish("1.2.4", zip)
	updater, _ = updaterFor(t, source)
	updater.target = func() string { return app }
	if state, _, err := updater.Check(); state != updateReady || err != nil {
		t.Fatalf("check: %d, %v", state, err)
	}
	return app, updater
}

// beside lists what is in the folder of the app.
func beside(t *testing.T, app string) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Dir(app))
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

func TestInstallPutsTheNewAppInThePlaceOfTheOldOne(t *testing.T) {
	fresh := filepath.Join(t.TempDir(), appName+".app")
	writeApp(t, fresh, "1.2.4")
	app, updater := installed(t, zipOf(t, fresh))

	if version, err := updater.Install(); version != "1.2.4" || err != nil {
		t.Fatalf("%q, %v", version, err)
	}
	if program, _ := os.ReadFile(filepath.Join(app, bundleProgram)); string(program) != "1.2.4" {
		t.Errorf("the program in the app is %q", program)
	}
	if left := beside(t, app); !reflect.DeepEqual(left, []string{filepath.Base(app)}) {
		t.Errorf("beside the app: %q", left)
	}
	if left := waiting(t, updater); len(left) != 0 {
		t.Errorf("still in the update folder: %q", left)
	}
	if version, err := updater.Install(); version != "" || err != nil {
		t.Errorf("a second time: %q, %v", version, err)
	}
}

// When the zip does not hold the app the old one stays, and the zip too: the next start tries again.
func TestInstallLeavesTheOldAppWhenTheZipIsWrong(t *testing.T) {
	another := filepath.Join(t.TempDir(), "Another.app")
	writeApp(t, another, "another")
	empty := filepath.Join(t.TempDir(), appName+".app")
	if err := os.MkdirAll(filepath.Join(empty, "Contents", "Resources"), 0o755); err != nil {
		t.Fatal(err)
	}
	cases := map[string][]byte{
		"not a zip":                   []byte("not a zip"),
		"another app":                 zipOf(t, another),
		"the app without its program": zipOf(t, empty),
	}
	for name, zip := range cases {
		app, updater := installed(t, zip)

		if version, err := updater.Install(); version != "1.2.4" || err == nil {
			t.Errorf("%s: %q, %v", name, version, err)
		}
		if program, _ := os.ReadFile(filepath.Join(app, bundleProgram)); string(program) != "1.2.3" {
			t.Errorf("%s: the program in the app is %q", name, program)
		}
		if left := beside(t, app); !reflect.DeepEqual(left, []string{filepath.Base(app)}) {
			t.Errorf("%s: beside the app: %q", name, left)
		}
		if left := waiting(t, updater); !reflect.DeepEqual(left, []string{"1.2.4-" + testZip}) {
			t.Errorf("%s: in the update folder: %q", name, left)
		}
	}
}
