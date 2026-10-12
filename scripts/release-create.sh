#!/usr/bin/env bash
#
# Create gopskit's release artifacts: every CLI built for each architecture, a tarball per CLI and
# architecture with the LICENSE, one tarball per architecture with all of them, and a SHA-256
# checksum file covering every tarball.
#
# semantic-release runs it before tagging; see .releaserc. Requires libsh (LIBSH_DIR) and Bazel.

set -euo pipefail

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)
ROOT_DIR=$(cd -- "${SCRIPT_DIR}/.." && pwd)

# libsh is installed on the machine, not vendored: see docs/CONTRIBUTING.md
if [[ -z ${LIBSH_DIR:-} || ! -r ${LIBSH_DIR}/lib.sh ]]; then
	printf 'Cannot read libsh at %s/lib.sh; install libsh and export LIBSH_DIR (see docs/CONTRIBUTING.md).\n' "${LIBSH_DIR:-<unset>}" >&2
	exit 1
fi
LIBSH_DIR=$(cd -P -- "${LIBSH_DIR}" && pwd)
# shellcheck source=/dev/null
. "${LIBSH_DIR}/lib.sh"
if ! declare -F lib::opt::parse >/dev/null || ! declare -F lib::log::print_success >/dev/null; then
	printf 'Incompatible libsh at %s: this script needs the option parser and intent logging of libsh 0.7.0 or newer.\n' "${LIBSH_DIR}" >&2
	exit 1
fi

# -------------------------
#   Flag spec
# -------------------------

# shellcheck disable=SC2034 # read by the dynamically loaded option parser
OPTS=(
	"-v,--version:version:1:optional"
	"-o,--output-dir:output_dir:1:optional"
	"-a,--architectures:architectures:1:optional"
	",--check-prerequisites:check_prerequisites:0:optional"
	"-h,--help:help:0:optional"
)

# shellcheck disable=SC2034 # read by the dynamically loaded option parser
declare -A OPTS_HELP=(
	[version]="The release version stamped into the binaries (default: \$GOPSKIT_VERSION, else git describe)"
	[output_dir]="Directory to create the artifacts in; replaced if it exists (default: <repository>/dist)"
	[architectures]="Comma-separated GOARCH values to build for (default: amd64,arm64)"
	[check_prerequisites]="Check that the required commands are installed, then exit"
	[help]="Show this help message and exit"
)

declare -A OPTS_VALUES=()

# Every gopskit CLI is released for Linux; gopskit-bridge only ever runs from its container image.
readonly PLATFORM="linux"
readonly BINARIES_QUERY='kind("go_binary", //...) except //cmd/gopskit-bridge:all'
readonly CHECKSUMS_FILE="CHECKSUMS_SHA256.txt"

#######################################
# Check that the commands the release needs are installed.
# Arguments:
#   None
# Outputs:
#   One line per prerequisite.
# Returns:
#   0 if all were found, 1 otherwise.
#######################################
function release_create::prerequisites() {
	local prerequisites=('bazel' 'git' 'tar' 'sha256sum')

	for prerequisite in "${prerequisites[@]}"; do
		if ! lib::os::is_executable "${prerequisite}"; then
			lib::log::print_error "Could not find '${prerequisite}' in PATH. Please install it to proceed!"
			return 1
		fi

		lib::log::print_info "Found '${prerequisite}' in PATH."
	done

	lib::log::print_success "Found all prerequisites: '${prerequisites[*]}'."
}

#######################################
# Build every CLI for one architecture and copy the binaries into the staging directory.
# Arguments:
#   1 - GOARCH to build for
#   2 - staging directory
#   3+ - Bazel labels of the binaries
# Outputs:
#   Progress messages; Bazel's own output.
# Returns:
#   0 on success, 1 if the build or a copy failed.
#######################################
function release_create::build() {
	local arch=$1 staging=$2
	shift 2
	local platform="@io_bazel_rules_go//go/toolchain:${PLATFORM}_${arch}"

	lib::log::print_info "Building the gopskit CLIs for ${PLATFORM}/${arch}"
	if ! bazel build --stamp --platforms="${platform}" "$@"; then
		lib::log::print_error "Bazel could not build the CLIs for ${PLATFORM}/${arch}"
		return 1
	fi

	local label name output
	for label in "$@"; do
		name=${label##*:}
		output=$(bazel cquery --stamp --platforms="${platform}" --output=files "${label}" 2>/dev/null)
		cp -- "${ROOT_DIR}/${output}" "${staging}/${name}_${arch}"
	done

	lib::log::print_success "Built the gopskit CLIs for ${PLATFORM}/${arch}"
}

#######################################
# Package the staged binaries of one architecture: one tarball per CLI, and one with all of them,
# each including the LICENSE.
# Arguments:
#   1 - GOARCH
#   2 - staging directory
#   3 - output directory
#   4+ - CLI names
# Outputs:
#   Progress messages.
# Returns:
#   0 on success, 1 if tar failed.
#######################################
function release_create::package() {
	local arch=$1 staging=$2 output_dir=$3
	shift 3
	local name binaries=()

	for name in "$@"; do
		tar -czf "${output_dir}/${name}_${PLATFORM}_${arch}.tar.gz" -C "${staging}" "${name}_${arch}" LICENSE || return 1
		binaries+=("${name}_${arch}")
	done

	tar -czf "${output_dir}/gopskit_${PLATFORM}_${arch}.tar.gz" -C "${staging}" "${binaries[@]}" LICENSE || return 1
	lib::log::print_success "Packaged the ${PLATFORM}/${arch} tarballs"
}

#######################################
# Main
#######################################
function release_create::main() {
	lib::opt::parse "$@" || return
	[[ ${OPTS_VALUES[help]:-} != 1 ]] || return 0

	release_create::prerequisites >/dev/null || return
	[[ ${OPTS_VALUES[check_prerequisites]:-} != 1 ]] || {
		release_create::prerequisites
		return
	}

	local output_dir=${OPTS_VALUES[output_dir]:-${ROOT_DIR}/dist}
	local -a archs
	IFS=',' read -r -a archs <<<"${OPTS_VALUES[architectures]:-amd64,arm64}"

	# scripts/bazel-create-status.sh stamps the binaries with this version
	if [[ -n ${OPTS_VALUES[version]:-} ]]; then
		export GOPSKIT_VERSION=${OPTS_VALUES[version]}
	fi
	lib::log::print_info "Releasing version ${GOPSKIT_VERSION:-$(git -C "${ROOT_DIR}" describe --tags --always --dirty)}"

	cd -- "${ROOT_DIR}"

	local -a labels names
	mapfile -t labels < <(bazel query "${BINARIES_QUERY}" 2>/dev/null)
	if lib::data::array_is_empty "${labels[@]+"${labels[@]}"}"; then
		lib::log::print_error "Bazel found no binaries to release"
		return 1
	fi
	local label
	for label in "${labels[@]}"; do
		names+=("${label##*:}")
	done

	rm -rf -- "${output_dir}"
	local staging="${output_dir}/tmp"
	mkdir -p -- "${staging}"
	cp -- "${ROOT_DIR}/LICENSE" "${staging}/LICENSE"

	local arch
	for arch in "${archs[@]}"; do
		release_create::build "${arch}" "${staging}" "${labels[@]}" || return
		release_create::package "${arch}" "${staging}" "${output_dir}" "${names[@]}" || {
			lib::log::print_error "Could not package the ${PLATFORM}/${arch} tarballs"
			return 1
		}
	done

	(cd -- "${output_dir}" && sha256sum -- *.tar.gz >"${CHECKSUMS_FILE}") || {
		lib::log::print_error "Could not compute the checksums"
		return 1
	}
	rm -rf -- "${staging}"

	lib::log::print_success "Created the release artifacts for ${names[*]} in ${output_dir}"
}

release_create::main "$@"
