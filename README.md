# Family Manager Agent

The program that drives the scanner connected to a computer on behalf of the *Scansiona* (Scan) page of Family Manager. A browser cannot talk to a scanner driver; the agent can, with what the system already has: Image Capture on macOS, WIA on Windows.

It is a single Go program for **Macs** (Apple silicon and Intel, macOS 13 or later) and **64-bit Windows**. It does not run on the server: it is downloaded from the page and opened on the computer. Its interface is in Italian, so menu items and messages are quoted here as they appear.

## What it does

- Starts when the system opens `familymanageragent://open` (the page does it), never at login, and quits after ten minutes without requests.
- Answers on `http://127.0.0.1:47115`, only from the computer itself and only to the sites the owner has accepted: the first call from a site asks "Permetti a `<sito>` di usare lo scanner?" and the answer is remembered.
- Lists the scanners the computer sees, USB and network, without opening any. It opens only the one the page picks and lets go of the previous one: opening a network scanner wakes it up.
- Says what the chosen scanner has (feeder, flatbed, resolutions, colour modes) and scans with the choices of the page: every sheet in the feeder, both sides, or one side at a time from the flatbed. Always A4, one JPEG per side. A resolution the scanner refuses stops the scan with an error.
- Keeps the scans in a temporary folder, deleted on exit: nothing is left on the computer.
- Shows an icon while it runs (menu bar on macOS, notification area on Windows) with *Informazioni* and *Esci* (quit). *Informazioni* says which version is running and which sites were accepted or refused. *Cerca aggiornamenti* (check for updates) and *Dimentica i siti accettati* (forget the accepted sites) are buttons of that window on macOS, menu items on Windows.
- Updates itself from the releases of this repository ([Self-update](#self-update)).

It knows nothing about Family Manager: no address, no key, no token. The file is uploaded by the page. The only address it calls is the one of this repository's releases on GitHub, and that request carries nothing but the name and version of the agent.

## Install

The two zips are in the [releases](https://github.com/leicaflorian/family-manager-agent/releases), and the Scan page links to them. The programs are not signed, so the first time they must be approved by hand.

- **macOS**: open `FamilyManagerAgent-mac.zip`, move *Family Manager Agent* to *Applications* and open it. macOS refuses: *System Settings → Privacy & Security → Open Anyway*.
- **Windows**: open `FamilyManagerAgent-windows.zip`, then *Family Manager Agent*. On "Windows protected your PC": *More info → Run anyway*. The program copies itself to `%LOCALAPPDATA%\FamilyManagerAgent\` and says so in a window; the zip can be thrown away. If an agent is already running it asks whether to close it (a scan in progress is interrupted) and takes its place.

## Self-update

Since 1.1.0; from 1.0.0 the update is done by hand, one last time.

Fifteen seconds after it starts, and then once a day, the agent reads `version.json` of the latest release. If the published version is higher than its own, it downloads the zip for its system, checks its sha256 against the one in `version.json`, and puts the new program in place when it quits. It asks nothing and replaces nothing during a scan. *Cerca aggiornamenti* does the same right away and restarts the agent, and always answers with a window: already up to date, or why it did not install.

What it does not do:

- **Move to a higher major version by itself** (1.x to 2.0.0): the major changes when the previous page can no longer talk to the agent, so that version comes with the page.
- **Install a zip whose sha256 differs** from the published one, or a release without checksums (1.0.0).
- **Update when it cannot be replaced**: on macOS the app must be in a writable folder such as *Applications*. Opened from where it was downloaded, macOS runs it from a read-only copy.

What it did is in `agent.log`, in the lines starting with `update:`. The checksum protects against a zip broken or changed on the way, not against a release published by the wrong person: whoever can publish a release here decides what runs on the computers that have the agent.

## Build

On a Mac, with Go (`brew install go`) and the Xcode command line tools (`swift` included). The first build needs the network, for the Go modules and `go-winres`.

```bash
./build.sh         # the two zips and version.json, in dist/
./build.sh probe   # only the probes, in build/: scanner-probe and scanner-probe.exe
go test ./...      # only the tests
```

`build.sh` checks the code for both systems (`go vet`), runs the tests, builds `Family Manager Agent.app` (signed locally, without a certificate) and `Family Manager Agent.exe`. The Mac program is universal: built for Apple silicon and for Intel, and joined with `lipo`, in the same zip. Then it writes the two zips with `version.json`: the version, the names of the zips and their sha256. `build/` and `dist/` are not versioned.

**The probe** never feeds a sheet. Alone it lists the scanners with their ids and opens none; with an id (`build/scanner-probe "<id>"`) it opens that one and prints its sources, resolutions and colour modes. The output also goes to `scanner-probe.txt`, next to it.

**The tests** are the `_test.go` files, next to the code they cover: the accepted sites, the requests, the scan session and the update. A failing test stops the build, and so the release. They do not open the agent, touch a scanner or call the network: the scanner and the releases are fakes, the server listens on a port of its own, and the files live in temporary folders. They run on the Mac only, for Apple silicon and, through Rosetta, for Intel; the Windows code is compiled, not run.

**The icon** is `icon.png`, 512 px on a flat background: replace it and rebuild. `mac-icon.swift` redraws it in the shape macOS wants, and `go-winres` puts it in the Windows program. The menu bar icon on macOS is drawn by the code.

## Release

The versions are in [`CHANGELOG.md`](CHANGELOG.md), in Italian: changes are written there as the work goes, under *Non rilasciato*, and the numbering rules are at the top of that file.

1. `./close-version.py X.Y.Z` writes the number to `VERSION` and turns *Non rilasciato* into the section of that version, dated today.
2. Commit everything on `dev`, the development branch. `main` holds only the published versions.
3. `git checkout main && git merge --ff-only dev`
4. `./release.sh` builds, tags the commit `vX.Y.Z`, pushes `main` and creates the GitHub release with the two zips and `version.json`, with the changelog section as notes. It stops when not on `main`, with uncommitted changes, or when the version is already published or has no changelog section.
5. `git checkout dev`

It needs `gh` (`brew install gh`), logged in, and `python3`.

Nothing has to be done in Family Manager: within an hour the Scan page offers the new version. The agents take it at their next start, so a published version cannot be withdrawn: a fix is a new version.

## Requests

| Request | Answer |
| --- | --- |
| `GET /scanners` | `{ scanners: [{ id, name, connection }] }`, `connection` being `usb` or `network`. Opens no scanner; during a scan it answers with the last list |
| `GET /status?api=2&scanner=<id>` | `{ version, os, api, scanner }`, `scanner` being `ready`, `empty`, `off` or `opening`. Opens **only** the scanner asked for and lets go of the previous one. Once open, the answer also has `source` (`feeder` or `flatbed`: where it reads from now), `sources`, `resolutions` and `colours` (`colour`, `grey`). `&source=flatbed` moves it to the flatbed; without it, the feeder if there is one. Without `scanner` it opens none and answers `off` |
| `POST /scan?api=2&scanner=<id>&source=…&resolution=…&colour=…` | `202`: feeds the sheets, or reads the one on the flatbed, after the pages already scanned. `400 settings` if a choice is missing or the scanner does not have it. `409` during a scan |
| `GET /status`, `POST /scan` without `api=2` | The previous page: `{ version, os, scanner }`, and a scan from the first USB scanner with a feeder, both sides, colour, 200 dpi (on Windows, the resolution of the driver if it refuses 200) |
| `GET /pages` | `{ session, state, error, message, pages }`: `state` is `idle`, `scanning`, `done` or `error`; `error` is `off`, `empty`, `jam` or `scanner` |
| `GET /pages/<id>` | The JPEG of one side |
| `DELETE /pages` | `204`: deletes the pages. `409` during a scan |

Everything travels in the URL, without a body. For a page that sends `api=2` the agent never picks the scanner: until the page asks for one, none is opened.

Without the `Origin` header, or with a `Host` other than `127.0.0.1:47115`: `403 forbidden`. A site never seen: `403 origin_pending` and the question on the computer; a refused one: `403 origin_denied`.

Port `47115` and these requests are what ties the agent to the page: whoever changes them here changes them in the page too (`extensions/document-reader/src/module/scan/agent.ts` in Family Manager) and in the Directus security policy (`CONTENT_SECURITY_POLICY_DIRECTIVES__CONNECT_SRC`).

## Try it with curl

```bash
open "build/Family Manager Agent.app"                    # or: open "familymanageragent://open"
curl -i http://127.0.0.1:47115/status                    # 403: no calling site
curl -i -H 'Origin: http://localhost:8056' http://127.0.0.1:47115/status   # the first time: the question on the Mac
curl -s -H 'Origin: http://localhost:8056' http://127.0.0.1:47115/pages
curl -s -H 'Origin: http://localhost:8056' http://127.0.0.1:47115/scanners                       # the list: no scanner opened
curl -s -H 'Origin: http://localhost:8056' 'http://127.0.0.1:47115/status?api=2'                 # still none
curl -s -H 'Origin: http://localhost:8056' 'http://127.0.0.1:47115/status?api=2&scanner=<id>'    # opens that one: opening, then ready or empty
```

`POST /scan` feeds real sheets: try it with any sheet, and finish with `DELETE /pages`.

To skip the question, add the test site to `origins.json` before opening the agent. Do **not** change `HOME`: Image Capture stops seeing the scanner.

What the agent sees and does is in `agent.log`, in the settings folder (`~/Library/Application Support/FamilyManagerAgent/` on macOS, `%AppData%\FamilyManagerAgent\` on Windows), next to `origins.json`. A call refused with `forbidden` leaves a `refused:` line: the first thing to read when the page says the agent does not answer and the agent is running. On Windows every step of a scan leaves a `scan:` line, so the last one says where a scan stopped.

On Windows, without the page: `test-windows.cmd`, copied to the PC next to the program. It starts the agent, feeds the sheets, downloads the pages as `pagina-1.jpg`… and writes `risultato.txt`.

## Files

| File | What it holds |
| --- | --- |
| `main.go` | Start, the icon, quitting by itself, the cleanup on exit |
| `server.go` | The answers, and `guard`, which decides who gets through |
| `origins.go` | The accepted sites |
| `update.go`, `update_darwin.go`, `update_windows.go` | The update: the check, the zip and its checksum, replacing the program on exit |
| `scanner.go` | The common part of the scanner: `Driver`, `Session`, the fixed settings |
| `scanner_darwin.go`, `scanner_darwin.m`, `agent_darwin.h` | The scanner on macOS (`ImageCaptureCore`) |
| `scanner_windows.go` | The scanner on Windows (WIA, with `go-ole`): written on the Mac, where it only compiles, and tried on the PC |
| `dialog_*.go`, `dialog_darwin.m` | The question and the messages |
| `install_windows.go`, `install_darwin.go` | On Windows: the copy to `%LOCALAPPDATA%`, the link in the registry, closing an agent already running |
| `icon.go`, `icon.png`, `mac-icon.swift` | The icons |
| `probe.go` | The probe: what the system sees of the scanner |
| `*_test.go` | The tests (see [Build](#build)) |
| `Info.plist` | The macOS bundle: identifier `it.familymanager.agent`, the URL scheme, no Dock icon |
| `build.sh`, `release.sh`, `close-version.py`, `VERSION`, `CHANGELOG.md` | Building, releasing, the version number and what each version holds |
| `test-windows.cmd` | The test on Windows without the page |
| `CLAUDE.md`, `AGENTS.md`, `.claude/` | The rules for whoever works here, person or agent (the two files are identical), and the Claude Code skills |

## Still to test

Tried by the owner: the scan from the page on macOS (an Intel Mac included, and Safari) and on Windows, where 1.2.1 works as a whole.

- On Windows, what 1.2.1 does not have: the choices of the page (another scanner, resolution, grey, the flatbed), network scanners left alone, the scan area that follows the resolution of the driver, the `scan:` lines of the log.
- Restarting after *Cerca aggiornamenti* with the *Informazioni* window open.
- The update with the app in *Applications*: it was tried from another folder, and macOS can stop a program from modifying apps (*Privacy & Security → App Management*).
