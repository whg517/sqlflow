-- 000004: Track when a user's password was last changed.
-- The JWT middleware compares a token's iat claim against this column;
-- a token issued before the most recent password reset is rejected,
-- so "reset password" actually cuts off already-issued access tokens.

-- Add the column with a default of now() so existing users get a value
-- that does not invalidate their current tokens retroactively.
ALTER TABLE users ADD COLUMN IF NOT EXISTS password_changed_at TIMESTAMPTZ NOT NULL DEFAULT (now());
