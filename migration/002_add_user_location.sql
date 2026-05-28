-- Add location tracking columns to users table
ALTER TABLE users ADD COLUMN IF NOT EXISTS latitude DECIMAL(10,7);
ALTER TABLE users ADD COLUMN IF NOT EXISTS longitude DECIMAL(10,7);
ALTER TABLE users ADD COLUMN IF NOT EXISTS location_updated_at TIMESTAMP;
