SET search_path TO kubesentry;

-- Permanent system-namespace exemption, formerly rendered by the
-- kubesentry-gateway chart's templates/default-exception.yaml PolicyException
-- CR (spec D9/D17). now() + interval is used in place of the old Go
-- "876000h" duration literal to express the same ~100-year effectively-forever
-- window.
INSERT INTO policy_exceptions (id, name, spec, expires_at) VALUES (2078506982033063936, 'default-system-namespaces', '{"allPolicies":true,"match":{"namespaces":["kube-system","kube-public","kube-node-lease","kubesentry-system"]},"reason":"System namespaces are excluded from policy enforcement by default"}'::jsonb, now() + interval '100 years');
