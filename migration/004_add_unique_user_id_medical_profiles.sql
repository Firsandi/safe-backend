-- Clean up duplicates: keep only the newest medical profile for each user, delete other duplicates
DELETE FROM medical_profiles
WHERE medical_id NOT IN (
    SELECT DISTINCT ON (user_id) medical_id
    FROM medical_profiles
    ORDER BY user_id, medical_id DESC
);

-- Add UNIQUE constraint to user_id in medical_profiles table
ALTER TABLE medical_profiles
ADD CONSTRAINT unique_user_id UNIQUE (user_id);
