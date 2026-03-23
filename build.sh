#!/bin/sh

# The script requires:
# - rockcraft
# - skopeo with sudo privilege
# - yq
# - docker or podman with alias docker=podman

set -e

rockcraft clean
rockcraft pack -v

VERSION=$(yq -r '.version' rockcraft.yaml)

if command -v podman >/dev/null 2>&1; then
    CONTAINER_ENGINE="podman"
    DESTINATION="containers-storage:$IMAGE"
else
    CONTAINER_ENGINE="docker"
    DESTINATION="docker-daemon:$IMAGE"
fi

echo "Using $CONTAINER_ENGINE to load the image..."

sudo skopeo --insecure-policy copy oci-archive:authorization-service_${VERSION}_amd64.rock "$DESTINATION"

echo "$IMAGE built and loaded into $CONTAINER_ENGINE"

if [ "${PUSH_IMAGE}" = "true" ]; then
  skopeo --insecure-policy copy oci-archive:authorization-service_${VERSION}_amd64.rock docker://$IMAGE
  echo "$IMAGE pushed"
fi


