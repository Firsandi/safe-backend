-- Clean up duplicates: keep only the newest medical profile for each user, delete other duplicates
DELETE FROM medical_profiles
WHERE medical_id NOT IN (
    SELECT DISTINCT ON (user_id) medical_id
    FROM medical_profiles
    ORDER BY user_id, medical_id DESC
);

-- Add UNIQUE constraint to user_id in medical_profiles table if it doesn't already exist
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 
        FROM pg_constraint 
        WHERE conname = 'unique_user_id'
          AND conrelid = 'medical_profiles'::regclass
    ) THEN
        ALTER TABLE medical_profiles ADD CONSTRAINT unique_user_id UNIQUE (user_id);
    END IF;
END;
$$;

