#!/usr/bin/env bash
# Publishes the version written in VERSION: builds the agent, tags the commit vX.Y.Z, pushes, and
# makes the release on GitHub with the two zips and version.json. The notes of the release are the
# section of the version in CHANGELOG.md, which close-version.py writes.
#
#   ./release.sh
#
# Needs what build.sh needs, and gh (brew install gh) logged in. Everything must be committed, on
# main: the tag goes on the commit the zips were built from. At the end it prints the fingerprints
# of the three files, to write in scripts/fetch-agent.mjs of Family Manager, which downloads them.
set -euo pipefail
cd "$(dirname "$0")"

VERSION="$(tr -d '[:space:]' < VERSION)"
TAG="v$VERSION"
FILES=(dist/FamilyManagerAgent-mac.zip dist/FamilyManagerAgent-windows.zip dist/version.json)

stop() { echo "$1" >&2; exit 1; }

[ "$(git branch --show-current)" = main ] || stop "Not on main."
[ -z "$(git status --porcelain)" ] || stop "There are changes not committed: commit them first."
if git rev-parse -q --verify "refs/tags/$TAG" > /dev/null; then
    stop "The version $VERSION is already published: raise the number in VERSION."
fi

# the section "## [X.Y.Z] - date" of the changelog, up to the next section or the list of links
NOTES="$(awk -v v="$VERSION" '
    index($0, "## [" v "]") == 1 { on = 1; next }
    on && (/^## \[/ || /^\[[^]]+\]: /) { exit }
    on { print }
' CHANGELOG.md)"
[ -n "${NOTES//[[:space:]]/}" ] || stop "No section [$VERSION] in CHANGELOG.md: close the version first, ./close-version.py $VERSION"

./build.sh

git tag "$TAG"
git push origin main "$TAG"
printf '%s\n' "$NOTES" | gh release create "$TAG" "${FILES[@]}" --title "Family Manager Agent $VERSION" --notes-file -

echo
echo "Published. In Family Manager, scripts/fetch-agent.mjs: the version $VERSION and"
shasum -a 256 "${FILES[@]}" | sed 's|dist/||'
