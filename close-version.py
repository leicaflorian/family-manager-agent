#!/usr/bin/env python3
"""Closes a version: writes the number in VERSION, and in CHANGELOG.md [Non rilasciato] becomes
[X.Y.Z] with the date of today, under a new empty [Non rilasciato].

    ./close-version.py X.Y.Z

Run on dev, before the commit release.sh publishes. It does not commit, tag or push. The number is
chosen by the owner; release.sh stops when CHANGELOG.md has no section for the version in VERSION.
"""
import datetime
import re
import sys
from pathlib import Path

HERE = Path(__file__).resolve().parent
CHANGELOG = HERE / "CHANGELOG.md"
VERSION = HERE / "VERSION"
HEADING = "## [Non rilasciato]"
EMPTY = "Niente ancora."


def number(version):
    return tuple(int(part) for part in version.split("."))


if len(sys.argv) != 2 or not re.fullmatch(r"\d+\.\d+\.\d+", sys.argv[1]):
    sys.exit("Usage: ./close-version.py X.Y.Z")
version = sys.argv[1]
lines = CHANGELOG.read_text(encoding="utf-8").split("\n")

if HEADING not in lines:
    sys.exit(f"No section {HEADING} in CHANGELOG.md")
start = lines.index(HEADING)
end = next((i for i in range(start + 1, len(lines)) if lines[i].startswith(("## [", "["))), len(lines))
body = "\n".join(lines[start + 1:end]).strip("\n")
if not body.strip() or body.strip() == EMPTY:
    sys.exit("Nothing in [Non rilasciato]: nothing to publish")

# the last published version is the one the link of [Non rilasciato] compares with
link = next((i for i, line in enumerate(lines) if line.startswith("[Non rilasciato]: ")), None)
found = re.fullmatch(r"\[Non rilasciato\]: (.+)/compare/v(\d+\.\d+\.\d+)\.\.\.HEAD", lines[link]) if link is not None else None
if not found:
    sys.exit("The link [Non rilasciato] does not compare vX.Y.Z...HEAD: fix the links at the end of CHANGELOG.md")
base, previous = found.groups()
if number(version) <= number(previous):
    sys.exit(f"{version} is not above {previous}, the last published version")

date = datetime.date.today().isoformat()
lines[link:link + 1] = [
    f"[Non rilasciato]: {base}/compare/v{version}...HEAD",
    f"[{version}]: {base}/compare/v{previous}...v{version}",
]
lines[start:end] = [HEADING, "", EMPTY, "", f"## [{version}] - {date}", "", *re.sub(r"\n{3,}", "\n\n", body).split("\n"), ""]
CHANGELOG.write_text("\n".join(lines), encoding="utf-8")
VERSION.write_text(version + "\n", encoding="utf-8")
print(f"VERSION: {version}. CHANGELOG.md: [{version}] - {date}, after {previous}")
