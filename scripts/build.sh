#!/bin/sh
# Builds vpn_specify for Linux (graphical interface and terminal).
#
#   ./scripts/build.sh          builds ./vpn_specify in the project root
#   ./scripts/build.sh -test    runs go vet and the tests first
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
