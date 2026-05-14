#!/bin/bash

# 为每条规则创建精确的测试YAML，仅触发该规则
# 避免一次触发多个规则

TEST_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# 清空目录
rm -f $TEST_DIR/security-specific/*.yaml 2>/dev/null
rm -f $TEST_DIR/efficiency-specific/*.yaml 2>/dev/null
rm -f $TEST_DIR/reliability-specific/*.yaml 2>/dev/null

mkdir -p $TEST_DIR/security-specific
mkdir -p $TEST_DIR/efficiency-specific
mkdir -p $TEST_DIR/reliability-specific

echo "为每条规则创建精确的测试YAML..."
echo ""

# ==================== 安全规则 ====================
echo "== 创建安全规则测试YAML =="

# 1. runAsPrivileged (仅测试此规则)
cat > $TEST_DIR/security-specific/run-as-privileged.yaml << 'EOF'
apiVersion: v1
kind: Pod
metadata:
  name: privileged-test
  namespace: test-builtin-rules
spec:
  containers:
  - name: app
    image: nginx:1.19
    securityContext:
      privileged: true
      runAsNonRoot: true
      readOnlyRootFilesystem: true
      allowPrivilegeEscalation: false
      capabilities:
        drop: ["ALL"]
EOF
echo "✓ run-as-privileged.yaml"

# 2. privilegeEscalationAllowed (仅测试此规则)
cat > $TEST_DIR/security-specific/privilege-escalation.yaml << 'EOF'
apiVersion: v1
kind: Pod
metadata:
  name: escalation-test
  namespace: test-builtin-rules
spec:
  containers:
  - name: app
    image: nginx:1.19
    securityContext:
      privileged: false
      allowPrivilegeEscalation: true
      runAsNonRoot: true
      readOnlyRootFilesystem: true
      capabilities:
        drop: ["ALL"]
EOF
echo "✓ privilege-escalation.yaml"

# 3. runAsRootAllowed (仅测试此规则)
cat > $TEST_DIR/security-specific/run-as-root.yaml << 'EOF'
apiVersion: v1
kind: Pod
metadata:
  name: root-test
  namespace: test-builtin-rules
spec:
  containers:
  - name: app
    image: nginx:1.19
    securityContext:
      privileged: false
      allowPrivilegeEscalation: false
      readOnlyRootFilesystem: true
      capabilities:
        drop: ["ALL"]
EOF
echo "✓ run-as-root.yaml"

# 4. hostPIDSet (仅测试此规则)
cat > $TEST_DIR/security-specific/host-pid.yaml << 'EOF'
apiVersion: v1
kind: Pod
metadata:
  name: host-pid-test
  namespace: test-builtin-rules
spec:
  hostPID: true
  containers:
  - name: app
    image: nginx:1.19
    securityContext:
      privileged: false
      allowPrivilegeEscalation: false
      runAsNonRoot: true
      readOnlyRootFilesystem: true
      capabilities:
        drop: ["ALL"]
EOF
echo "✓ host-pid.yaml"

# 5. hostIPCSet
cat > $TEST_DIR/security-specific/host-ipc.yaml << 'EOF'
apiVersion: v1
kind: Pod
metadata:
  name: host-ipc-test
  namespace: test-builtin-rules
spec:
  hostIPC: true
  containers:
  - name: app
    image: nginx:1.19
    securityContext:
      privileged: false
      allowPrivilegeEscalation: false
      runAsNonRoot: true
      readOnlyRootFilesystem: true
      capabilities:
        drop: ["ALL"]
EOF
echo "✓ host-ipc.yaml"

# 6. dangerousCapabilities
cat > $TEST_DIR/security-specific/dangerous-capabilities.yaml << 'EOF'
apiVersion: v1
kind: Pod
metadata:
  name: dangerous-caps-test
  namespace: test-builtin-rules
spec:
  containers:
  - name: app
    image: nginx:1.19
    securityContext:
      privileged: false
      allowPrivilegeEscalation: false
      runAsNonRoot: true
      readOnlyRootFilesystem: true
      capabilities:
        add: ["SYS_ADMIN"]
        drop: ["NET_RAW"]
EOF
echo "✓ dangerous-capabilities.yaml"

# 7. sensitiveContainerEnvVar
cat > $TEST_DIR/security-specific/sensitive-env-var.yaml << 'EOF'
apiVersion: v1
kind: Pod
metadata:
  name: sensitive-env-test
  namespace: test-builtin-rules
spec:
  containers:
  - name: app
    image: nginx:1.19
    env:
    - name: DATABASE_PASSWORD
      value: "secret123"
    - name: API_KEY
      value: "sk_test_12345"
    securityContext:
      privileged: false
      allowPrivilegeEscalation: false
      runAsNonRoot: true
      readOnlyRootFilesystem: true
      capabilities:
        drop: ["ALL"]
EOF
echo "✓ sensitive-env-var.yaml"

# 8. sensitiveConfigmapContent
cat > $TEST_DIR/security-specific/sensitive-configmap.yaml << 'EOF'
apiVersion: v1
kind: ConfigMap
metadata:
  name: sensitive-config
  namespace: test-builtin-rules
data:
  DATABASE_PASSWORD: "secret123"
  AWS_SECRET_ACCESS_KEY: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
EOF
echo "✓ sensitive-configmap.yaml"

# 9. tlsSettingsMissing
cat > $TEST_DIR/security-specific/tls-settings-missing.yaml << 'EOF'
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: no-tls-ingress
  namespace: test-builtin-rules
spec:
  rules:
  - host: example.com
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: web
            port:
              number: 80
EOF
echo "✓ tls-settings-missing.yaml"

# ==================== 效率规则 ====================
echo ""
echo "== 创建效率规则测试YAML =="

# 10. cpuRequestsMissing
cat > $TEST_DIR/efficiency-specific/cpu-requests-missing.yaml << 'EOF'
apiVersion: v1
kind: Pod
metadata:
  name: cpu-request-test
  namespace: test-builtin-rules
spec:
  containers:
  - name: app
    image: nginx:1.19
    resources:
      limits:
        cpu: "1"
        memory: "128Mi"
EOF
echo "✓ cpu-requests-missing.yaml"

# 11. memoryRequestsMissing
cat > $TEST_DIR/efficiency-specific/memory-requests-missing.yaml << 'EOF'
apiVersion: v1
kind: Pod
metadata:
  name: memory-request-test
  namespace: test-builtin-rules
spec:
  containers:
  - name: app
    image: nginx:1.19
    resources:
      requests:
        cpu: "100m"
      limits:
        cpu: "500m"
        memory: "256Mi"
EOF
echo "✓ memory-requests-missing.yaml"

# 12. cpuLimitsMissing
cat > $TEST_DIR/efficiency-specific/cpu-limits-missing.yaml << 'EOF'
apiVersion: v1
kind: Pod
metadata:
  name: cpu-limits-test
  namespace: test-builtin-rules
spec:
  containers:
  - name: app
    image: nginx:1.19
    resources:
      requests:
        cpu: "100m"
        memory: "128Mi"
      limits:
        memory: "256Mi"
EOF
echo "✓ cpu-limits-missing.yaml"

# 13. memoryLimitsMissing
cat > $TEST_DIR/efficiency-specific/memory-limits-missing.yaml << 'EOF'
apiVersion: v1
kind: Pod
metadata:
  name: memory-limits-test
  namespace: test-builtin-rules
spec:
  containers:
  - name: app
    image: nginx:1.19
    resources:
      requests:
        cpu: "100m"
        memory: "128Mi"
      limits:
        cpu: "500m"
EOF
echo "✓ memory-limits-missing.yaml"

# ==================== 可靠性规则 ====================
echo ""
echo "== 创建可靠性规则测试YAML =="

# 14. readinessProbeMissing
cat > $TEST_DIR/reliability-specific/readiness-probe-missing.yaml << 'EOF'
apiVersion: apps/v1
kind: Deployment
metadata:
  name: no-readiness-probe
  namespace: test-builtin-rules
spec:
  replicas: 2
  selector:
    matchLabels:
      app: web
  template:
    metadata:
      labels:
        app: web
    spec:
      containers:
      - name: app
        image: nginx:1.19
        livenessProbe:
          httpGet:
            path: /healthz
            port: 8080
          initialDelaySeconds: 10
EOF
echo "✓ readiness-probe-missing.yaml"

# 15. livenessProbeMissing
cat > $TEST_DIR/reliability-specific/liveness-probe-missing.yaml << 'EOF'
apiVersion: v1
kind: Pod
metadata:
  name: no-liveness-probe
  namespace: test-builtin-rules
spec:
  containers:
  - name: app
    image: nginx:1.19
EOF
echo "✓ liveness-probe-missing.yaml"

# 16. tagNotSpecified - latest
cat > $TEST_DIR/reliability-specific/tag-latest.yaml << 'EOF'
apiVersion: v1
kind: Pod
metadata:
  name: latest-tag-test
  namespace: test-builtin-rules
spec:
  containers:
  - name: app
    image: nginx:latest
EOF
echo "✓ tag-latest.yaml"

# 17. tagNotSpecified - no tag
cat > $TEST_DIR/reliability-specific/tag-notag.yaml << 'EOF'
apiVersion: v1
kind: Pod
metadata:
  name: no-tag-test
  namespace: test-builtin-rules
spec:
  containers:
  - name: app
    image: nginx
EOF
echo "✓ tag-notag.yaml"

# 18. pullPolicyNotAlways
cat > $TEST_DIR/reliability-specific/pull-policy-not-always.yaml << 'EOF'
apiVersion: v1
kind: Pod
metadata:
  name: pull-policy-test
  namespace: test-builtin-rules
spec:
  containers:
  - name: app
    image: nginx:1.19
    imagePullPolicy: IfNotPresent
EOF
echo "✓ pull-policy-not-always.yaml"

# 19. priorityClassNotSet
cat > $TEST_DIR/reliability-specific/priority-class-not-set.yaml << 'EOF'
apiVersion: apps/v1
kind: Deployment
metadata:
  name: no-priority-class
  namespace: test-builtin-rules
spec:
  replicas: 2
  selector:
    matchLabels:
      app: web
  template:
    metadata:
      labels:
        app: web
    spec:
      containers:
      - name: app
        image: nginx:1.19
EOF
echo "✓ priority-class-not-set.yaml"

# 20. deploymentMissingReplicas
cat > $TEST_DIR/reliability-specific/deployment-single-replica.yaml << 'EOF'
apiVersion: apps/v1
kind: Deployment
metadata:
  name: single-replica-test
  namespace: test-builtin-rules
spec:
  replicas: 1
  selector:
    matchLabels:
      app: web
  template:
    metadata:
      labels:
        app: web
    spec:
      containers:
      - name: app
        image: nginx:1.19
EOF
echo "✓ deployment-single-replica.yaml"

# 21. topologySpreadConstraint
cat > $TEST_DIR/reliability-specific/topology-spread-missing.yaml << 'EOF'
apiVersion: apps/v1
kind: Deployment
metadata:
  name: no-topology-spread
  namespace: test-builtin-rules
spec:
  replicas: 2
  selector:
    matchLabels:
      app: web
  template:
    metadata:
      labels:
        app: web
    spec:
      containers:
      - name: app
        image: nginx:1.19
EOF
echo "✓ topology-spread-missing.yaml"

echo ""
echo "✓ 完成！为所有关键规则创建了精确的测试YAML"
