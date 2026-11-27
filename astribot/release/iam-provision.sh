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
exec 8>"$iam_dir/.upstream-tokens.lock"
flock 8
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

if [[ ${1:-} == --rotate-upstream-tokens ]]; then
	force_rotate=true
else
	force_rotate=false
fi
namespace=${RLARK_MANAGEMENT_NAMESPACE:-default}
accounts=('astribot-iam-admin admin admin' 'astribot-iam-operator operator operator' 'astribot-iam-user user user')
declare -A old_exists old_digest

# Snapshot every old object before generating or applying any replacement.
for entry in "${accounts[@]}"; do
	read -r name role token_key <<<"$entry"
	secret_name="rlark-service-account-$name"
	if kubectl --kubeconfig "$management_kubeconfig" -n "$namespace" get secret "$secret_name" -o json \
		>"$work_dir/$name.old.json" 2>"$work_dir/$name.get-error"; then
		old_exists[$name]=true
		if ! jq -e '.data["token-sha256"] | strings' "$work_dir/$name.old.json" >/dev/null; then
			printf 'Existing service account %s has no token-sha256 digest; refusing to continue.\n' "$name" >&2
			exit 1
		fi
		if ! old_digest[$name]=$(jq -r '.data["token-sha256"]' "$work_dir/$name.old.json" | base64 --decode 2>"$work_dir/$name.decode-error"); then
			printf 'Could not decode token-sha256 for service account %s.\n' "$name" >&2
			cat "$work_dir/$name.decode-error" >&2
			exit 1
		fi
	else
		if ! grep -Fqi 'Error from server (NotFound): secrets' "$work_dir/$name.get-error"; then
			printf 'Could not read Gateway service account %s in namespace %s.\n' "$name" "$namespace" >&2
			cat "$work_dir/$name.get-error" >&2
			exit 1
		fi
		old_exists[$name]=false
		old_digest[$name]=""
		rm -f "$work_dir/$name.old.json"
	fi
done

if [[ -L $tokens_file || -e $tokens_file ]]; then
	if [[ -L $tokens_file || ! -f $tokens_file || $(stat -c '%a' "$tokens_file") != 600 ]] ||
		! jq -e '(.admin | type == "string" and length > 0) and (.operator | type == "string" and length > 0) and (.user | type == "string" and length > 0)' "$tokens_file" >/dev/null; then
		printf 'Existing IAM upstream token file is invalid or has unsafe permissions; refusing to rotate.\n' >&2
		exit 1
	fi
fi

if [[ $force_rotate == false && -f $tokens_file ]]; then
	all_tokens_match=true
	for entry in "${accounts[@]}"; do
		read -r name role token_key <<<"$entry"
		token=$(jq -r --arg key "$token_key" '.[$key]' "$tokens_file")
		expected_digest=$(printf '%s' "$token" | sha256sum | cut -d ' ' -f1)
		if [[ ${old_exists[$name]} != true || ${old_digest[$name]} != "$expected_digest" ]]; then
			all_tokens_match=false
		fi
	done
	if [[ $all_tokens_match == true ]]; then
		printf 'IAM upstream tokens are already provisioned. Use --rotate-upstream-tokens to rotate them.\n'
		exit 0
	fi
fi

# Gateway service-account credentials are random bearer tokens. Only their
# SHA-256 digests are written to the management namespace.
for entry in "${accounts[@]}"; do
	read -r name role token_key <<<"$entry"
	random=$(openssl rand -base64 32 | tr '+/' '-_' | tr -d '=\n')
	token="rlsa.$name.$random"
	digest=$(printf '%s' "$token" | sha256sum | cut -d ' ' -f1)
	printf '%s' "$token" >"$work_dir/token-$token_key"
	chmod 600 "$work_dir/token-$token_key"
	if [[ ${old_exists[$name]} == true ]]; then
		jq --arg digest "$digest" '.data["token-sha256"] = ($digest | @base64)' "$work_dir/$name.old.json" >"$work_dir/$name.json"
	else
		jq -n --arg name "$name" --arg role "$role" --arg digest "$digest" --arg namespace "$namespace" \
			'{apiVersion:"v1",kind:"Secret",metadata:{name:("rlark-service-account-"+$name),namespace:$namespace,labels:{"rlark.io/service-account":"true"}},type:"Opaque",data:{name:($name|@base64),description:("Astribot IAM "+$role|@base64),role:($role|@base64),"tenant-ids":("default"|@base64),"token-sha256":($digest|@base64)}}' \
			>"$work_dir/$name.json"
	fi
	unset token random digest
done

attempted=()
for entry in "${accounts[@]}"; do
	read -r name role token_key <<<"$entry"
	attempted+=("$name")
	if kubectl --kubeconfig "$management_kubeconfig" apply -f "$work_dir/$name.json" >/dev/null; then
		continue
	fi
	printf 'Could not provision Gateway service account %s in namespace %s; restoring prior Secret state.\n' "$name" "$namespace" >&2
	rollback_failed=false
	for rollback_name in "${attempted[@]}"; do
		if [[ ${old_exists[$rollback_name]} == true ]]; then
			if ! kubectl --kubeconfig "$management_kubeconfig" apply -f "$work_dir/$rollback_name.old.json" >/dev/null; then
				printf 'Failed to restore prior Secret %s.\n' "$rollback_name" >&2
				rollback_failed=true
			fi
		elif ! kubectl --kubeconfig "$management_kubeconfig" -n "$namespace" delete secret "rlark-service-account-$rollback_name" --ignore-not-found >/dev/null; then
			printf 'Failed to remove newly created Secret %s.\n' "$rollback_name" >&2
			rollback_failed=true
		fi
	done
	if [[ $rollback_failed == true ]]; then
		printf 'Secret rollback was incomplete; inspect the management namespace before retrying.\n' >&2
	fi
	exit 1
done

jq -n --rawfile admin "$work_dir/token-admin" --rawfile operator "$work_dir/token-operator" --rawfile user "$work_dir/token-user" \
	'{admin:$admin,operator:$operator,user:$user}' >"$work_dir/upstream-tokens.json"
chmod 600 "$work_dir/upstream-tokens.json"
mv -f "$work_dir/upstream-tokens.json" "$tokens_file"
printf 'Provisioned IAM upstream service accounts and wrote %s with mode 0600. Restart astribot-iam to load the new tokens.\n' "$tokens_file"
