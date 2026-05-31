#!/usr/bin/env bash
set -euo pipefail

PGHOST="${PG_HOST:-localhost}"
PGPORT="${PG_PORT:-5432}"
PGUSER="${PG_USER:-postgres}"
PGPASSWORD="${PG_PASS:-postgres}"
BASE_DB="${PG_BASE_DB:-postgres}"
CHECK_DB="${MIGRATION_CHECK_DB:-agent_os_migration_check_$(date +%s)}"
KEEP_DB="${KEEP_MIGRATION_CHECK_DB:-0}"
REQUIRE_VECTOR="${MIGRATION_REQUIRE_VECTOR:-1}"

export PGPASSWORD

require_tool() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required tool: $1" >&2
    exit 1
  fi
}

psql_base() {
  if [[ "$PSQL_MODE" == "docker" ]]; then
    docker compose exec -T postgres psql -v ON_ERROR_STOP=1 -U "$PGUSER" -d "$BASE_DB" "$@"
  else
    psql -v ON_ERROR_STOP=1 -h "$PGHOST" -p "$PGPORT" -U "$PGUSER" -d "$BASE_DB" "$@"
  fi
}

psql_check() {
  if [[ "$PSQL_MODE" == "docker" ]]; then
    docker compose exec -T postgres psql -v ON_ERROR_STOP=1 -U "$PGUSER" -d "$CHECK_DB" "$@"
  else
    psql -v ON_ERROR_STOP=1 -h "$PGHOST" -p "$PGPORT" -U "$PGUSER" -d "$CHECK_DB" "$@"
  fi
}

cleanup() {
  if [[ "$KEEP_DB" == "1" ]]; then
    echo "keeping migration check database: ${CHECK_DB}"
    return
  fi
  psql_base -qAt -c "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = '${CHECK_DB}' AND pid <> pg_backend_pid();" >/dev/null 2>&1 || true
  psql_base -qAt -c "DROP DATABASE IF EXISTS \"${CHECK_DB}\";" >/dev/null 2>&1 || true
}
trap cleanup EXIT

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

PSQL_MODE="local"
if ! command -v psql >/dev/null 2>&1; then
  require_tool docker
  PSQL_MODE="docker"
fi

if [[ ! -d migrations ]]; then
  echo "migrations directory not found" >&2
  exit 1
fi

mapfile -t migrations < <(find migrations -maxdepth 1 -type f -name '[0-9][0-9][0-9]_*.sql' | sort)
if [[ "${#migrations[@]}" -eq 0 ]]; then
  echo "no migrations found" >&2
  exit 1
fi

echo "==> Creating disposable migration check database ${CHECK_DB}"
psql_base -qAt -c "DROP DATABASE IF EXISTS \"${CHECK_DB}\";" >/dev/null
psql_base -qAt -c "CREATE DATABASE \"${CHECK_DB}\";" >/dev/null

if [[ "$REQUIRE_VECTOR" == "1" ]]; then
  echo "==> Checking pgvector extension availability"
  if ! psql_check -qAt -c "CREATE EXTENSION IF NOT EXISTS vector;" >/dev/null; then
    echo "pgvector extension is required for migration 016. Install pgvector or set MIGRATION_REQUIRE_VECTOR=0 only for syntax-only environments." >&2
    exit 1
  fi
fi

# gen_random_uuid() is required by the historical SQL migrations. PostgreSQL 13+
# exposes it from pgcrypto; newer versions may have it built-in, but this keeps
# local/dev databases deterministic.
echo "==> Preparing pgcrypto extension"
psql_check -qAt -c "CREATE EXTENSION IF NOT EXISTS pgcrypto;" >/dev/null

psql_check -qAt -c "CREATE TABLE IF NOT EXISTS schema_migrations (version TEXT PRIMARY KEY, filename TEXT NOT NULL, applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW());" >/dev/null

previous=""
for migration in "${migrations[@]}"; do
  file="$(basename "$migration")"
  version="${file%%_*}"
  if [[ -n "$previous" ]] && (( 10#$version <= 10#$previous )); then
    echo "migration versions must be strictly increasing: ${previous} then ${version}" >&2
    exit 1
  fi
  previous="$version"
  echo "==> Applying ${file}"
  if [[ "$PSQL_MODE" == "docker" ]]; then
    psql_check < "$migration" >/dev/null
  else
    psql_check -f "$migration" >/dev/null
  fi
  psql_check -qAt -c "INSERT INTO schema_migrations(version, filename) VALUES ('${version}', '${file}');" >/dev/null
done

applied_count="$(psql_check -qAt -c 'SELECT COUNT(*) FROM schema_migrations;')"
if [[ "$applied_count" != "${#migrations[@]}" ]]; then
  echo "expected ${#migrations[@]} applied migrations, got ${applied_count}" >&2
  exit 1
fi

printf '\nMigration apply check passed (%s migrations).\n' "$applied_count"
