#!/usr/bin/env bash
#
# set_admin.sh - Grant or revoke the system admin flag (is_system_admin) for a user.
#
# Selects the target user by --id and/or --email. When both are given they are
# combined with AND (the row must match both). Reads database credentials from
# the project .env and runs the UPDATE via psql inside the Postgres container
# (default: court-booking-db, override with the DB_CONTAINER env var).
#
# Usage:
#   ./set_admin.sh -e user@example.com -v true
#   ./set_admin.sh -i 123e4567-e89b-12d3-a456-426614174000 -v false
#   ./set_admin.sh -i <uuid> -e user@example.com          # both must match (AND); value defaults to true
#
# Options:
#   -i, --id <uuid>       Match users.id
#   -e, --email <email>   Match users.email
#   -v, --value <bool>    true|false (default: true)
#   -h, --help            Show this help

set -euo pipefail

usage() {
  # Print the leading comment header (skip the shebang, stop at the first
  # non-comment line).
  awk 'NR==1{next} /^#/{sub(/^# ?/,""); print; next} {exit}' "$0"
  exit "${1:-0}"
}

# Resolve project root (directory of this script) so .env is found regardless of CWD.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

ID=""
EMAIL=""
VALUE="true"

while [[ $# -gt 0 ]]; do
  case "$1" in
    -i|--id)    ID="${2:-}"; shift 2 ;;
    -e|--email) EMAIL="${2:-}"; shift 2 ;;
    -v|--value) VALUE="${2:-}"; shift 2 ;;
    -h|--help)  usage 0 ;;
    *) echo "Unknown argument: $1" >&2; usage 1 ;;
  esac
done

# At least one selector is required.
if [[ -z "$ID" && -z "$EMAIL" ]]; then
  echo "Error: provide at least one of --id or --email." >&2
  usage 1
fi

# Normalize and validate the boolean value.
VALUE="$(echo "$VALUE" | tr '[:upper:]' '[:lower:]')"
if [[ "$VALUE" != "true" && "$VALUE" != "false" ]]; then
  echo "Error: --value must be 'true' or 'false' (got: '$VALUE')." >&2
  exit 1
fi

# Read a single key from .env (last occurrence wins, mirroring dotenv behavior).
read_env() {
  grep -E "^$1=" .env | tail -n1 | cut -d= -f2- || true
}

if [[ ! -f .env ]]; then
  echo "Error: .env not found in $SCRIPT_DIR." >&2
  exit 1
fi

DB_USER="$(read_env POSTGRES_USER)"
DB_PASSWORD="$(read_env POSTGRES_PASSWORD)"
DB_NAME="$(read_env POSTGRES_DB)"
DB_CONTAINER="${DB_CONTAINER:-court-booking-db}"

if [[ -z "$DB_USER" || -z "$DB_NAME" ]]; then
  echo "Error: POSTGRES_USER / POSTGRES_DB missing from .env." >&2
  exit 1
fi

# Build the WHERE clause from the provided selectors. The user-supplied values
# are passed as psql variables (:'uid' / :'email'), which psql quotes and
# escapes safely - so this clause is assembled only from trusted literals.
CONDITIONS=()
HUMAN=()
[[ -n "$ID" ]]    && { CONDITIONS+=("id = :'uid'");       HUMAN+=("id = $ID"); }
[[ -n "$EMAIL" ]] && { CONDITIONS+=("email = :'email'"); HUMAN+=("email = $EMAIL"); }
WHERE=""
WHERE_HUMAN=""
for i in "${!CONDITIONS[@]}"; do
  [[ -n "$WHERE" ]] && { WHERE+=" AND "; WHERE_HUMAN+=" AND "; }
  WHERE+="${CONDITIONS[$i]}"
  WHERE_HUMAN+="${HUMAN[$i]}"
done

SQL="UPDATE public.users
SET is_system_admin = :'val'::boolean
WHERE ${WHERE}
RETURNING id, email, display_name, is_system_admin;"

echo "Setting is_system_admin = ${VALUE} where ${WHERE_HUMAN}..." >&2

# Feed the SQL via stdin (not -c): psql only performs :'var' variable
# interpolation when reading a script from stdin or a file, not with -c.
OUTPUT="$(printf '%s\n' "$SQL" | docker exec -i \
  -e PGPASSWORD="$DB_PASSWORD" \
  "$DB_CONTAINER" \
  psql -U "$DB_USER" -d "$DB_NAME" \
  -v ON_ERROR_STOP=1 \
  --set uid="$ID" --set email="$EMAIL" --set val="$VALUE")"

echo "$OUTPUT"

# psql prints "UPDATE 0" when no row matched.
if echo "$OUTPUT" | grep -q '^UPDATE 0$'; then
  echo "Warning: no matching user found. Nothing was changed." >&2
  exit 2
fi
