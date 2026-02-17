#!/bin/bash

# Kubernetes Kind cluster setup script for testing Authorization Service with Istio

set -e

CLUSTER_NAME=${1:-authz-dev}
KUBE_VERSION=${2:-v1.31.0}

echo "🚀 Creating Kind cluster: $CLUSTER_NAME"

kind create cluster \
  --name "$CLUSTER_NAME" \
  --image "kindest/node:$KUBE_VERSION" \
  --config - <<EOF
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
name: $CLUSTER_NAME
nodes:
- role: control-plane
  extraPortMappings:
  - containerPort: 80
    hostPort: 80
    listenAddress: "0.0.0.0"
  - containerPort: 443
    hostPort: 443
    listenAddress: "0.0.0.0"
  - containerPort: 9090
    hostPort: 9090
    listenAddress: "0.0.0.0"
  - containerPort: 8888
    hostPort: 8888
    listenAddress: "0.0.0.0"
- role: worker
- role: worker
EOF

echo "✓ Cluster created successfully"

echo ""
echo "📦 Installing Istio..."

# Download and install Istio
curl -L https://istio.io/downloadIstio | sh -
cd istio-* || exit 1

export PATH=$PWD/bin:$PATH

# Install Istio with demo profile
./bin/istioctl install --set profile=demo -y --skip-confirmation

# Label namespace for Istio injection
kubectl label namespace default istio-injection=enabled --overwrite

echo "✓ Istio installed successfully"

echo ""
echo "✅ Kubernetes cluster is ready!"
echo ""
echo "Cluster name: $CLUSTER_NAME"
echo "Kube version: $KUBE_VERSION"
echo ""
echo "Next steps:"
echo "  1. Build and load Docker image: docker build -t authorization-service:latest . && kind load docker-image authorization-service:latest --name $CLUSTER_NAME"
echo "  2. Deploy: kubectl apply -f k8s/deployment.yaml"
echo "  3. Configure: kubectl apply -f k8s/istio-authz-policy.yaml"
echo "  4. Check status: kubectl get pods"
echo ""
echo "To delete the cluster:"
echo "  kind delete cluster --name $CLUSTER_NAME"
