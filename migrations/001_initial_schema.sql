-- Users table
CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email VARCHAR(255) UNIQUE NOT NULL,
    username VARCHAR(50) UNIQUE NOT NULL,
    native_languages TEXT[] NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Chat sessions table
CREATE TABLE chat_sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_a_id UUID NOT NULL REFERENCES users(id),
    user_b_id UUID NOT NULL REFERENCES users(id),
    language_a VARCHAR(10) NOT NULL, -- A's native language being practiced
    language_b VARCHAR(10) NOT NULL, -- B's native language being practiced
    started_at TIMESTAMP WITH TIME ZONE,
    ended_at TIMESTAMP WITH TIME ZONE,
    duration_minutes INTEGER,
    status VARCHAR(20) NOT NULL DEFAULT 'waiting',
    completed_minimum BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Match requests (temporary storage, main queue in Redis)
CREATE TABLE match_requests (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id),
    native_languages TEXT[] NOT NULL,
    practice_language VARCHAR(10) NOT NULL,
    requested_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending'
);

-- Indexes for performance
CREATE INDEX idx_chat_sessions_users ON chat_sessions(user_a_id, user_b_id);
CREATE INDEX idx_chat_sessions_status ON chat_sessions(status);
CREATE INDEX idx_match_requests_user ON match_requests(user_id);
CREATE INDEX idx_match_requests_status ON match_requests(status);
CREATE INDEX idx_match_requests_expires ON match_requests(expires_at);