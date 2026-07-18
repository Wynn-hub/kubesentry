SET search_path TO kubesentry;
DELETE FROM policy_exceptions WHERE name = 'default-system-namespaces';
