#!/usr/bin/env bash
set -euo pipefail
umask 077
backup_dir=${1:?Usage: verify_database_backups.sh /absolute/snapshot/directory}
[[ "$backup_dir" = /* && -f "$backup_dir/SHA256SUMS" ]] || exit 1
(cd "$backup_dir" && sha256sum --check --quiet SHA256SUMS)
for entry in 'docker-compose-postgres-1/media_workflow' 'docker-compose-postgres-1/media-materials' 'media-agent-auth-auth-postgres-1/kratos'; do
  container=${entry%/*}
  database=${entry#*/}
  verify_db="backup_verify_$(date -u +%Y%m%d%H%M%S)_${RANDOM}"
  [[ "$verify_db" =~ ^backup_verify_[0-9_]+$ ]] || exit 1
  docker exec "$container" sh -c 'createdb -U "$POSTGRES_USER" "$1"' sh "$verify_db"
  cleanup() { docker exec "$container" sh -c 'dropdb -U "$POSTGRES_USER" "$1"' sh "$verify_db"; }
  trap cleanup EXIT
  if ! docker exec -i "$container" sh -c 'pg_restore -U "$POSTGRES_USER" --no-owner --no-acl --exit-on-error --dbname="$1"' sh "$verify_db" < "$backup_dir/$container/$database.dump" > "$backup_dir/restore-$database.log" 2>&1; then
    echo "Restore verification failed: $database; private log retained" >&2
    exit 1
  fi
  docker exec "$container" sh -c 'psql -U "$POSTGRES_USER" -d "$1" -Atc "SELECT count(*) FROM pg_tables WHERE schemaname NOT IN ('\''pg_catalog'\'','\''information_schema'\'')"' sh "$verify_db"
  cleanup
  trap - EXIT
  printf 'Restore verified: %s\n' "$database"
done
