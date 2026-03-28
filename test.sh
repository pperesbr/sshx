#!/bin/bash
# Usage: ./test.sh <vendor> <host> <prompt>
# Example: ./test.sh huawei-vrp hostname "<hostname>"

set -euo pipefail

VENDOR="${1:?Usage: ./test.sh <vendor> <host> <prompt>}"
HOST="${2:?Missing host}"
PROMPT="${3:?Missing prompt}"

read -rp "User: " USER
read -rsp "Password: " PASS
echo

case "$VENDOR" in
  huawei-vrp)   TEST="TestHuaweiVRPLive" ;;
  cisco-iosxr)  TEST="TestCiscoIOSXRLive" ;;
  nokia-sros)   TEST="TestNokiaSROSLive" ;;
  juniper)      TEST="TestJuniperJunosLive" ;;
  huawei-olt)   TEST="TestHuaweiOLTLive" ;;
  nokia-7360)   TEST="TestNokiaISAMLive" ;;
  *)            echo "Unknown vendor: $VENDOR"; exit 1 ;;
esac

SSHX_HOST="$HOST" \
SSHX_USER="$USER" \
SSHX_PASS="$PASS" \
SSHX_PROMPT="$PROMPT" \
go test -v -count=1 -run "$TEST" ./...
