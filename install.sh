#!/usr/bin/env bash
# Download and install a verified UnlockScope release. This script never pipes
# downloaded content to a shell and verifies the archive before extraction.
set -euo pipefail

REPO="shui1iao/UnlockScope"
VERSION="${UNLOCKSCOPE_VERSION:-latest}"
PREFIX="${PREFIX:-}"
BIN_NAME="unlockscope"

need_command() { command -v "$1" >/dev/null 2>&1 || { echo "error: required command not found: $1" >&2; exit 1; }; }
need_command tar
if command -v curl >/dev/null 2>&1; then
  download() { curl --fail --location --silent --show-error --retry 2 "$1" -o "$2"; }
else
  need_command wget
  download() { wget --https-only --quiet --output-document="$2" "$1"; }
fi

case "$(uname -s)" in
  Linux) os=linux ;;
  Darwin) os=darwin ;;
  FreeBSD) os=freebsd ;;
  *) echo "error: unsupported operating system: $(uname -s)" >&2; exit 1 ;;
esac
case "$(uname -m)" in
  x86_64|amd64) arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
  i386|i686) arch=386 ;;
  *) echo "error: unsupported architecture: $(uname -m)" >&2; exit 1 ;;
esac

workdir="$(mktemp -d)"
cleanup() { rm -rf "$workdir"; }
trap cleanup EXIT

if [ "$VERSION" = latest ]; then
  metadata="$workdir/latest.json"
  download "https://api.github.com/repos/${REPO}/releases/latest" "$metadata"
  VERSION="$(sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$metadata" | head -n 1)"
  [ -n "$VERSION" ] || { echo "error: could not determine latest release; set UNLOCKSCOPE_VERSION=vX.Y.Z" >&2; exit 1; }
fi
VERSION="${VERSION#v}"
archive="${BIN_NAME}_${VERSION}_${os}_${arch}.tar.gz"
base="https://github.com/${REPO}/releases/download/v${VERSION}"
archive_path="$workdir/$archive"
checksum_path="$workdir/$archive.sha256"
download "$base/$archive" "$archive_path"
download "$base/$archive.sha256" "$checksum_path"

expected="$(awk 'NF {print $1; exit}' "$checksum_path")"
[ "${#expected}" -eq 64 ] || { echo "error: malformed checksum file" >&2; exit 1; }
actual="$(sha256sum "$archive_path" 2>/dev/null | awk '{print $1}' || shasum -a 256 "$archive_path" | awk '{print $1}')"
[ "$actual" = "$expected" ] || { echo "error: SHA-256 verification failed" >&2; exit 1; }

tar -xzf "$archive_path" -C "$workdir"
[ -f "$workdir/$BIN_NAME" ] || { echo "error: release archive has no $BIN_NAME binary" >&2; exit 1; }
chmod 0755 "$workdir/$BIN_NAME"

if [ -z "$PREFIX" ]; then
  if [ -w /usr/local/bin ]; then PREFIX=/usr/local/bin; else PREFIX="${HOME:-/tmp}/.local/bin"; fi
fi
mkdir -p "$PREFIX"
tmp_bin="$PREFIX/.${BIN_NAME}.tmp.$$"
cp "$workdir/$BIN_NAME" "$tmp_bin"
chmod 0755 "$tmp_bin"
mv -f "$tmp_bin" "$PREFIX/$BIN_NAME"
printf 'installed %s to %s\n' "v${VERSION}" "$PREFIX/$BIN_NAME"
case ":${PATH}:" in *":${PREFIX}:"*) ;; *) printf 'add %s to PATH to invoke unlockscope\n' "$PREFIX" >&2 ;; esac
