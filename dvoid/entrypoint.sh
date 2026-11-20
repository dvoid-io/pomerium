#!/busybox/sh
# distroless :debug has no /bin/sh — busybox lives under /busybox/ (sh, base64, gunzip, printf).
# dvoid (org-specific): materialise the config file from the environment.
#
# The estate generates Pomerium's config from its catalog (platform/services/
# pomerium/gen-config.mjs) and deploys this image as an IMAGE source on Railway,
# where there is no Dockerfile to bake a file into. Pomerium reads most options
# from env, but the ones that need structure — `jwt_claims_headers` with custom
# header names, `routes` — only round-trip through the config FILE (from env the
# claim-headers option is a comma list of claim names; see config/custom.go
# JWTClaimHeaders.UnmarshalJSON). So the whole YAML travels as one variable and
# lands where upstream's CMD already points. Secrets stay as plain env vars (env
# overrides the file).
#
# Two encodings, exactly one at a time:
#   POMERIUM_CONFIG_GZ_B64  base64 of gzip of the YAML. Railway caps a variable at
#                           32,768 characters and the plain form outgrew it
#                           (2026-11-19); gzip brings it to about a tenth.
#   POMERIUM_CONFIG_B64     base64 of the YAML (the original form, still honoured).
# Both set is refused rather than guessed: during a migration that is exactly the
# moment a stale value would silently win. A value that does not decode is fatal,
# so Pomerium never starts on a truncated file. Neither set: the image's own file.
# Tested by dvoid/entrypoint_test.sh in the image's base stage.
set -eu
out=/pomerium/config.yaml
die() { echo "dvoid-entrypoint: $*" >&2; exit 64; }

gz="${POMERIUM_CONFIG_GZ_B64:-}"
plain="${POMERIUM_CONFIG_B64:-}"
if [ -n "$gz" ] && [ -n "$plain" ]; then
  die "both POMERIUM_CONFIG_GZ_B64 and POMERIUM_CONFIG_B64 are set; remove one (the generator emits POMERIUM_CONFIG_GZ_B64)"
fi

tmp="$(/busybox/mktemp)"
if [ -n "$gz" ]; then
  /busybox/printf "%s" "$gz" | /busybox/base64 -d > "$tmp.gz" 2>/dev/null \
    || die "POMERIUM_CONFIG_GZ_B64 is not valid base64"
  /busybox/gunzip -c "$tmp.gz" > "$tmp" 2>/dev/null \
    || die "POMERIUM_CONFIG_GZ_B64 does not decompress (corrupt or truncated gzip)"
  /busybox/rm -f "$tmp.gz"
elif [ -n "$plain" ]; then
  /busybox/printf "%s" "$plain" | /busybox/base64 -d > "$tmp" 2>/dev/null \
    || die "POMERIUM_CONFIG_B64 is not valid base64"
fi
if [ -n "$gz$plain" ]; then
  [ -s "$tmp" ] || die "the config variable decoded to an empty file"
  /busybox/cat "$tmp" > "$out"
fi
/busybox/rm -f "$tmp"
exec /bin/pomerium "$@"
