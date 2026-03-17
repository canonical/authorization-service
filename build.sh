#!/bin/sh

# The script requires:
# - rockcraft
# - skopeo with sudo privilege
# - yq
# - docker

set -e

rockcraft clean
rockcraft pack -v

VERSION=$(yq -r '.version' rockcraft.yaml)
sudo skopeo --insecure-policy copy oci-archive:authorization-service_${VERSION}_amd64.rock docker-daemon:$IMAGE

echo "$IMAGE built"

if [ "${PUSH_IMAGE}" = "true" ]; then
  skopeo --insecure-policy copy oci-archive:authorization-service_${VERSION}_amd64.rock docker://$IMAGE
  echo "$IMAGE pushed"
fi


