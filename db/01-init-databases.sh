#!/bin/bash
set -e

# Path to the schema file in the container
SCHEMA_FILE="/docker-entrypoint-initdb.d/schema.sql"

echo "Starting custom database initialization..."

# 1. Create the Test Database if it does not exist
if [ -n "$POSTGRES_TEST_DB" ]; then
    echo "Creating database: $POSTGRES_TEST_DB"
    psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<-EOSQL
        SELECT 'CREATE DATABASE $POSTGRES_TEST_DB'
        WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = '$POSTGRES_TEST_DB')\gexec
EOSQL

    # 2. Apply schema to the Test Database
    # Note: The main database ($POSTGRES_DB) will typically apply schema.sql automatically
    # because the file exists in the /docker-entrypoint-initdb.d directory.
    echo "Applying schema to test database: $POSTGRES_TEST_DB"
    psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_TEST_DB" -f "$SCHEMA_FILE"
fi

echo "Custom database initialization finished."
