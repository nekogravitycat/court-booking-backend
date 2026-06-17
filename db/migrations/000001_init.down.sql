-- Down migration for the initial schema baseline.
-- Tears the entire public schema back down to empty. Because 000001 is the
-- consolidated baseline, the only meaningful reverse is a full teardown.
DROP SCHEMA public CASCADE;
CREATE SCHEMA public;
GRANT ALL ON SCHEMA public TO public;
