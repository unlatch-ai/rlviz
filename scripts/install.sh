#!/bin/sh
set -eu

REPOSITORY="TheSnakeFang/rlviz"
VERSION="${RLVIZ_VERSION:-latest}"
INSTALL_DIR="${RLVIZ_INSTALL_DIR:-$HOME/.local/bin}"

case "$(uname -s)" in
  Darwin) os=darwin ;;
  Linux) os=linux ;;
  *) echo "rlviz: unsupported operating system: $(uname -s)" >&2; exit 1 ;;
esac

case "$(uname -m)" in
  x86_64|amd64) arch=x86_64 ;;
  arm64|aarch64) arch=arm64 ;;
  *) echo "rlviz: unsupported architecture: $(uname -m)" >&2; exit 1 ;;
esac

if [ "$VERSION" = latest ]; then
  release_url="https://github.com/$REPOSITORY/releases/latest/download"
  resolved_version=$(curl -fsSLI -o /dev/null -w '%{url_effective}' "https://github.com/$REPOSITORY/releases/latest" | sed 's#.*/v##')
else
  resolved_version=${VERSION#v}
  release_url="https://github.com/$REPOSITORY/releases/download/v$resolved_version"
fi

archive="rlviz_${resolved_version}_${os}_${arch}.tar.gz"
temporary=$(mktemp -d)
trap 'rm -rf "$temporary"' EXIT HUP INT TERM

curl -fsSL "$release_url/$archive" -o "$temporary/$archive"
curl -fsSL "$release_url/checksums.txt" -o "$temporary/checksums.txt"

expected=$(awk -v file="$archive" '$2 == file { print $1 }' "$temporary/checksums.txt")
if [ -z "$expected" ]; then
  echo "rlviz: checksum not found for $archive" >&2
  exit 1
fi

if command -v sha256sum >/dev/null 2>&1; then
  actual=$(sha256sum "$temporary/$archive" | awk '{ print $1 }')
else
  actual=$(shasum -a 256 "$temporary/$archive" | awk '{ print $1 }')
fi

if [ "$actual" != "$expected" ]; then
  echo "rlviz: checksum verification failed" >&2
  exit 1
fi

mkdir -p "$INSTALL_DIR"
tar -xzf "$temporary/$archive" -C "$temporary"
install "$temporary/rlviz" "$INSTALL_DIR/rlviz"

echo "installed rlviz to $INSTALL_DIR"
