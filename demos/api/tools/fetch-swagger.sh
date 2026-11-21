#!/bin/sh
# Swagger UI (Apache-2.0) is vendored at build time, not committed: pinned version + sha256 of the npm tarball.
# Usage: tools/fetch-swagger.sh <web dir>   → <web dir>/vendor/swagger-ui/{swagger-ui-bundle.js,swagger-ui.css,LICENSE}
set -eu
VER=5.17.14
SHA=c57badf459aa6e65cc036b3862d0502a63f9a22546407ffcb0e64f85f816bb28
OUT="${1:?web dir}/vendor/swagger-ui"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
curl -sfL -o "$TMP/sw.tgz" "https://registry.npmjs.org/swagger-ui-dist/-/swagger-ui-dist-$VER.tgz"
echo "$SHA  $TMP/sw.tgz" | sha256sum -c - >/dev/null 2>&1 || echo "$SHA  $TMP/sw.tgz" | shasum -a 256 -c - >/dev/null
tar xzf "$TMP/sw.tgz" -C "$TMP"
mkdir -p "$OUT"
cp "$TMP/package/swagger-ui-bundle.js" "$TMP/package/swagger-ui.css" "$TMP/package/LICENSE" "$OUT/"
