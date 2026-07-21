#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
temporary=$(mktemp -d)
trap 'rm -rf "$temporary"' EXIT HUP INT TERM

mkdir -p "$temporary/bin" "$temporary/release" "$temporary/payload" "$temporary/install"
cat >"$temporary/bin/uname" <<'EOF'
#!/bin/sh
case "${1:-}" in
  -s) echo Linux ;;
  -m) echo x86_64 ;;
  *) echo Linux ;;
esac
EOF
cat >"$temporary/bin/curl" <<'EOF'
#!/bin/sh
output=
url=
while [ "$#" -gt 0 ]; do
  case "$1" in
    -o) output=$2; shift ;;
    http*) url=$1 ;;
  esac
  shift
done
case "$url" in
  */releases/latest) printf '%s' 'https://github.com/TheSnakeFang/rlviz/releases/tag/v1.2.3'; exit 0 ;;
esac
cp "$INSTALL_TEST_RELEASE/$(basename "$url")" "$output"
EOF
chmod +x "$temporary/bin/uname" "$temporary/bin/curl"

cat >"$temporary/payload/rlviz" <<'EOF'
#!/bin/sh
echo rlviz-test
EOF
chmod +x "$temporary/payload/rlviz"

archive=rlviz_1.2.3_linux_x86_64.tar.gz
tar -czf "$temporary/release/$archive" -C "$temporary/payload" rlviz
if command -v sha256sum >/dev/null 2>&1; then
  checksum=$(sha256sum "$temporary/release/$archive" | awk '{ print $1 }')
else
  checksum=$(shasum -a 256 "$temporary/release/$archive" | awk '{ print $1 }')
fi
printf '%s  %s\n' "$checksum" "$archive" >"$temporary/release/checksums.txt"

PATH="$temporary/bin:$PATH" INSTALL_TEST_RELEASE="$temporary/release" \
  RLVIZ_VERSION=v1.2.3 RLVIZ_INSTALL_DIR="$temporary/install" \
  "$root/scripts/install.sh" >/dev/null

test -x "$temporary/install/rlviz"
test "$("$temporary/install/rlviz")" = rlviz-test

rm -rf "$temporary/install"
mkdir "$temporary/install"
PATH="$temporary/bin:$PATH" INSTALL_TEST_RELEASE="$temporary/release" \
  RLVIZ_VERSION=latest RLVIZ_INSTALL_DIR="$temporary/install" \
  "$root/scripts/install.sh" >/dev/null
test "$("$temporary/install/rlviz")" = rlviz-test

printf '%064d  %s\n' 0 "$archive" >"$temporary/release/checksums.txt"
if PATH="$temporary/bin:$PATH" INSTALL_TEST_RELEASE="$temporary/release" \
  RLVIZ_VERSION=1.2.3 RLVIZ_INSTALL_DIR="$temporary/rejected" \
  "$root/scripts/install.sh" >/dev/null 2>&1; then
  echo "install test: checksum mismatch unexpectedly succeeded" >&2
  exit 1
fi
test ! -e "$temporary/rejected/rlviz"

echo "install script tests passed"
