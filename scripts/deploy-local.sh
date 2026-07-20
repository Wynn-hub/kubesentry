#!/usr/bin/env bash
# One-command local deploy: installs the gateway chart first, self-registers
# this cluster ("local") with it, then installs the member chart in one
# shot with the harvested gateway.address/caBundle/clusterName/registrationToken.
#
# gateway.address is chart-required now (standalone deployment removed) —
# there is no longer an "install member bare, fill in gateway fields later"
# path, so this mirrors test/e2e's TestMain bootstrap order exactly: gateway
# chart first, harvest registration, then a single member chart install.
set -euo pipefail
cd "$(dirname "$0")/.."

NAMESPACE="${LOCAL_NAMESPACE:-kubesentry-system}"
CLUSTER_NAME="local"
REGISTRY="${REGISTRY:-wynnhub}"
TAG="${LOCAL_TAG:-local}"

echo "→ Deploying to kube context: $(kubectl config current-context)"

echo "→ installing gateway chart"
# No --wait: the gateway/console Deployments each carry an initContainer
# that blocks until the db-migrate post-install hook Job completes, but
# Helm's --wait blocks on ordinary-resource readiness *before* firing
# post-install hooks — combined, that deadlocks every install. Hook
# execution itself is still waited on (bounded by --timeout) regardless of
# --wait, so this still blocks until migration completes; the explicit
# `kubectl wait` calls below cover Deployment readiness instead.
helm upgrade --install kubesentry-gateway charts/kubesentry-gateway \
  --namespace "$NAMESPACE" --create-namespace \
  --set gateway.image.repository="$REGISTRY/kubesentry-gateway" \
  --set gateway.image.tag="$TAG" \
  --set gateway.image.pullPolicy=Never \
  --set console.image.repository="$REGISTRY/kubesentry-console" \
  --set console.image.tag="$TAG" \
  --set console.image.pullPolicy=Never \
  --set console.webImage.repository="$REGISTRY/kubesentry-console-web" \
  --set console.webImage.tag="$TAG" \
  --set console.webImage.pullPolicy=Never \
  --set db.image.repository="$REGISTRY/kubesentry-db" \
  --set db.image.tag="$TAG" \
  --timeout 5m

echo "→ waiting for gateway + console deployments"
kubectl -n "$NAMESPACE" rollout status deployment/kubesentry-gateway --timeout=3m
kubectl -n "$NAMESPACE" rollout status deployment/kubesentry-console --timeout=3m

# gateway has zero CRDs (all data lives in Postgres via GORM) and console-api
# has no REST route for cluster CRUD yet — registration is a one-time DB
# seed. cmd/db's register-cluster subcommand does that seed and prints the
# plaintext one-time token to stdout; run it as a throwaway in-cluster Job
# (same db image the migrate hook already uses) so it reaches Postgres over
# the cluster-internal Service without needing a port-forward from this
# script. Mirrors test/e2e's TestMain, which does the same seed in-process
# via GORM instead (it already links the Go packages).
echo "→ registering this cluster (\"$CLUSTER_NAME\") with the gateway"
kubectl delete job kubesentry-register-cluster -n "$NAMESPACE" --ignore-not-found >/dev/null 2>&1
kubectl apply -n "$NAMESPACE" -f - >/dev/null <<EOF
apiVersion: batch/v1
kind: Job
metadata:
  name: kubesentry-register-cluster
spec:
  backoffLimit: 0
  template:
    spec:
      restartPolicy: Never
      containers:
        - name: register
          image: "$REGISTRY/kubesentry-db:$TAG"
          imagePullPolicy: Never
          args: ["register-cluster", "--name", "$CLUSTER_NAME"]
          env:
            - name: DATABASE_DSN
              valueFrom:
                secretKeyRef:
                  name: kubesentry-postgres-credentials
                  key: dsn
EOF
if ! kubectl wait --for=condition=complete job/kubesentry-register-cluster -n "$NAMESPACE" --timeout=60s; then
  echo "cluster registration job did not complete:" >&2
  kubectl logs job/kubesentry-register-cluster -n "$NAMESPACE" >&2 || true
  exit 1
fi
TOKEN=$(kubectl logs job/kubesentry-register-cluster -n "$NAMESPACE" | tail -n1)
kubectl delete job kubesentry-register-cluster -n "$NAMESPACE" --ignore-not-found >/dev/null 2>&1
if [ -z "$TOKEN" ]; then
  echo "registration job produced no token" >&2
  exit 1
fi
CA_B64=$(kubectl get secret kubesentry-gateway-tls -n "$NAMESPACE" -o jsonpath='{.data.ca\.crt}')

# NodePort + node IP rather than the in-cluster svc DNS — the latter only
# resolves inside this cluster, so it can't be reused as guidance for a
# genuinely separate member cluster. This mirrors the gateway's grpc.nodePort
# default (see charts/kubesentry-gateway/values.yaml).
NODE_IP=$(kubectl get nodes -o jsonpath='{.items[0].status.addresses[0].address}')
GRPC_NODE_PORT=$(kubectl get svc kubesentry-gateway -n "$NAMESPACE" -o jsonpath='{.spec.ports[0].nodePort}')

echo "→ installing member chart (webhook + operator), pointed at the gateway"
helm upgrade --install kubesentry charts/kubesentry \
  --namespace "$NAMESPACE" --create-namespace \
  --set webhook.image.repository="$REGISTRY/kubesentry-webhook" \
  --set webhook.image.tag="$TAG" \
  --set webhook.image.pullPolicy=Never \
  --set operator.image.repository="$REGISTRY/kubesentry-operator" \
  --set operator.image.tag="$TAG" \
  --set operator.image.pullPolicy=Never \
  --set gateway.address="$NODE_IP:$GRPC_NODE_PORT" \
  --set gateway.tlsServerName="kubesentry-gateway.$NAMESPACE.svc" \
  --set gateway.caBundle="$CA_B64" \
  --set gateway.clusterName="$CLUSTER_NAME" \
  --set gateway.registrationToken="$TOKEN" \
  --wait --timeout 5m

echo ""
echo "Deployed. Inspect with:"
echo "  kubectl -n $NAMESPACE get pods"
