# Copyright 2026 Canonical Ltd.
# SPDX-License-Identifier: Apache-2.0

resource "kubernetes_service_account" "bootstrapper" {
  count = var.deploy_openfga ? 1 : 0

  metadata {
    name      = "cerberus-bootstrapper"
    namespace = kubernetes_namespace.cerberus.metadata[0].name
    labels = {
      app = "cerberus-bootstrap"
    }
  }
}

resource "kubernetes_role" "bootstrapper" {
  count = var.deploy_openfga ? 1 : 0

  metadata {
    name      = "cerberus-bootstrapper"
    namespace = kubernetes_namespace.cerberus.metadata[0].name
    labels = {
      app = "cerberus-bootstrap"
    }
  }
  rule {
    api_groups = [""]
    resources  = ["configmaps"]
    verbs      = ["get", "update", "patch"]
  }
  rule {
    api_groups = ["apps"]
    resources  = ["deployments"]
    verbs      = ["get", "update", "patch"]
  }
}

resource "kubernetes_role_binding" "bootstrapper" {
  count = var.deploy_openfga ? 1 : 0

  metadata {
    name      = "cerberus-bootstrapper"
    namespace = kubernetes_namespace.cerberus.metadata[0].name
    labels = {
      app = "cerberus-bootstrap"
    }
  }
  role_ref {
    api_group = "rbac.authorization.k8s.io"
    kind      = "Role"
    name      = kubernetes_role.bootstrapper[0].metadata[0].name
  }
  subject {
    kind      = "ServiceAccount"
    name      = kubernetes_service_account.bootstrapper[0].metadata[0].name
    namespace = kubernetes_namespace.cerberus.metadata[0].name
  }
}

resource "kubernetes_job" "cerberus_bootstrap" {
  count = var.deploy_openfga ? 1 : 0

  metadata {
    name      = "cerberus-bootstrap"
    namespace = kubernetes_namespace.cerberus.metadata[0].name
    labels = {
      app = "cerberus-bootstrap"
    }
  }
  spec {
    template {
      metadata {
        labels = {
          app = "cerberus-bootstrap"
        }
      }
      spec {
        service_account_name = kubernetes_service_account.bootstrapper[0].metadata[0].name
        restart_policy       = "Never"

        # 1. Wait for PostgreSQL and OpenFGA to be active
        init_container {
          name  = "wait-for-dependencies"
          image = "curlimages/curl:8.5.0"
          command = ["sh", "-c", <<-EOT
            until curl -s http://openfga:8080/healthz; do
              echo "Waiting for OpenFGA..."
              sleep 2
            done
            echo "OpenFGA is ready!"
          EOT
          ]
        }

        # 2. Seed the Store ID in PostgreSQL database (supports internal or external PG)
        init_container {
          name  = "seed-fga-store"
          image = "postgres:14-alpine"
          env {
            name  = "PGPASSWORD"
            value = local.db_password
          }
          command = ["sh", "-c", <<-EOT
            until pg_isready -h ${local.db_host} -p ${local.db_port} -U ${local.db_user}; do
              echo "Waiting for PostgreSQL..."
              sleep 2
            done
            psql -h ${local.db_host} -p ${local.db_port} -U ${local.db_user} -d openfga -c "INSERT INTO store (id,name,created_at,updated_at) VALUES ('01GP1254CHWJC1MNGVB0WDG1T0','cerberus',NOW(),NOW()) ON CONFLICT DO NOTHING;"
          EOT
          ]
        }

        # 3. Write Authorization Model to OpenFGA
        init_container {
          name              = "write-fga-model"
          image             = var.cerberus_image
          image_pull_policy = var.cerberus_image_pull_policy
          command           = ["/usr/bin/app", "authz", "write-model", "01GP1254CHWJC1MNGVB0WDG1T0"]
          env {
            name  = "OPENFGA_ADDRESS"
            value = "http://openfga:8081"
          }
          env {
            name  = "OPENFGA_STORE_ID"
            value = "01GP1254CHWJC1MNGVB0WDG1T0"
          }
          env {
            name  = "OPENFGA_API_KEY"
            value = "42"
          }
          env {
            name  = "LOG_LEVEL"
            value = "debug"
          }
        }

        # 4. Fetch the resulting Model ID & Patch the ConfigMap & Rollout Cerberus Deployment
        container {
          name  = "patch-config"
          image = "curlimages/curl:8.5.0"
          command = ["sh", "-c", <<-EOT
            echo "Retrieving Authorization Model ID from OpenFGA..."
            while true; do
              RESPONSE=$(curl -s http://openfga:8080/stores/01GP1254CHWJC1MNGVB0WDG1T0/authorization-models)
              MODEL_ID=$(echo "$RESPONSE" | grep -oE '"id":"[0-9A-Z]{26}"' | head -n1 | cut -d'"' -f4)
              if [ -n "$MODEL_ID" ]; then
                echo "Discovered Model ID: $MODEL_ID"
                break
              fi
              echo "Waiting for Model registration..."
              sleep 2
            done

            NAMESPACE=$(cat /var/run/secrets/kubernetes.io/serviceaccount/namespace)
            TOKEN=$(cat /var/run/secrets/kubernetes.io/serviceaccount/token)

            echo "Updating ConfigMap..."
            curl -s --cacert /var/run/secrets/kubernetes.io/serviceaccount/ca.crt \
                 -X PATCH \
                 -H "Authorization: Bearer $TOKEN" \
                 -H "Content-Type: application/merge-patch+json" \
                 -d "{\"data\":{\"OPENFGA_AUTHZ_MODEL_ID\":\"$MODEL_ID\"}}" \
                 "https://kubernetes.default.svc/api/v1/namespaces/$NAMESPACE/configmaps/authorization-service-config"

            echo "Triggering Rollout Restart on Cerberus deployment..."
            TIMESTAMP=$(date +%s)
            curl -s --cacert /var/run/secrets/kubernetes.io/serviceaccount/ca.crt \
                 -X PATCH \
                 -H "Authorization: Bearer $TOKEN" \
                 -H "Content-Type: application/merge-patch+json" \
                 -d "{\"spec\":{\"template\":{\"metadata\":{\"annotations\":{\"cerberus.bootstrap/restartedAt\":\"$TIMESTAMP\"}}}}}" \
                 "https://kubernetes.default.svc/apis/apps/v1/namespaces/$NAMESPACE/deployments/authorization-service"

            echo "Bootstrapping orchestrator done!"
          EOT
          ]
        }
      }
    }
  }
  depends_on = [
    kubernetes_deployment.openfga,
    kubernetes_service.openfga,
    kubernetes_service.postgres
  ]
}
