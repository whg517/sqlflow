-- DingTalk OAuth columns for users table
ALTER TABLE users ADD COLUMN dingtalk_user_id TEXT DEFAULT '';
ALTER TABLE users ADD COLUMN dingtalk_union_id TEXT DEFAULT '';
