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
helm upgrade --install kubesentry-gateway charts/kubesentry-gateway \
  --namespace "$NAMESPACE" --create-namespace \
  --set gateway.image.repository="$REGISTRY/kubesentry-gateway" \
  --set gateway.image.tag="$TAG" \
  --set gateway.image.pullPolicy=Never \
  --set console.image.repository="$REGISTRY/kubesentry-console" \
  --set console.image.tag="$TAG" \
  --set console.image.pullPolicy=Never \
  --wait --timeout 5m

echo "→ registering this cluster (\"$CLUSTER_NAME\") with the gateway"
kubectl get cluster "$CLUSTER_NAME" >/dev/null 2>&1 || kubectl apply -f - <<EOF
apiVersion: kubesentry.io/v1alpha1
kind: Cluster
metadata:
  name: $CLUSTER_NAME
EOF

echo "→ waiting for the one-time registration token"
TOKEN_B64=""
for _ in $(seq 1 30); do
  TOKEN_B64=$(kubectl get secret "kubesentry-reg-$CLUSTER_NAME" -n "$NAMESPACE" -o jsonpath='{.data.token}' 2>/dev/null || true)
  [ -n "$TOKEN_B64" ] && break
  sleep 2
done
if [ -z "$TOKEN_B64" ]; then
  echo "registration token never appeared (kubesentry-reg-$CLUSTER_NAME)" >&2
  exit 1
fi
TOKEN=$(echo "$TOKEN_B64" | base64 -d)
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
