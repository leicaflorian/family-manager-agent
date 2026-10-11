#!/usr/bin/env bash
# Publishes the version written in VERSION: builds the agent, tags the commit vX.Y.Z, pushes, and
# makes the release on GitHub with the two zips and version.json.
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

./build.sh

git tag "$TAG"
git push origin main "$TAG"
gh release create "$TAG" "${FILES[@]}" --title "Family Manager Agent $VERSION" --generate-notes

echo
echo "Published. In Family Manager, scripts/fetch-agent.mjs: the version $VERSION and"
shasum -a 256 "${FILES[@]}" | sed 's|dist/||'
