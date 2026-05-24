CREATE TABLE IF NOT EXISTS sos_acknowledgements (
    acknowledgement_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    sos_id UUID NOT NULL REFERENCES sos_events(sos_id) ON DELETE CASCADE,
    responder_id UUID NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
    acknowledged_at TIMESTAMP DEFAULT NOW(),
    UNIQUE(sos_id, responder_id)
);
