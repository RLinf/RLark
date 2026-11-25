#!/usr/bin/env bash
set -euo pipefail

# Create or rotate one site's authenticated Gateway relay token.
# The value is 32 random bytes, base64 encoded, and is never printed.
# Extend the station-relay-tokens Role's resourceNames for new sites before
# rotating. Both station-controller and station-api need get in default.
site=${1:?usage: rotate-station-relay-token.sh SITE}
if [[ ! $site =~ ^[a-z0-9]([-a-z0-9]*[a-z0-9])?$ ]]; then
  echo "site must be a DNS label" >&2
  exit 2
fi

umask 077
token_file=$(mktemp)
trap 'rm -f "$token_file"' EXIT
openssl rand -base64 32 | tr -d '\n' > "$token_file"
kubectl create secret generic "station-relay-token-$site" \
  --namespace default \
  --from-file="token=$token_file" \
  --dry-run=client -o yaml |
  kubectl apply -f -
