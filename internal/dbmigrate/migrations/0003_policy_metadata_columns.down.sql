SET search_path TO kubesentry;

ALTER TABLE kubesentry.policy_exceptions DROP COLUMN annotations;
ALTER TABLE kubesentry.policy_exceptions DROP COLUMN labels;

ALTER TABLE kubesentry.policy_groups DROP COLUMN annotations;
ALTER TABLE kubesentry.policy_groups DROP COLUMN labels;

ALTER TABLE kubesentry.policies DROP COLUMN annotations;
ALTER TABLE kubesentry.policies DROP COLUMN labels;
