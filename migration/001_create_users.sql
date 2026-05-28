-- Enable UUID
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- Users
CREATE TABLE IF NOT EXISTS users (
    user_id    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name       VARCHAR(100) NOT NULL,
    email      VARCHAR(150) UNIQUE NOT NULL,
    password   VARCHAR(255) NOT NULL,
    phone_number VARCHAR(20) NOT NULL,
    fcm_token  TEXT,
    created_at TIMESTAMP DEFAULT NOW()
);

-- Medical profiles
CREATE TABLE IF NOT EXISTS medical_profiles (
    medical_id    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id       UUID NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
    blood_type     VARCHAR(5),
    medical_notes TEXT
);

-- Emergency contacts
CREATE TABLE IF NOT EXISTS emergency_contacts (
    contact_id  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    requester_id UUID NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
    receiver_id  UUID NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
    status       VARCHAR(20) CHECK (status IN ('pending','accepted','rejected')) DEFAULT 'pending'
);

-- SOS events
CREATE TABLE IF NOT EXISTS sos_events (
    sos_id        UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id       UUID NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
    trigger_type  VARCHAR(20) CHECK (trigger_type IN ('manual','auto')) NOT NULL,
    status        VARCHAR(20) CHECK (status IN ('active','resolved','false_alarm')) DEFAULT 'active',
    initial_latitude   DECIMAL(10,7),
    initial_longitude   DECIMAL(10,7),
    medical_snapshot  JSONB,
    created_at    TIMESTAMP DEFAULT NOW()
);

-- SOS tracking
CREATE TABLE IF NOT EXISTS sos_tracking (
    tracking_id  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    sos_id       UUID NOT NULL REFERENCES sos_events(sos_id) ON DELETE CASCADE,
    latitude     DECIMAL(10,7) NOT NULL,
    longitude    DECIMAL(10,7) NOT NULL,
    recorded_at  TIMESTAMP DEFAULT NOW()
);