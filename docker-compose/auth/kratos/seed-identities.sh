#!/bin/sh
set -eu

admin_url="${KRATOS_ADMIN_URL:-http://kratos:4434}"
email="${DEV_USER_EMAIL:-admin@media-agent.local}"
password="${DEV_USER_PASSWORD:-Media-agent-2026}"

until wget -qO /dev/null "$admin_url/health/ready"; do
  sleep 1
done

encoded_email=$(printf '%s' "$email" | sed 's/@/%40/g')
if wget -qO - "$admin_url/admin/identities?credentials_identifier=$encoded_email" | grep -Fq "\"email\":\"$email\""; then
  printf 'Identity already exists: %s\n' "$email"
  exit 0
fi

payload=$(printf '%s' "{
  \"schema_id\": \"default\",
  \"traits\": {
    \"email\": \"$email\",
    \"name\": {\"first\": \"Admin\", \"last\": \"User\"}
  },
  \"metadata_admin\": {\"role\": \"platform_admin\"},
  \"credentials\": {
    \"password\": {\"config\": {\"password\": \"$password\"}}
  }
}")

wget -qO /dev/null \
  --header='Content-Type: application/json' \
  --post-data="$payload" \
  "$admin_url/admin/identities"

printf 'Created local development identity: %s\n' "$email"
