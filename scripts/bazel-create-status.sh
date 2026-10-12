#!/usr/bin/env bash
#
# Create the workspace status Bazel stamps the binaries with (see .bazelrc and the x_defs in
# cmd/*/BUILD.bazel). Bazel runs it for every build and reads its stdout as "KEY value" lines.
#
# It deliberately needs nothing but Bash, Git and coreutils - not libsh: it runs on every build,
# in CI too, and any other output on stdout would end up in the stamp.

set -euo pipefail

ROOT=$(git rev-parse --show-toplevel)

# the Go SDK version from MODULE.bazel's go_sdk.download(version = "...")
GO_VERSION=$(sed -n 's/^go_sdk\.download(version = "\([^"]*\)").*/\1/p' "${ROOT}/MODULE.bazel")

# a human-readable platform: the OS release's name on Linux, "<kernel> <release>" elsewhere
PLATFORM="$(uname -s) $(uname -r)"
if [[ -r /etc/os-release ]]; then
  PLATFORM=$(. /etc/os-release && printf '%s' "${PRETTY_NAME:-${PLATFORM}}")
fi

# UTC in RFC 3339, without GNU date's --rfc-3339
DATE=$(date -u +%Y-%m-%dT%H:%M:%SZ)

# the release version: semantic-release passes the upcoming version via GOPSKIT_VERSION, since the
# tag doesn't exist yet when it builds; everywhere else the nearest tag describes the build
VERSION="${GOPSKIT_VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo "0.0.0-unknown")}"

echo STABLE_GOPSKIT_VERSION "${VERSION#v}"
echo GIT_COMMIT_SHA "$(git rev-parse --short=7 HEAD)"
echo GIT_BRANCH "$(git rev-parse --abbrev-ref HEAD)"
echo GO_VERSION "${GO_VERSION}"
echo BUILD_DATE "${DATE}"
echo PLATFORM "${PLATFORM}"
