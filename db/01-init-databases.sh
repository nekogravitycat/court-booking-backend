#!/bin/bash
set -e

# This script only ensures the databases exist. Schema is no longer applied
# here: the application (and the test harness) run golang-migrate migrations
# against whichever database they connect to. See db/migrations/.

echo "Starting custom database initialization..."

# Create the Test Database if it does not exist
if [ -n "$POSTGRES_TEST_DB" ]; then
  echo "Creating database: $POSTGRES_TEST_DB"
  psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<-EOSQL
    SELECT 'CREATE DATABASE $POSTGRES_TEST_DB'
    WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = '$POSTGRES_TEST_DB')\gexec
EOSQL
else
  # Warn if POSTGRES_TEST_DB is not set
  # Skip test database creation
  echo "----------------------------------------------------------------"
  echo "WARNING: POSTGRES_TEST_DB environment variable is not set!"
  echo "         Skipping test database creation."
  echo "----------------------------------------------------------------"
fi

echo "Custom database initialization finished."
