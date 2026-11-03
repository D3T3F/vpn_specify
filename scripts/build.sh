#!/bin/sh
# Compila o vpn_specify para Linux (só a versão de terminal: up, run, status).
#
#   ./scripts/build.sh          gera ./vpn_specify na raiz do projeto
#   ./scripts/build.sh -test    roda go vet e os testes antes
#
# Os testes que criam interface tun só rodam como root:
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
