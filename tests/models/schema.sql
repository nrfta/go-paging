-- Users table for basic pagination tests
CREATE TABLE users (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	email VARCHAR(255) NOT NULL UNIQUE,
	name VARCHAR(255) NOT NULL,
	age INTEGER,
	is_active BOOLEAN DEFAULT true,
	created_at TIMESTAMP NOT NULL DEFAULT NOW(),
	updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

-- Posts table for testing sorting and relationships
CREATE TABLE posts (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	title VARCHAR(500) NOT NULL,
	content TEXT,
	view_count INTEGER DEFAULT 0,
	published_at TIMESTAMP,
	created_at TIMESTAMP NOT NULL DEFAULT NOW(),
	updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

-- Indexes for efficient pagination queries
CREATE INDEX idx_users_created_at ON users(created_at DESC, id DESC);
CREATE INDEX idx_users_email ON users(email);
CREATE INDEX idx_posts_user_id ON posts(user_id);
CREATE INDEX idx_posts_created_at ON posts(created_at DESC, id DESC);
CREATE INDEX idx_posts_published_at ON posts(published_at DESC NULLS LAST, id DESC);
