#!/usr/bin/env bash
set -euo pipefail

: "${R:?set R to the runtime directory from controlPlane.variables.R}"

gateway_url=${ASTRIBOT_IAM_GATEWAY_URL:-http://127.0.0.1:19000}
management_kubeconfig=${KUBECONFIG:-$R/identity/rlark-release/kubeconfig}
iam_dir=$R/iam
tokens_file=$iam_dir/upstream-tokens.json
signing_key=$iam_dir/submission-private.pem
verification_key=$iam_dir/submission-public.pem

umask 077
mkdir -p "$iam_dir"
chmod 700 "$iam_dir"
work_dir=$(mktemp -d "$iam_dir/.provision.XXXXXX")
trap 'rm -rf "$work_dir"' EXIT

# Publish the signing key first. If interrupted, the public key can be derived
# on the next run without replacing the original signing key.
(
	flock 9
	if [[ ! -e $signing_key ]]; then
		if [[ -e $verification_key ]]; then
			printf 'The public submission key exists without its signing key. Restore the matching private key before proceeding.\n' >&2
			exit 1
		fi
		openssl genpkey -algorithm Ed25519 -out "$work_dir/submission-private.pem"
		chmod 600 "$work_dir/submission-private.pem"
		mv "$work_dir/submission-private.pem" "$signing_key"
	fi
	openssl pkey -in "$signing_key" -pubout -out "$work_dir/submission-public.pem"
	if [[ ! -e $verification_key ]]; then
		mv "$work_dir/submission-public.pem" "$verification_key"
	elif ! cmp -s "$work_dir/submission-public.pem" "$verification_key"; then
		printf 'IAM submission keys do not match. Restore the matching key pair before proceeding.\n' >&2
		exit 1
	fi
) 9>"$iam_dir/.submission-keys.lock"

if [[ ${1:-} == --submission-keys-only ]]; then
	printf 'IAM submission keys are provisioned.\n'
	exit 0
fi

if [[ -f $tokens_file && ! -L $tokens_file && $(stat -c '%a' "$tokens_file") == 600 ]] &&
  jq -e '(.admin | type == "string" and length > 0) and (.operator | type == "string" and length > 0) and (.user | type == "string" and length > 0)' "$tokens_file" >/dev/null; then
	printf 'IAM upstream tokens are already provisioned.\n'
	exit 0
fi

manual_commands() {
	printf 'export GATEWAY_URL=%q\n' "$gateway_url" >&2
	cat >&2 <<'EOF'
IAM service-account provisioning did not complete. No token file was written.
The Gateway supports one-time token creation and rotation at these endpoints.
After obtaining an admin Gateway JWT, run these commands with GATEWAY_TOKEN set:
  GATEWAY_TOKEN=$(ADMIN_PASSWORD="$ADMIN_PASSWORD" jq -n '{username:"admin",password:env.ADMIN_PASSWORD}' | curl -fsS -H 'Content-Type: application/json' --data-binary @- "$GATEWAY_URL/api/v1/auth/login" | jq -r '.token')
  curl -fsS -H "Authorization: Bearer $GATEWAY_TOKEN" -H 'Content-Type: application/json' -d '{"name":"astribot-iam-admin","role":"admin"}' "$GATEWAY_URL/api/v1/service-accounts"
  curl -fsS -H "Authorization: Bearer $GATEWAY_TOKEN" -H 'Content-Type: application/json' -d '{"name":"astribot-iam-operator","role":"operator"}' "$GATEWAY_URL/api/v1/service-accounts"
  curl -fsS -H "Authorization: Bearer $GATEWAY_TOKEN" -H 'Content-Type: application/json' -d '{"name":"astribot-iam-user","role":"user"}' "$GATEWAY_URL/api/v1/service-accounts"
  curl -fsS -X POST -H "Authorization: Bearer $GATEWAY_TOKEN" "$GATEWAY_URL/api/v1/service-accounts/astribot-iam-admin/token"
  curl -fsS -X POST -H "Authorization: Bearer $GATEWAY_TOKEN" "$GATEWAY_URL/api/v1/service-accounts/astribot-iam-operator/token"
  curl -fsS -X POST -H "Authorization: Bearer $GATEWAY_TOKEN" "$GATEWAY_URL/api/v1/service-accounts/astribot-iam-user/token"
Use the create commands for absent accounts and the rotation commands for existing accounts. Capture each returned token directly into the corresponding JSON field; do not put tokens in shell history.
EOF
}

if ! admin_password=$(kubectl --kubeconfig "$management_kubeconfig" -n default get secret rlark-ui-auth -o jsonpath='{.data.admin-password}' | base64 --decode); then
	printf 'Could not read admin-password from default/rlark-ui-auth using the release kubeconfig.\n' >&2
	manual_commands
	exit 1
fi
if [[ -z $admin_password ]]; then
	printf 'default/rlark-ui-auth has no admin-password.\n' >&2
	manual_commands
	exit 1
fi

ADMIN_PASSWORD=$admin_password jq -n '{username:"admin",password:env.ADMIN_PASSWORD}' >"$work_dir/login.json"
unset admin_password
if ! curl -fsS -H 'Content-Type: application/json' --data-binary "@$work_dir/login.json" \
	"$gateway_url/api/v1/auth/login" -o "$work_dir/login-response.json"; then
	printf 'Could not log in to the Gateway at %s.\n' "$gateway_url" >&2
	manual_commands
	exit 1
fi
rm -f "$work_dir/login.json"
gateway_token=$(jq -er '.token | select(type == "string" and length > 0)' "$work_dir/login-response.json") || {
	printf 'The Gateway login response did not contain a token.\n' >&2
	manual_commands
	exit 1
}
rm -f "$work_dir/login-response.json"
printf 'header = "Authorization: Bearer %s"\n' "$gateway_token" >"$work_dir/curl-auth.conf"
chmod 600 "$work_dir/curl-auth.conf"

declare -A tokens
for account in 'astribot-iam-admin admin' 'astribot-iam-operator operator' 'astribot-iam-user user'; do
	read -r name role <<<"$account"
	jq -n --arg name "$name" --arg role "$role" \
		'{name:$name,role:$role}' >"$work_dir/request.json"
	if ! status=$(curl -sS -o "$work_dir/response.json" -w '%{http_code}' \
		--config "$work_dir/curl-auth.conf" -H 'Content-Type: application/json' \
		--data-binary "@$work_dir/request.json" "$gateway_url/api/v1/service-accounts"); then
		printf 'Gateway service-account creation request failed for %s.\n' "$name" >&2
		manual_commands
		exit 1
	fi
	if [[ $status == 409 ]]; then
		if ! curl -fsS --config "$work_dir/curl-auth.conf" \
			"$gateway_url/api/v1/service-accounts" -o "$work_dir/accounts.json" ||
			! jq -e --arg name "$name" --arg role "$role" 'any(.data[]; .name == $name and .role == $role)' "$work_dir/accounts.json" >/dev/null; then
			printf 'Existing service account %s is missing or has a different role; refusing to rotate it.\n' "$name" >&2
			manual_commands
			exit 1
		fi
		if ! status=$(curl -sS -o "$work_dir/response.json" -w '%{http_code}' -X POST \
			--config "$work_dir/curl-auth.conf" "$gateway_url/api/v1/service-accounts/$name/token"); then
			printf 'Gateway token rotation request failed for %s.\n' "$name" >&2
			manual_commands
			exit 1
		fi
	fi
	if [[ $status != 201 && $status != 200 ]]; then
		printf 'Gateway returned HTTP %s while provisioning %s.\n' "$status" "$name" >&2
		manual_commands
		exit 1
	fi
	token=$(jq -er '.token | select(type == "string" and length > 0)' "$work_dir/response.json") || {
		printf 'Gateway did not return a token for %s.\n' "$name" >&2
		manual_commands
		exit 1
	}
	case $role in
		admin) tokens[admin]=$token ;;
		operator) tokens[operator]=$token ;;
		user) tokens[user]=$token ;;
	esac
	unset token
	printf 'Provisioned Gateway service account %s.\n' "$name"
done
unset gateway_token

IAM_ADMIN_TOKEN=${tokens[admin]} IAM_OPERATOR_TOKEN=${tokens[operator]} IAM_USER_TOKEN=${tokens[user]} \
	jq -n '{admin:env.IAM_ADMIN_TOKEN,operator:env.IAM_OPERATOR_TOKEN,user:env.IAM_USER_TOKEN}' >"$work_dir/upstream-tokens.json"
chmod 600 "$work_dir/upstream-tokens.json"
mv -f "$work_dir/upstream-tokens.json" "$tokens_file"
printf 'Wrote %s with mode 0600.\n' "$tokens_file"
