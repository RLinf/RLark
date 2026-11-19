-- Initialize database for persistencer
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Create rlark database (if not exists)
-- SELECT 'CREATE DATABASE rlark' WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'rlark')\gexec

-- Grant permissions
GRANT ALL PRIVILEGES ON DATABASE rlark TO postgres;
