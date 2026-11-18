#!/bin/sh
# Tags a released aicoded: on top of the framework tag <version>, a commit whose cmd/aicoded/go.mod
# requires aicoded.dev/framework <version> with no replace, tagged cmd/aicoded/<version>, so that
# go install aicoded.dev/framework/cmd/aicoded@<version> builds an aicoded that creates apps on
# that version. The framework tag must already be pushed and on the Go proxy. Pushes nothing.
set -eu

version=${1:-}
if ! printf '%s\n' "$version" | grep -Eq '^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$'; then
	echo "usage: scripts/release-cli.sh v<major>.<minor>.<patch>, such as v0.11.0" >&2
	exit 2
fi

root=$(git rev-parse --show-toplevel)
cd "$root"
cli_tag=cmd/aicoded/$version
if ! git rev-parse -q --verify "refs/tags/$version" >/dev/null; then
	echo "release-cli.sh: no tag $version: release the framework first" >&2
	exit 1
fi
if git rev-parse -q --verify "refs/tags/$cli_tag" >/dev/null; then
	echo "release-cli.sh: tag $cli_tag exists already" >&2
	exit 1
fi

export GOWORK=off GOFLAGS=-mod=mod
proxied=$(go mod download -json "aicoded.dev/framework@$version" | sed -n 's/^[[:space:]]*"Hash": "\([0-9a-f]*\)",*$/\1/p')
if [ -z "$proxied" ]; then
	echo "release-cli.sh: aicoded.dev/framework@$version is not on the Go proxy: push the tag $version first" >&2
	exit 1
fi
if [ "$proxied" != "$(git rev-parse "refs/tags/$version^{commit}")" ]; then
	echo "release-cli.sh: the Go proxy serves $version from commit $proxied, not from the local tag $version" >&2
	exit 1
fi

tmp=$(mktemp -d)
cleanup() {
	git -C "$root" worktree remove --force "$tmp/tree" 2>/dev/null || true
	rm -rf "$tmp"
}
trap cleanup EXIT
trap 'exit 130' INT TERM HUP
git worktree add -q --detach "$tmp/tree" "refs/tags/$version^{commit}"

cd "$tmp/tree/cmd/aicoded"
go mod edit -dropreplace=aicoded.dev/framework -require="aicoded.dev/framework@$version"
go mod tidy
go build ./...
go build -trimpath -o "$tmp/aicoded" .

if [ "$(go list -m -f '{{.Version}} {{.Replace}}' aicoded.dev/framework)" != "$version <nil>" ]; then
	echo "release-cli.sh: cmd/aicoded does not require aicoded.dev/framework $version unreplaced" >&2
	exit 1
fi
changed=$(git status --porcelain)
if [ "$changed" != " M cmd/aicoded/go.mod
 M cmd/aicoded/go.sum" ] && [ "$changed" != " M cmd/aicoded/go.mod" ]; then
	printf 'release-cli.sh: the release must change only cmd/aicoded/go.mod and go.sum, not:\n%s\n' "$changed" >&2
	exit 1
fi

git commit -q -m "Release aicoded $version" -- go.mod go.sum
git tag -a "$cli_tag" -m "aicoded $version"
echo "Tagged $cli_tag at $(git rev-parse --short HEAD), on top of $version. Push it with:"
echo "  git push origin $cli_tag"
echo "Then install it with:"
echo "  go install aicoded.dev/framework/cmd/aicoded@$version"
