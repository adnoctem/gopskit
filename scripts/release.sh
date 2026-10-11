#!/usr/bin/env bash
#
# Manage the package release workflow for 'gopskit': building all applications, creating a combined tarball of all
# applications and their LICENSE, as well as a separate tarball for each application within a predefined build
# directory.
#

set -o errexit
#set -o nounset
set -o pipefail

# Libraries
SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)

# shellcheck source=scripts/lib/paths.sh
. "${SCRIPT_DIR}/lib/paths.sh"

# shellcheck source=scripts/lib/stdout.sh
. "${SCRIPT_DIR}/lib/stdout.sh"

# Constants
BUILD_DIR="$(lib::paths::root)/dist"
BUILD_TMP_DIR="${BUILD_DIR}/tmp"

PLATFORM="linux"
BAZEL_CACHE_PATH="${HOME}/.cache/bazel"

# File references
license="$(lib::paths::root)/LICENSE"

# query package names and labels from Bazel
PKGS=()
LABELS=()

# multi-arch builds
ARCH=('amd64' 'arm64')

# ----------------------
#   'help' usage function
# ----------------------
function release::usage() {
	echo
	echo "Usage: $(basename "${0}") <COMMAND>"
	echo
	echo "executables     - Build gopskit executables"
	echo "tarballs        - Build gopskit distribution tarballs"
	echo "checksums       - Calculate the SHA256 checksums"
	echo "cleanup         - Clean-up temporary build directories"
	echo "help            - Print this usage information"
	echo
}

# ----------------------
#   'create_directories' function
# ----------------------
function release::lib::create_directories() {
	if [[ ! -d "$BUILD_DIR" ]]; then
		log::yellow "Creating build directory: $BUILD_DIR"
		mkdir -p "$BUILD_DIR"
	else
		log::red "Removing old build directory: $BUILD_DIR"
		rm -rf "$BUILD_DIR"
	fi

	# intentional
	if [[ ! -d "$BUILD_TMP_DIR" ]]; then
		log::yellow "Creating temporary directory within build directory: $BUILD_TMP_DIR"
		mkdir -p "$BUILD_TMP_DIR"
	fi

	log::green "Created build directories!"
	return 0
}

# ----------------------
#   'build_executables' function
# ----------------------
function release::build_executables() {
	# create build directories
	release::lib::create_directories
	rc=$?
	if [[ $rc -ne 0 ]]; then
		log::red "Could not create build directories!"
		return "$rc"
	fi

	for arch in "${ARCH[@]}"; do
		# build
		log::yellow "Building 'gopskit' executables for architecture: $arch"
		platform=$(printf "@io_bazel_rules_go//go/toolchain:%s_%s" "$PLATFORM" "$arch")
		bazel --output_user_root="$BAZEL_CACHE_PATH" build --stamp --platforms="$platform" //...
		rc=$?
		if [[ $rc -ne 0 ]]; then
			log::red "Bazel build failed. Could not build executables!"
			return 1
		fi
		log::green "Built 'gopskit' executables for architecture: $arch!"

		#copy
		for lbl in "${LABELS[@]}"; do
			pkg=$(echo "$lbl" | cut -d':' -f 2)
			log::yellow "Copying $pkg binary to $BUILD_TMP_DIR"

			output=$(bazel --output_user_root="$BAZEL_CACHE_PATH" cquery \
				--output=files --platforms="$platform" "$lbl" 2>/dev/null)
			arch_pkg=$(printf "%s_%s" "$pkg" "$arch")
			out_pkg=$([ "$PLATFORM" == "windows" ] && echo "$arch_pkg.exe" || echo "$arch_pkg") # append .exe on Windows
			destination=$(printf "%s/%s" "$BUILD_TMP_DIR" "$out_pkg")
			cp "$output" "$destination"
		done

	done
	return 0
}

# ----------------------
#   'build_tarballs' function
# ----------------------
function release::build_tarballs() {
	log::yellow "Copying package LICENSE to $BUILD_TMP_DIR"
	cp "$license" "$BUILD_TMP_DIR"

	for arch in "${ARCH[@]}"; do
		for pkg in "${PKGS[@]}"; do
			log::yellow "Building tarball for $pkg"
			input=$(printf "%s_%s" "$pkg" "$arch")
			output=$(printf "%s/%s_%s_%s.tar.gz" "$BUILD_DIR" "$pkg" "$PLATFORM" "$arch")

			tar czfvP "$output" -C "$BUILD_TMP_DIR" "$input" LICENSE >/dev/null
			rc=$?
			if [[ $rc -ne 0 ]]; then
				log::red "Could not build tarball for package: $pkg and architecture $arch"
				return "$rc"
			fi
		done

		log::yellow "Building 'gopskit' tarball for architecture: $arch"
		tarb=$(printf "%s/gopskit_%s_%s.tar.gz" "$BUILD_DIR" "$PLATFORM" "$arch")
		tar czfvP "$tarb" -C "$BUILD_TMP_DIR" "${PKGS[@]/%/_$arch}" "LICENSE" >/dev/null
		rc=$?
		if [[ $rc -ne 0 ]]; then
			log::red "Could not build 'gopskit' tarball"
			return "$rc"
		fi
	done

	log::yellow "Changing working directory to $BUILD_DIR"
	old_wd=$(pwd)
	cd "$BUILD_DIR"

	file=$(printf "%s_checksums.txt" "$PLATFORM")
	[ -e "$file" ] && rm -f "$file"
	for dtar in *.tar.gz; do
		sha256sum "$dtar" >>"$file"
		rc=$?
		if [[ $rc -ne 0 ]]; then
			log::red "Could not calculate SHA256 checksums for tarball: $dtar"
			return "$rc"
		fi
	done

	# switch back
	cd "$old_wd"

	log::green "Built 'gopskit' distribution tarballs!"
	return 0
}

# ----------------------
#   'calculate_checksums' function
# ----------------------
function release::calculate_checksums() {
	log::yellow "Changing working directory to $BUILD_DIR"
	old_wd=$(pwd)
	cd "$BUILD_DIR"

	file="CHECKSUMS_SHA256.txt"
	[ -e "$file" ] && rm -f "$file"
	for dtar in *.txt; do
		echo "# SHA256" >>"$file"
		cat "$dtar" >>"$file"
		rc=$?
		if [[ $rc -ne 0 ]]; then
			log::red "Could not calculate SHA256 checksums for tarball: $dtar"
			return "$rc"
		fi
	done

	# switch back
	cd "$old_wd"

	log::green "Calculated 'gopskit' SHA256 checksums!"
	return 0
}

# --------------------------------
#   MAIN
# --------------------------------
function main() {
	local cmd=${1}

	# initialize constants - gopskit-bridge only ever runs from its container image, so it gets no tarball
	raw=$(bazel --output_user_root="$BAZEL_CACHE_PATH" query 'kind("go_binary", //...) except //cmd/gopskit-bridge:all' 2>/dev/null)
	while IFS="$(printf '\n')" read -r line; do LABELS+=("$line"); done <<<"$raw"
	while IFS="$(printf '\n')" read -r line; do PKGS+=("$(echo "$line" | cut -d':' -f 2)"); done <<<"$raw"
	unset IFS

	# CI config
	if [[ "$CI" == "true" ]]; then
		if [[ "$OS" =~ "ubuntu" ]]; then
			PLATFORM="linux"
		elif [[ "$OS" =~ "macos" ]]; then
			PLATFORM="darwin"
		elif [[ "$OS" =~ "windows" ]]; then
			PLATFORM="windows"
		fi

		if [[ -n "$CACHE_PATH" ]]; then
			BAZEL_CACHE_PATH="$CACHE_PATH"
		fi
	fi

	case "${cmd}" in
	executables)
		release::build_executables
		rc=$?
		if [[ $rc -ne 0 ]]; then
			log::red "Could not build 'gopskit' executables!"
			return "$rc"
		fi
		;;
	tarballs)
		release::build_tarballs
		rc=$?
		if [[ $rc -ne 0 ]]; then
			log::red "Could not build 'gopskit' tarballs!"
			return "$rc"
		fi
		;;
	checksums)
		release::calculate_checksums
		rc=$?
		if [[ $rc -ne 0 ]]; then
			log::red "Could not calculate 'gopskit' checksums!"
			return "$rc"
		fi
		;;
	cleanup)
		if [[ -d "$BUILD_TMP_DIR" ]]; then
			log::yellow "Removing temporary directory within build directory: $BUILD_TMP_DIR"
			rm -rf "$BUILD_TMP_DIR"
		fi
		rc=$?
		if [[ $rc -ne 0 ]]; then
			log::red "Could not cleanup 'gopskit' build directories!"
			return "$rc"
		fi
		;;
	help)
		release::usage
		return $?
		;;
	*)
		log::red "Unknown command: ${cmd}. See 'help' command for usage information:"
		release::usage
		;;
	esac

	log::green "release.sh finished!"
}

# ------------
# 'main' call
# ------------
main "$@"
