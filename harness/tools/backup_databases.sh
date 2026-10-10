#!/usr/bin/env bash
# Online logical backups; never includes unrelated Penpot/MySQL databases.
set -euo pipefail
umask 077
backup_parent=${1:?Usage: backup_databases.sh /absolute/private/backup/directory}
[[ "$backup_parent" = /* ]] || { echo 'Use an absolute backup directory' >&2; exit 1; }
mkdir -p "$backup_parent"
chmod 700 "$backup_parent"
backup_dir=$(mktemp -d "$backup_parent/snapshot-$(date -u +%Y%m%dT%H%M%SZ)-XXXXXX")
for container in docker-compose-postgres-1 media-agent-auth-auth-postgres-1; do
  mkdir "$backup_dir/$container"
  docker exec "$container" sh -c 'pg_dumpall -U "$POSTGRES_USER" --globals-only' > "$backup_dir/$container/globals.sql"
  mapfile -t databases < <(docker exec "$container" sh -c 'psql -U "$POSTGRES_USER" -d postgres -Atc "SELECT datname FROM pg_database WHERE NOT datistemplate ORDER BY datname"')
  ((${#databases[@]})) || { echo 'Database discovery failed' >&2; exit 1; }
  for database in "${databases[@]}"; do
    [[ "$database" =~ ^[a-zA-Z0-9_-]+$ ]] || { echo 'Unsupported database name' >&2; exit 1; }
    docker exec "$container" sh -c 'pg_dump -U "$POSTGRES_USER" -Fc --dbname="$1"' sh "$database" > "$backup_dir/$container/$database.dump"
    docker exec -i "$container" pg_restore --list < "$backup_dir/$container/$database.dump" > "$backup_dir/$container/$database.toc"
    [[ -s "$backup_dir/$container/$database.dump" && -s "$backup_dir/$container/$database.toc" ]]
    printf 'Backed up %s/%s\n' "$container" "$database"
  done
done
# SQLite's online backup API includes committed WAL data, unlike copying .db.
python3 - "$backup_dir/conductor.db" <<'PY'
import sqlite3, sys
with sqlite3.connect('file:/data/conductor/conductor.db?mode=ro', uri=True, timeout=30) as source:
    with sqlite3.connect(sys.argv[1]) as destination:
        source.backup(destination)
        if destination.execute('PRAGMA integrity_check').fetchall() != [('ok',)]:
            raise SystemExit('Conductor backup integrity check failed')
PY
find "$backup_dir" -type f -exec chmod 600 {} +
(cd "$backup_dir" && find . -type f ! -name SHA256SUMS -print0 | sort -z | xargs -0 sha256sum > SHA256SUMS)
printf 'BACKUP_DIRECTORY=%s\n' "$backup_dir"
