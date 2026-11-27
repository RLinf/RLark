#!/usr/bin/env bash
set -euo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)
test_root=$(mktemp -d)
trap 'rm -rf "$test_root"' EXIT
mkdir -p "$test_root/bin" "$test_root/runtime"
cat >"$test_root/bin/kubectl" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail
manifest=${@: -1}
if [[ $* == *"get secret"* ]]; then
	name=${@: -3:1}
  if [[ ${MOCK_GET_FAIL:-} == "$name" ]]; then
    printf 'error: unable to connect to server\n' >&2
    exit 1
  fi
  if [[ ! -f "$MOCK_SECRET_DIR/$name.json" ]]; then
    printf 'Error from server (NotFound): secrets "%s" not found\n' "$name" >&2
    exit 1
  fi
  if [[ ${MOCK_DECODE_FAIL:-} == "$name" ]]; then
    jq '.data["token-sha256"] = "%%%"' "$MOCK_SECRET_DIR/$name.json"
  else
    cat "$MOCK_SECRET_DIR/$name.json"
  fi
  exit 0
fi
if [[ $* == *"delete secret"* ]]; then
  name=${@: -2:1}
  rm -f "$MOCK_SECRET_DIR/$name.json"
  exit 0
fi
name=$(jq -r '.metadata.name' "$manifest")
if [[ ${MOCK_FAIL_APPLY_NAME:-} == "$name" && ! -e "$MOCK_SECRET_DIR/fail-once" ]]; then
  touch "$MOCK_SECRET_DIR/fail-once"
  exit 1
fi
cp "$manifest" "$MOCK_SECRET_DIR/$name.json"
MOCK
chmod +x "$test_root/bin/kubectl"
export PATH="$test_root/bin:$PATH"
export MOCK_SECRET_DIR="$test_root"
export R="$test_root/runtime"
export KUBECONFIG="$test_root/kubeconfig"

provision() {
  bash "$repo_root/astribot/release/iam-provision.sh" "$@"
}
secret_digest() {
  jq -r '.data["token-sha256"]' "$test_root/rlark-service-account-astribot-iam-admin.json" | base64 -d
}
token() {
  jq -r '.admin' "$R/iam/upstream-tokens.json"
}

bootstrap_output=$(provision)
[[ $bootstrap_output == *'Restart astribot-iam to load the new tokens.'* ]]
first_token=$(token)
first_digest=$(secret_digest)
[[ $(printf '%s' "$first_token" | sha256sum | cut -d ' ' -f1) == "$first_digest" ]]
[[ $(stat -c '%a' "$R/iam/upstream-tokens.json") == 600 ]]
for spec in 'admin admin' 'operator operator' 'user user'; do
  read -r key role <<<"$spec"
  name="astribot-iam-$key"
  secret="$test_root/rlark-service-account-$name.json"
  [[ $(jq -r '.metadata.labels["rlark.io/service-account"]' "$secret") == true ]]
  [[ $(jq -r '.data.role | @base64d' "$secret") == "$role" ]]
  [[ $(jq -r '.data["tenant-ids"] | @base64d' "$secret") == default ]]
  token_value=$(jq -r --arg key "$key" '.[$key]' "$R/iam/upstream-tokens.json")
  [[ $token_value == "rlsa.$name."* ]]
done
provision
[[ $(token) == "$first_token" ]]

for failure in get decode; do
  case $failure in
    get) export MOCK_GET_FAIL=rlark-service-account-astribot-iam-admin ;;
    decode) export MOCK_DECODE_FAIL=rlark-service-account-astribot-iam-admin ;;
  esac
  before=$(token)
  before_digest=$(secret_digest)
  rm -f "$test_root/fail-once"
  if provision --rotate-upstream-tokens >/dev/null; then
    printf 'expected %s failure to abort provisioning\n' "$failure" >&2
    exit 1
  fi
  [[ $(token) == "$before" ]]
  [[ $(secret_digest) == "$before_digest" ]]
  unset MOCK_GET_FAIL MOCK_DECODE_FAIL
done

rm "$R/iam/upstream-tokens.json"
provision
recovered_token=$(token)
[[ $recovered_token != "$first_token" ]]
[[ $(printf '%s' "$recovered_token" | sha256sum | cut -d ' ' -f1) == "$(secret_digest)" ]]
old_token_file=$(sha256sum "$R/iam/upstream-tokens.json" | cut -d ' ' -f1)
old_admin_digest=$(secret_digest)
old_operator_digest=$(jq -r '.data["token-sha256"] | @base64d' "$test_root/rlark-service-account-astribot-iam-operator.json")
export MOCK_FAIL_APPLY_NAME=rlark-service-account-astribot-iam-operator
if provision --rotate-upstream-tokens >/dev/null 2>&1; then
  printf 'expected simulated partial rotation failure\n' >&2
  exit 1
fi
unset MOCK_FAIL_APPLY_NAME
rm -f "$test_root/fail-once"
[[ $(sha256sum "$R/iam/upstream-tokens.json" | cut -d ' ' -f1) == "$old_token_file" ]]
[[ $(secret_digest) == "$old_admin_digest" ]]
[[ $(jq -r '.data["token-sha256"] | @base64d' "$test_root/rlark-service-account-astribot-iam-operator.json") == "$old_operator_digest" ]]
provision >/dev/null
recovered_token=$(token)
[[ $(printf '%s' "$recovered_token" | sha256sum | cut -d ' ' -f1) == "$(secret_digest)" ]]
provision --rotate-upstream-tokens
rotated_token=$(token)
[[ $rotated_token != "$recovered_token" ]]
[[ $(printf '%s' "$rotated_token" | sha256sum | cut -d ' ' -f1) == "$(secret_digest)" ]]
printf 'iam-provision bootstrap, idempotence, recovery, and rotation passed.\n'
