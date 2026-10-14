#!/bin/sh

# The script requires:
# - rockcraft
# - skopeo with sudo privilege
# - yq
# - docker or podman with alias docker=podman

set -e

IMAGE="${IMAGE:-authorization-service:dev}"
ARCH="${ARCH:-$(dpkg --print-architecture 2>/dev/null || true)}"
: "${ARCH:=$(go env GOARCH 2>/dev/null || true)}"
case "$ARCH" in
  amd64|arm64) ;;
  *)
    echo "ERROR: unsupported or undetermined rock arch: '$ARCH' (supported: amd64, arm64)" >&2
    exit 1
    ;;
esac
VERSION=$(yq -r '.version' rockcraft.yaml)
ROCK_FILE="authorization-service_${VERSION}_${ARCH}.rock"

rockcraft clean
rockcraft pack -v --platform "${ARCH}"

if command -v podman >/dev/null 2>&1; then
    CONTAINER_ENGINE="podman"
    DESTINATION="containers-storage:$IMAGE"
else
    CONTAINER_ENGINE="docker"
    DESTINATION="docker-daemon:$IMAGE"
fi

echo "Using $CONTAINER_ENGINE to load the image (${ARCH})..."

sudo skopeo --insecure-policy copy "oci-archive:${ROCK_FILE}" "$DESTINATION"

echo "$IMAGE built and loaded into $CONTAINER_ENGINE"

if [ "${PUSH_IMAGE}" = "true" ]; then
  skopeo --insecure-policy copy "oci-archive:${ROCK_FILE}" "docker://$IMAGE"
  echo "$IMAGE pushed"
fi


