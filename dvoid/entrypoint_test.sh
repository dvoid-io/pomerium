#!/usr/bin/env bash
# dvoid (org-specific): prove dvoid/entrypoint.sh in the image's final base stage
# (the distroless :debug busybox it actually runs under), without building Pomerium:
# a stub stands in for /bin/pomerium and a scratch dir for /pomerium.
#
#   bash dvoid/entrypoint_test.sh
#
# Needs docker, gzip, base64 (coreutils). Runs in CI before the image build
# (.github/workflows/dvoid-build-image.yml).
set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
BASE="$(sed -n 's/^FROM \(gcr\.io\/distroless\/[^ ]*\).*/\1/p' "$HERE/../Dockerfile" | tail -1)"
[[ -n "$BASE" ]] || { echo "no distroless base stage in Dockerfile" >&2; exit 2; }
WORK="$(mktemp -d)"; trap 'rm -rf "$WORK"' EXIT
mkdir -p "$WORK/pomerium"; chmod 0777 "$WORK/pomerium"
printf '#!/busybox/sh\necho "pomerium-stub $*"\n' > "$WORK/pomerium-stub"; chmod 0755 "$WORK/pomerium-stub"

# A config shaped like the real one: multi-line, quotes, `${…}` that must survive.
cat > "$WORK/want.yaml" <<'EOF'
authenticate_service_url: https://authenticate.example
jwt_claims_headers:
  x-dvoid-email: email
routes:
- from: https://app.example
  set_request_headers:
    x-dvoid-access-token: ${pomerium.access_token}
  policy:
    allow:
      or:
      - claim/urn:zitadel:iam:user:resourceowner:id: "123"   # org
EOF
PLAIN="$(base64 -w0 < "$WORK/want.yaml")"
GZ="$(gzip -9n < "$WORK/want.yaml" | base64 -w0)"

fails=0
run() { # run <env…> — the entrypoint with a fresh /pomerium/config.yaml sentinel
  printf 'image-default\n' > "$WORK/pomerium/config.yaml"
  docker run --rm --entrypoint /bin/dvoid-entrypoint.sh \
    -v "$HERE/entrypoint.sh:/bin/dvoid-entrypoint.sh:ro" -v "$WORK/pomerium-stub:/bin/pomerium:ro" \
    -v "$WORK/pomerium:/pomerium" "$@" "$BASE" --config /pomerium/config.yaml > "$WORK/out" 2>&1
}
ok()   { echo "ok   $1"; }
bad()  { echo "FAIL $1"; sed 's/^/     /' "$WORK/out"; fails=$((fails + 1)); }
expect_file() { # expect_file <desc> <file> <env…>
  local d="$1" f="$2"; shift 2
  if run "$@" && cmp -s "$f" "$WORK/pomerium/config.yaml" && grep -q '^pomerium-stub --config /pomerium/config.yaml$' "$WORK/out"; then ok "$d"; else bad "$d"; fi
}
expect_refusal() { # expect_refusal <desc> <message fragment> <env…>
  local d="$1" m="$2"; shift 2
  if ! run "$@" && grep -q "$m" "$WORK/out" && ! grep -q pomerium-stub "$WORK/out" \
     && [[ "$(cat "$WORK/pomerium/config.yaml")" == image-default ]]; then ok "$d"; else bad "$d"; fi
}
printf 'image-default\n' > "$WORK/default.yaml"

expect_file "gzip variable is decompressed byte for byte" "$WORK/want.yaml" -e "POMERIUM_CONFIG_GZ_B64=$GZ"
expect_file "plain variable still works (backward compatible)" "$WORK/want.yaml" -e "POMERIUM_CONFIG_B64=$PLAIN"
expect_file "neither set keeps the image's own file" "$WORK/default.yaml"
expect_refusal "both set is refused" "both POMERIUM_CONFIG_GZ_B64 and POMERIUM_CONFIG_B64" \
  -e "POMERIUM_CONFIG_GZ_B64=$GZ" -e "POMERIUM_CONFIG_B64=$PLAIN"
expect_refusal "truncated gzip is refused" "does not decompress" -e "POMERIUM_CONFIG_GZ_B64=${GZ:0:$(( ${#GZ} - 24 ))}"
expect_refusal "plain base64 in the gzip variable is refused" "does not decompress" -e "POMERIUM_CONFIG_GZ_B64=$PLAIN"
expect_refusal "garbage in the gzip variable is refused" "POMERIUM_CONFIG_GZ_B64" -e "POMERIUM_CONFIG_GZ_B64=!!not base64!!"

(( fails == 0 )) || { echo "$fails case(s) failed"; exit 1; }
echo "entrypoint: all cases pass ($BASE)"
