-- +goose Up
-- Create languages table for storing supported languages
CREATE TABLE IF NOT EXISTS languages (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL UNIQUE,
    short_name VARCHAR(10) NOT NULL UNIQUE,
    is_active BOOLEAN DEFAULT true,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Create indexes for better performance on languages table
CREATE INDEX IF NOT EXISTS idx_languages_name ON languages(name);
CREATE INDEX IF NOT EXISTS idx_languages_short_name ON languages(short_name);
CREATE INDEX IF NOT EXISTS idx_languages_is_active ON languages(is_active);

-- Create updated_at trigger function
CREATE OR REPLACE FUNCTION update_updated_at_column() RETURNS TRIGGER AS 'BEGIN NEW.updated_at = CURRENT_TIMESTAMP; RETURN NEW; END;' LANGUAGE plpgsql;

-- Create trigger to automatically update updated_at column for languages
CREATE TRIGGER update_languages_updated_at
    BEFORE UPDATE ON languages
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- Insert initial supported languages
INSERT INTO languages (name, short_name) VALUES
    ('English', 'EN'),
    ('Spanish', 'ES'),
    ('French', 'FR'),
    ('German', 'DE'),
    ('Italian', 'IT'),
    ('Portuguese', 'PT'),
    ('Russian', 'RU'),
    ('Chinese', 'ZH'),
    ('Japanese', 'JA'),
    ('Korean', 'KO'),
    ('Arabic', 'AR'),
    ('Hindi', 'HI'),
    ('Dutch', 'NL'),
    ('Swedish', 'SV'),
    ('Norwegian', 'NO'),
    ('Danish', 'DA'),
    ('Finnish', 'FI'),
    ('Polish', 'PL'),
    ('Czech', 'CS'),
    ('Turkish', 'TR')
ON CONFLICT (name) DO NOTHING;

-- Create sessions table for storing language exchange session data
CREATE TABLE IF NOT EXISTS sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user1_id VARCHAR(255) NOT NULL,
    user2_id VARCHAR(255) NOT NULL,
    user1_native_language VARCHAR(50) NOT NULL,
    user1_practice_language VARCHAR(50) NOT NULL,
    user2_native_language VARCHAR(50) NOT NULL,
    user2_practice_language VARCHAR(50) NOT NULL,
    status VARCHAR(20) DEFAULT 'active' CHECK (status IN ('active', 'completed', 'cancelled')),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    ended_at TIMESTAMP WITH TIME ZONE,
    duration_seconds INTEGER
);

-- Create indexes for better performance on sessions table
CREATE INDEX IF NOT EXISTS idx_sessions_status ON sessions(status);
CREATE INDEX IF NOT EXISTS idx_sessions_created_at ON sessions(created_at);
CREATE INDEX IF NOT EXISTS idx_sessions_user1_id ON sessions(user1_id);
CREATE INDEX IF NOT EXISTS idx_sessions_user2_id ON sessions(user2_id);

-- Create trigger to automatically update updated_at column for sessions
CREATE TRIGGER update_sessions_updated_at
    BEFORE UPDATE ON sessions
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- +goose Down
-- Drop sessions table and related objects
DROP TRIGGER IF EXISTS update_sessions_updated_at ON sessions;
DROP INDEX IF EXISTS idx_sessions_status;
DROP INDEX IF EXISTS idx_sessions_created_at;
DROP INDEX IF EXISTS idx_sessions_user1_id;
DROP INDEX IF EXISTS idx_sessions_user2_id;
DROP TABLE IF EXISTS sessions;

-- Drop languages table and related objects
DROP TRIGGER IF EXISTS update_languages_updated_at ON languages;
DROP INDEX IF EXISTS idx_languages_name;
DROP INDEX IF EXISTS idx_languages_short_name;
DROP INDEX IF EXISTS idx_languages_is_active;
DROP TABLE IF EXISTS languages;

-- Drop the trigger function
DROP FUNCTION IF EXISTS update_updated_at_column();