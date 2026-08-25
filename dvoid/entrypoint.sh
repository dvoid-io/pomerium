#!/bin/sh
# dvoid (org-specific): materialise the config file from the environment.
#
# The estate generates Pomerium's config from its catalog (platform/services/
# pomerium/gen-config.mjs) and deploys this image as an IMAGE source on Railway,
# where there is no Dockerfile to bake a file into. Pomerium reads most options
# from env, but the ones that need structure — `jwt_claims_headers` with custom
# header names, `routes` — only round-trip through the config FILE (from env the
# claim-headers option is a comma list of claim names; see config/custom.go
# JWTClaimHeaders.UnmarshalJSON). So the whole YAML travels as one variable,
# base64 so newlines and quotes survive every hop, and lands where upstream's
# CMD already points. Secrets stay as plain env vars (env overrides the file).
set -eu
if [ -n "${POMERIUM_CONFIG_B64:-}" ]; then
  printf '%s' "$POMERIUM_CONFIG_B64" | base64 -d > /pomerium/config.yaml
fi
exec /bin/pomerium "$@"
