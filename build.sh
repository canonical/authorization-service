#!/bin/sh

# The script requires:
# - rockcraft
# - skopeo with sudo privilege
# - yq
# - docker

set -e

rockcraft clean
rockcraft pack -v


echo "$IMAGE built"

if [ "${PUSH_IMAGE}" = "true" ]; then
  skopeo --insecure-policy copy  oci-archive:authorization-service_$(yq -r '.version' rockcraft.yaml)_amd64.rock docker://$IMAGE
  echo "$IMAGE pushed"
fi


