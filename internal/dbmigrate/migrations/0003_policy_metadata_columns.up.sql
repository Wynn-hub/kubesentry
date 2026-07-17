SET search_path TO kubesentry;

ALTER TABLE kubesentry.policies ADD COLUMN labels JSONB NOT NULL DEFAULT '{}';
ALTER TABLE kubesentry.policies ADD COLUMN annotations JSONB NOT NULL DEFAULT '{}';

ALTER TABLE kubesentry.policy_groups ADD COLUMN labels JSONB NOT NULL DEFAULT '{}';
ALTER TABLE kubesentry.policy_groups ADD COLUMN annotations JSONB NOT NULL DEFAULT '{}';

ALTER TABLE kubesentry.policy_exceptions ADD COLUMN labels JSONB NOT NULL DEFAULT '{}';
ALTER TABLE kubesentry.policy_exceptions ADD COLUMN annotations JSONB NOT NULL DEFAULT '{}';
