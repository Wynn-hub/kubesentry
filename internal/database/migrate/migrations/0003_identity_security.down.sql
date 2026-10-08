SET search_path TO kubesentry;

DROP TABLE security_settings;
DROP TABLE password_history;
DROP TABLE user_activation_tokens;

ALTER TABLE users
    DROP COLUMN locked_until,
    DROP COLUMN failed_login_count,
    DROP COLUMN must_change_password,
    DROP COLUMN password_changed_at,
    DROP COLUMN last_login_ip,
    DROP COLUMN last_login_at;
