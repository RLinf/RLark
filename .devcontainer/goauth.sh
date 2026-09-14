#!/bin/bash

host=gitlab.infini-ai.com
result=$(echo -e "protocol=https\nhost=$host" | git credential fill 2>/dev/null)
username=$(echo "$result" | grep "^username=" | cut -d'=' -f2)
password=$(echo "$result" | grep "^password=" | cut -d'=' -f2)
if [ -z "$username" ] || [ -z "$password" ]; then
    exit 1
fi

echo "https://$host"
echo ""
echo "Authorization: Basic $(echo -n "$username:$password" | base64)"
echo ""
