-- 移除“记住我”登录：浏览器登录统一使用 15 天服务端会话，删除记录即可吊销。
DROP TABLE IF EXISTS user_remember_tokens;
DROP INDEX IF EXISTS idx_user_sessions_remember_family_id;
ALTER TABLE user_sessions DROP COLUMN IF EXISTS remember_family_id;
