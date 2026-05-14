#!/bin/bash

# 脚本用途：为每条内置规则生成测试YAML文件
# 用法: bash generate-test-yamls.sh

set -e

TEST_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NAMESPACE="test-builtin-rules"

# 创建命名空间（稍后手动创建）
echo "Creating test namespace: $NAMESPACE"

# ==================== 安全规则 ====================

cat > "$TEST_DIR/security/privilege-escalation-fail.yaml" << 'EOF'
---
# privilegeEscalationAllowed: Container with allowPrivilegeEscalation=true
apiVersion: v1
kind: Pod
metadata:
  name: privilege-escalation-pod
  namespace: test-builtin-rules
spec:
  containers:
  - name: app
    image: nginx:1.19
    securityContext:
      allowPrivilegeEscalation: true
EOF

cat > "$TEST_DIR/security/run-as-root-fail.yaml" << 'EOF'
---
# runAsRootAllowed: Container without runAsNonRoot
apiVersion: v1
kind: Pod
metadata:
  name: run-as-root-pod
  namespace: test-builtin-rules
spec:
  containers:
  - name: app
    image: nginx:1.19
    # 不设置 securityContext.runAsNonRoot，默认以root运行
EOF

cat > "$TEST_DIR/security/not-readonly-filesystem-fail.yaml" << 'EOF'
---
# notReadOnlyRootFilesystem: Container without readOnlyRootFilesystem
apiVersion: v1
kind: Pod
metadata:
  name: not-readonly-fs-pod
  namespace: test-builtin-rules
spec:
  containers:
  - name: app
    image: nginx:1.19
    securityContext:
      readOnlyRootFilesystem: false
EOF

cat > "$TEST_DIR/security/linux-hardening-fail.yaml" << 'EOF'
---
# linuxHardening: Container without Seccomp, SELinux, or capabilities.drop
apiVersion: v1
kind: Pod
metadata:
  name: no-hardening-pod
  namespace: test-builtin-rules
spec:
  containers:
  - name: app
    image: nginx:1.19
    # 没有Seccomp、SELinux或capabilities配置
EOF

cat > "$TEST_DIR/security/insecure-capabilities-fail.yaml" << 'EOF'
---
# insecureCapabilities: Container with insecure capabilities
apiVersion: v1
kind: Pod
metadata:
  name: insecure-caps-pod
  namespace: test-builtin-rules
spec:
  containers:
  - name: app
    image: nginx:1.19
    securityContext:
      capabilities:
        add: ["NET_RAW", "SYS_PTRACE"]
EOF

cat > "$TEST_DIR/security/dangerous-capabilities-fail.yaml" << 'EOF'
---
# dangerousCapabilities: Container with dangerous capabilities
apiVersion: v1
kind: Pod
metadata:
  name: dangerous-caps-pod
  namespace: test-builtin-rules
spec:
  containers:
  - name: app
    image: nginx:1.19
    securityContext:
      capabilities:
        add: ["SYS_ADMIN", "NET_ADMIN"]
EOF

cat > "$TEST_DIR/security/host-pid-fail.yaml" << 'EOF'
---
# hostPIDSet: Pod with hostPID=true
apiVersion: v1
kind: Pod
metadata:
  name: host-pid-pod
  namespace: test-builtin-rules
spec:
  hostPID: true
  containers:
  - name: app
    image: nginx:1.19
EOF

cat > "$TEST_DIR/security/host-ipc-fail.yaml" << 'EOF'
---
# hostIPCSet: Pod with hostIPC=true
apiVersion: v1
kind: Pod
metadata:
  name: host-ipc-pod
  namespace: test-builtin-rules
spec:
  hostIPC: true
  containers:
  - name: app
    image: nginx:1.19
EOF

cat > "$TEST_DIR/security/host-network-fail.yaml" << 'EOF'
---
# hostNetworkSet: Pod with hostNetwork=true
apiVersion: v1
kind: Pod
metadata:
  name: host-network-pod
  namespace: test-builtin-rules
spec:
  hostNetwork: true
  containers:
  - name: app
    image: nginx:1.19
EOF

cat > "$TEST_DIR/security/host-port-fail.yaml" << 'EOF'
---
# hostPortSet: Container with hostPort
apiVersion: v1
kind: Pod
metadata:
  name: host-port-pod
  namespace: test-builtin-rules
spec:
  containers:
  - name: app
    image: nginx:1.19
    ports:
    - containerPort: 8080
      hostPort: 8080
EOF

cat > "$TEST_DIR/security/sensitive-env-var-fail.yaml" << 'EOF'
---
# sensitiveContainerEnvVar: Container with sensitive env var
apiVersion: v1
kind: Pod
metadata:
  name: sensitive-env-pod
  namespace: test-builtin-rules
spec:
  containers:
  - name: app
    image: nginx:1.19
    env:
    - name: DATABASE_PASSWORD
      value: "secret123"
    - name: AWS_SECRET_ACCESS_KEY
      value: "secret456"
EOF

cat > "$TEST_DIR/security/automount-token-fail.yaml" << 'EOF'
---
# automountServiceAccountToken: Pod without automountServiceAccountToken=false
apiVersion: v1
kind: Pod
metadata:
  name: automount-token-pod
  namespace: test-builtin-rules
spec:
  containers:
  - name: app
    image: nginx:1.19
  # automountServiceAccountToken 默认为 true
EOF

cat > "$TEST_DIR/security/sensitive-configmap-fail.yaml" << 'EOF'
---
# sensitiveConfigmapContent: ConfigMap with sensitive content
apiVersion: v1
kind: ConfigMap
metadata:
  name: sensitive-config
  namespace: test-builtin-rules
data:
  DATABASE_PASSWORD: "secret123"
  AWS_SECRET_ACCESS_KEY: "myawssecret"
  PRIVATE_KEY: "-----BEGIN PRIVATE KEY-----"
EOF

cat > "$TEST_DIR/security/tls-settings-missing-fail.yaml" << 'EOF'
---
# tlsSettingsMissing: Ingress without TLS
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

cat > "$TEST_DIR/security/clusterrole-exec-fail.yaml" << 'EOF'
---
# clusterrolePodExecAttach: ClusterRole with pods/exec
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: dangerous-clusterrole
rules:
- apiGroups: [""]
  resources: ["pods"]
  verbs: ["get", "list", "watch", "exec"]
EOF

cat > "$TEST_DIR/security/role-exec-fail.yaml" << 'EOF'
---
# rolePodExecAttach: Role with pods/attach
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: dangerous-role
  namespace: test-builtin-rules
rules:
- apiGroups: [""]
  resources: ["pods"]
  verbs: ["get", "list", "watch", "attach"]
EOF

cat > "$TEST_DIR/security/clusterrolebinding-exec-fail.yaml" << 'EOF'
---
# clusterrolebindingPodExecAttach: ClusterRoleBinding to ClusterRole with exec
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: dangerous-binding
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: dangerous-clusterrole
subjects:
- kind: ServiceAccount
  name: default
  namespace: test-builtin-rules
EOF

cat > "$TEST_DIR/security/rolebinding-role-exec-fail.yaml" << 'EOF'
---
# rolebindingRolePodExecAttach: RoleBinding to Role with exec
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: dangerous-role-binding
  namespace: test-builtin-rules
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: dangerous-role
subjects:
- kind: ServiceAccount
  name: default
  namespace: test-builtin-rules
EOF

cat > "$TEST_DIR/security/rolebinding-clusterrole-exec-fail.yaml" << 'EOF'
---
# rolebindingClusterRolePodExecAttach: RoleBinding to ClusterRole with exec
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: clusterrole-binding
  namespace: test-builtin-rules
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: dangerous-clusterrole
subjects:
- kind: ServiceAccount
  name: default
  namespace: test-builtin-rules
EOF

cat > "$TEST_DIR/security/clusterrolebinding-admin-fail.yaml" << 'EOF'
---
# clusterrolebindingClusterAdmin: ClusterRoleBinding to cluster-admin
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: admin-binding
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: cluster-admin
subjects:
- kind: ServiceAccount
  name: default
  namespace: test-builtin-rules
EOF

cat > "$TEST_DIR/security/rolebinding-clusteradmin-clusterrole-fail.yaml" << 'EOF'
---
# rolebindingClusterAdminClusterRole: RoleBinding to cluster-admin ClusterRole
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: admin-rolebinding
  namespace: test-builtin-rules
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: cluster-admin
subjects:
- kind: ServiceAccount
  name: default
  namespace: test-builtin-rules
EOF

cat > "$TEST_DIR/security/rolebinding-clusteradmin-role-fail.yaml" << 'EOF'
---
# rolebindingClusterAdminRole: RoleBinding to Role with wildcard permissions
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: wildcard-role
  namespace: test-builtin-rules
rules:
- apiGroups: ["*"]
  resources: ["*"]
  verbs: ["*"]
EOF

# ==================== 效率规则 ====================

cat > "$TEST_DIR/efficiency/cpu-requests-missing-fail.yaml" << 'EOF'
---
# cpuRequestsMissing: Pod without CPU requests
apiVersion: v1
kind: Pod
metadata:
  name: no-cpu-request-pod
  namespace: test-builtin-rules
spec:
  containers:
  - name: app
    image: nginx:1.19
    resources:
      limits:
        cpu: "1"
        memory: "128Mi"
      # missing requests
EOF

cat > "$TEST_DIR/efficiency/memory-requests-missing-fail.yaml" << 'EOF'
---
# memoryRequestsMissing: Pod without memory requests
apiVersion: v1
kind: Pod
metadata:
  name: no-memory-request-pod
  namespace: test-builtin-rules
spec:
  containers:
  - name: app
    image: nginx:1.19
    resources:
      requests:
        cpu: "100m"
      limits:
        memory: "256Mi"
      # missing memory requests
EOF

cat > "$TEST_DIR/efficiency/cpu-limits-missing-fail.yaml" << 'EOF'
---
# cpuLimitsMissing: Pod without CPU limits
apiVersion: v1
kind: Pod
metadata:
  name: no-cpu-limits-pod
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
      # missing cpu limits
EOF

cat > "$TEST_DIR/efficiency/memory-limits-missing-fail.yaml" << 'EOF'
---
# memoryLimitsMissing: Pod without memory limits
apiVersion: v1
kind: Pod
metadata:
  name: no-memory-limits-pod
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
      # missing memory limits
EOF

# ==================== 可靠性规则 ====================

cat > "$TEST_DIR/reliability/readiness-probe-missing-fail.yaml" << 'EOF'
---
# readinessProbeMissing: Deployment without readiness probe
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
        # missing readiness probe
EOF

cat > "$TEST_DIR/reliability/liveness-probe-missing-fail.yaml" << 'EOF'
---
# livenessProbeMissing: Pod without liveness probe
apiVersion: v1
kind: Pod
metadata:
  name: no-liveness-probe
  namespace: test-builtin-rules
spec:
  containers:
  - name: app
    image: nginx:1.19
    # missing liveness probe
EOF

cat > "$TEST_DIR/reliability/tag-not-specified-fail.yaml" << 'EOF'
---
# tagNotSpecified: Pod with image tag as 'latest'
apiVersion: v1
kind: Pod
metadata:
  name: latest-tag-pod
  namespace: test-builtin-rules
spec:
  containers:
  - name: app
    image: nginx:latest
EOF

cat > "$TEST_DIR/reliability/tag-not-specified-fail-2.yaml" << 'EOF'
---
# tagNotSpecified: Pod with image without tag
apiVersion: v1
kind: Pod
metadata:
  name: no-tag-pod
  namespace: test-builtin-rules
spec:
  containers:
  - name: app
    image: nginx
EOF

cat > "$TEST_DIR/reliability/pull-policy-not-always-fail.yaml" << 'EOF'
---
# pullPolicyNotAlways: Pod with imagePullPolicy != Always
apiVersion: v1
kind: Pod
metadata:
  name: pull-policy-pod
  namespace: test-builtin-rules
spec:
  containers:
  - name: app
    image: nginx:1.19
    imagePullPolicy: IfNotPresent
EOF

cat > "$TEST_DIR/reliability/priority-class-not-set-fail.yaml" << 'EOF'
---
# priorityClassNotSet: Deployment without priorityClassName
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
      # missing priorityClassName
EOF

cat > "$TEST_DIR/reliability/deployment-single-replica-fail.yaml" << 'EOF'
---
# deploymentMissingReplicas: Deployment with only one replica
apiVersion: apps/v1
kind: Deployment
metadata:
  name: single-replica
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

cat > "$TEST_DIR/reliability/metadata-instance-mismatch-fail.yaml" << 'EOF'
---
# metadataAndInstanceMismatched: Pod where app.kubernetes.io/instance != metadata.name
apiVersion: v1
kind: Pod
metadata:
  name: my-pod
  namespace: test-builtin-rules
  labels:
    app.kubernetes.io/instance: different-name
spec:
  containers:
  - name: app
    image: nginx:1.19
EOF

cat > "$TEST_DIR/reliability/topology-spread-missing-fail.yaml" << 'EOF'
---
# topologySpreadConstraint: Deployment without topology spread constraint
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
      # missing topologySpreadConstraints
EOF

cat > "$TEST_DIR/reliability/hpa-max-availability-fail.yaml" << 'EOF'
---
# hpaMaxAvailability: HPA with maxReplicas <= minReplicas
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: bad-hpa
  namespace: test-builtin-rules
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: web
  minReplicas: 3
  maxReplicas: 3
  metrics:
  - type: Resource
    resource:
      name: cpu
      target:
        type: Utilization
        averageUtilization: 80
EOF

cat > "$TEST_DIR/reliability/hpa-min-availability-fail.yaml" << 'EOF'
---
# hpaMinAvailability: HPA with minReplicas <= 1
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: low-min-hpa
  namespace: test-builtin-rules
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: web
  minReplicas: 1
  maxReplicas: 10
  metrics:
  - type: Resource
    resource:
      name: cpu
      target:
        type: Utilization
        averageUtilization: 80
EOF

echo "✓ Generated all test YAML files in:"
echo "  - $TEST_DIR/security/"
echo "  - $TEST_DIR/efficiency/"
echo "  - $TEST_DIR/reliability/"
echo ""
echo "Next steps:"
echo "1. Create namespace: kubectl create namespace $NAMESPACE"
echo "2. Register policies with kubesentry (use PolicyGroup)"
echo "3. Run tests: bash test-builtin-rules.sh"
