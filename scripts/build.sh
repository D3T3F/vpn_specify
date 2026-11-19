#!/bin/sh
# Builds vpn_specify (graphical interface and terminal).
#
#   ./scripts/build.sh          builds ./vpn_specify in the project root
#   ./scripts/build.sh -test    runs go vet and the tests first
#   ./scripts/build.sh -linux   also builds dist/vpn_specify-<version>-linux-{amd64,arm64}.tar.gz
#   ./scripts/build.sh -mac     also builds dist/vpn_specify-<version>-macos-{arm64,amd64}.tar.gz
#   ./scripts/build.sh -android also builds dist/vpn_specify-<version>-android-arm64.apk
#                               (needs ANDROID_HOME with the SDK and NDK, and a JDK)
#
# The tests that create a tun interface only run as root:
#   sudo -E go test ./internal/tunnel
set -eu
cd "$(dirname "$0")/.."
version=$(tr -d ' \r\n' < VERSION)

do_test=0 do_linux=0 do_mac=0 do_android=0
for arg in "$@"; do
	case "$arg" in
	-test) do_test=1 ;;
	-linux) do_linux=1 ;;
	-mac) do_mac=1 ;;
	-android) do_android=1 ;;
	esac
done

# package_unix <platform> <goos> <arch> builds dist/vpn_specify-<version>-<platform>-<arch>.tar.gz:
# a single executable (GUI and terminal), LICENSE and licenses/, as build.ps1 does.
package_unix() {
	platform=$1 goos=$2 arch=$3
	name="vpn_specify-$version-$platform-$arch"
	stage="dist/$name"
	rm -rf "$stage"
	mkdir -p "$stage/licenses"
	CGO_ENABLED=0 GOOS="$goos" GOARCH="$arch" go build -trimpath -ldflags "-s -w -X vpn_specify/internal/version.Version=$version" -o "$stage/vpn_specify" ./cmd/vpn_specify
	cp LICENSE "$stage/LICENSE"
	# license files of the Go modules used by the program (as Copy-ThirdPartyLicenses in build.ps1);
	# go list goes to a file first, so that its failure stops the script (set -e)
	tmp=$(mktemp -d)
	GOOS="$goos" GOARCH="$arch" go list -deps -f '{{with .Module}}{{if not .Main}}{{.Path}}|{{.Dir}}{{end}}{{end}}' ./cmd/vpn_specify > "$tmp/deps"
	sort -u "$tmp/deps" > "$tmp/modules"
	while IFS='|' read -r modpath moddir; do
		[ -n "$moddir" ] || continue
		for f in "$moddir"/*; do
			[ -f "$f" ] || continue
			case "$(basename "$f")" in
			LICEN[CS]E*|COPYING*) suffix= ;;
			NOTICE*) suffix=-NOTICE ;;
			*) continue ;;
			esac
			cp "$f" "$stage/licenses/$(printf '%s' "$modpath" | tr '/:' '__')$suffix.txt"
		done
	done < "$tmp/modules"
	rm -rf "$tmp"
	find "$stage" -type d -exec chmod 0755 {} +
	find "$stage" -type f -exec chmod 0644 {} +
	chmod 0755 "$stage/vpn_specify"
	# root owner in the archive, as the Windows build makes it
	if tar --version 2>/dev/null | grep -q GNU; then
		owner="--owner=0 --group=0 --numeric-owner"
	else
		owner="--uid=0 --gid=0 --numeric-owner"
	fi
	rm -f "dist/$name.tar.gz"
	(cd dist && tar $owner -czf "$name.tar.gz" "$name")
	echo "$platform $arch: $(pwd)/dist/$name.tar.gz"
}

if [ "$do_test" -eq 1 ]; then
	go vet ./...
	go test ./...
fi
CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X vpn_specify/internal/version.Version=$version" -o vpn_specify ./cmd/vpn_specify
echo "vpn_specify $version: $(pwd)/vpn_specify"
if [ "$do_linux" -eq 1 ]; then
	package_unix linux linux amd64
	package_unix linux linux arm64
fi
if [ "$do_mac" -eq 1 ]; then
	package_unix macos darwin arm64
	package_unix macos darwin amd64
fi
if [ "$do_android" -eq 1 ]; then
	(cd android && sh ./gradlew assembleRelease --console=plain)
	mkdir -p dist
	cp android/app/build/outputs/apk/release/app-release.apk "dist/vpn_specify-$version-android-arm64.apk"
	echo "Android: $(pwd)/dist/vpn_specify-$version-android-arm64.apk"
	[ -f android/keystore.properties ] || echo "warning: signed with the debug key, for testing only (see android/keystore.properties.example)" >&2
fi
