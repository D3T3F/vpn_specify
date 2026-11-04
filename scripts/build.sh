#!/bin/sh
# Builds vpn_specify for Linux (graphical interface and terminal).
#
#   ./scripts/build.sh          builds ./vpn_specify in the project root
#   ./scripts/build.sh -test    runs go vet and the tests first
#   ./scripts/build.sh -android also builds dist/vpn_specify-<version>-android-arm64.apk
#                               (needs ANDROID_HOME with the SDK and NDK, and a JDK)
#
# The tests that create a tun interface only run as root:
#   sudo -E go test ./internal/tunnel
set -eu
cd "$(dirname "$0")/.."
version=$(tr -d ' \r\n' < VERSION)
if [ "${1:-}" = "-test" ]; then
	go vet ./...
	go test ./...
fi
CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X vpn_specify/internal/version.Version=$version" -o vpn_specify ./cmd/vpn_specify
echo "vpn_specify $version: $(pwd)/vpn_specify"
if [ "${1:-}" = "-android" ]; then
	(cd android && sh ./gradlew assembleRelease --console=plain)
	mkdir -p dist
	cp android/app/build/outputs/apk/release/app-release.apk "dist/vpn_specify-$version-android-arm64.apk"
	echo "Android: $(pwd)/dist/vpn_specify-$version-android-arm64.apk"
	[ -f android/keystore.properties ] || echo "warning: signed with the debug key, for testing only (see android/keystore.properties.example)" >&2
fi
