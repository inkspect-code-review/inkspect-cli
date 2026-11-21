#!/bin/sh
set -eu
# Native client only. Version is pinned; publishing a new installer is intentional.
version=0.1.2
case "$(uname -s)" in Darwin) os=darwin ;; Linux) os=linux ;; *) echo 'Use install.ps1 on Windows.' >&2; exit 1 ;; esac
case "$(uname -m)" in arm64|aarch64) arch=arm64 ;; x86_64|amd64) arch=amd64 ;; *) echo 'Unsupported CPU architecture.' >&2; exit 1 ;; esac
command -v curl >/dev/null 2>&1 || { echo 'curl is required to download Inkspect.' >&2; exit 1; }
command -v tar >/dev/null 2>&1 || { echo 'tar is required to unpack Inkspect.' >&2; exit 1; }
if command -v sha256sum >/dev/null 2>&1; then hash_tool=sha256sum
elif command -v shasum >/dev/null 2>&1; then hash_tool=shasum
else echo 'A SHA-256 checker (sha256sum or shasum) is required.' >&2; exit 1; fi
asset="inkspect_${version}_${os}_${arch}.tar.gz"
base="https://inkspect.si/cli/releases/v${version}"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT HUP INT TERM
curl --fail --silent --show-error --location --proto '=https' --proto-redir '=https' "$base/$asset" -o "$tmp/$asset" || { echo 'This release is not available. See https://inkspect.si/cli-docs.' >&2; exit 1; }
curl --fail --silent --show-error --location --proto '=https' --proto-redir '=https' "$base/SHA256SUMS" -o "$tmp/SHA256SUMS"
expected=$(awk -v name="$asset" '$2 == name {print $1}' "$tmp/SHA256SUMS")
if [ "$hash_tool" = sha256sum ]; then actual=$(sha256sum "$tmp/$asset" | cut -d ' ' -f 1)
else actual=$(shasum -a 256 "$tmp/$asset" | cut -d ' ' -f 1); fi
[ ${#expected} -eq 64 ] && [ "$expected" = "$actual" ] || { echo 'Checksum verification failed.' >&2; exit 1; }
# Only the expected binary is extracted, never arbitrary archive paths.
tar -xzf "$tmp/$asset" -C "$tmp" inkspect
[ -f "$tmp/inkspect" ] && [ ! -L "$tmp/inkspect" ] || { echo 'Invalid release archive.' >&2; exit 1; }
destination="${INKSPECT_INSTALL_DIR:-$HOME/.local/bin}"
mkdir -p "$destination"
[ ! -L "$destination/inkspect" ] || { echo 'Refusing to replace a symbolic link.' >&2; exit 1; }
install -m 755 "$tmp/inkspect" "$destination/inkspect"
printf 'Installed Inkspect %s to %s/inkspect\n' "$version" "$destination"
case ":$PATH:" in *":$destination:"*) ;; *) printf 'Add this directory to your shell PATH: %s\nThen reopen your terminal.\n' "$destination" ;; esac
"$destination/inkspect" --version
