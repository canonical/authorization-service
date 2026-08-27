# Copyright 2026 Canonical Ltd.
# SPDX-License-Identifier: Apache-2.0

resource "kubernetes_service_account" "bootstrapper" {
  metadata {
    name      = "authorization-service-bootstrapper"
    namespace = kubernetes_namespace.authorization-service.metadata[0].name
    labels = {
      app = "authorization-service-bootstrap"
    }
  }
}

resource "kubernetes_role" "bootstrapper" {
  metadata {
    name      = "authorization-service-bootstrapper"
    namespace = kubernetes_namespace.authorization-service.metadata[0].name
    labels = {
      app = "authorization-service-bootstrap"
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
  metadata {
    name      = "authorization-service-bootstrapper"
    namespace = kubernetes_namespace.authorization-service.metadata[0].name
    labels = {
      app = "authorization-service-bootstrap"
    }
  }
  role_ref {
    api_group = "rbac.authorization.k8s.io"
    kind      = "Role"
    name      = kubernetes_role.bootstrapper.metadata[0].name
  }
  subject {
    kind      = "ServiceAccount"
    name      = kubernetes_service_account.bootstrapper.metadata[0].name
    namespace = kubernetes_namespace.authorization-service.metadata[0].name
  }
}

resource "kubernetes_job" "authorization-service_bootstrap" {
  metadata {
    name      = "authorization-service-bootstrap"
    namespace = kubernetes_namespace.authorization-service.metadata[0].name
    labels = {
      app = "authorization-service-bootstrap"
    }
  }
  spec {
    template {
      metadata {
        labels = {
          app = "authorization-service-bootstrap"
        }
      }
      spec {
        service_account_name = kubernetes_service_account.bootstrapper.metadata[0].name
        restart_policy       = "Never"

        # 1. Wait for PostgreSQL and OpenFGA to be active
        init_container {
          name  = "wait-for-dependencies"
          image = "curlimages/curl:8.5.0"
          command = ["sh", "-c", <<-EOT
            until curl -s http://${local.fga_address == "http://openfga:8081" ? "openfga:8080" : split("/", local.fga_address)[2]}/healthz; do
              echo "Waiting for OpenFGA..."
              sleep 2
            done
            echo "OpenFGA is ready!"
          EOT
          ]
        }

        # 2. Seed the Store ID in PostgreSQL database (only if deploying internal OpenFGA)
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
            if [ "${var.deploy_openfga}" = "true" ]; then
              psql -h ${local.db_host} -p ${local.db_port} -U ${local.db_user} -d openfga -c "INSERT INTO store (id,name,created_at,updated_at) VALUES ('01GP1254CHWJC1MNGVB0WDG1T0','authorization-service',NOW(),NOW()) ON CONFLICT DO NOTHING;"
            else
              echo "Skip OpenFGA store seeding (using external OpenFGA)."
            fi
          EOT
          ]
        }

        # 3. Apply Authorization-service DB schema migrations
        init_container {
          name              = "run-migrations"
          image             = var.authorization-service_image
          image_pull_policy = var.authorization-service_image_pull_policy
          command           = ["/usr/bin/app"]
          args              = ["migrate", "--dsn", "$(DATABASE_URL)", "up"]
          env_from {
            config_map_ref {
              name = kubernetes_config_map.authorization-service_config.metadata[0].name
            }
          }
          env_from {
            secret_ref {
              name = kubernetes_secret.authorization-service_secret.metadata[0].name
            }
          }
        }

        # 4. Write Authorization Model to OpenFGA (only if deploying internal OpenFGA)
        init_container {
          name              = "write-fga-model"
          image             = var.authorization-service_image
          image_pull_policy = var.authorization-service_image_pull_policy
          command           = ["/usr/bin/app", "authz", "write-model", "01GP1254CHWJC1MNGVB0WDG1T0"]
          env_from {
            config_map_ref {
              name = kubernetes_config_map.authorization-service_config.metadata[0].name
            }
          }
          env_from {
            secret_ref {
              name = kubernetes_secret.authorization-service_secret.metadata[0].name
            }
          }
          env {
            name  = "OPENFGA_ADDRESS"
            value = local.fga_address
          }
          env {
            name  = "OPENFGA_STORE_ID"
            value = local.fga_store_id
          }
          env {
            name  = "OPENFGA_API_KEY"
            value = local.fga_api_key
          }
          env {
            name  = "LOG_LEVEL"
            value = "debug"
          }
        }

        # 5. Ensure Kafka Topics (only if Kafka is enabled)
        init_container {
          name              = "ensure-topics"
          image             = var.authorization-service_image
          image_pull_policy = var.authorization-service_image_pull_policy
          command           = ["sh", "-c", <<-EOT
            if [ "${var.kafka_enabled}" = "true" ]; then
              echo "Ensuring Kafka topics..."
              /usr/bin/app ensure-topics
            else
              echo "Kafka is disabled, skipping topic creation."
            fi
          EOT
          ]
          env_from {
            config_map_ref {
              name = kubernetes_config_map.authorization-service_config.metadata[0].name
            }
          }
          env_from {
            secret_ref {
              name = kubernetes_secret.authorization-service_secret.metadata[0].name
            }
          }
        }

        # 6. Seed rules into PostgreSQL DB
        init_container {
          name              = "run-seed"
          image             = var.authorization-service_image
          image_pull_policy = var.authorization-service_image_pull_policy
          command           = ["/usr/bin/app", "seed"]
          env_from {
            config_map_ref {
              name = kubernetes_config_map.authorization-service_config.metadata[0].name
            }
          }
          env_from {
            secret_ref {
              name = kubernetes_secret.authorization-service_secret.metadata[0].name
            }
          }
        }

        # 7. Fetch resulting Model ID & Patch ConfigMap & Rollout Deployments
        container {
          name  = "patch-config"
          image = "curlimages/curl:8.5.0"
          command = ["sh", "-c", <<-EOT
            DEPLOY_OPENFGA="${var.deploy_openfga}"
            if [ "$DEPLOY_OPENFGA" = "true" ]; then
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
                   -d "{\"data\":{\"OPENFGA_AUTHORIZATION_MODEL_ID\":\"$MODEL_ID\"}}" \
                   "https://kubernetes.default.svc/api/v1/namespaces/$NAMESPACE/configmaps/authorization-service-config"
            else
              echo "Skip OpenFGA Model Discovery and ConfigMap patching (using external OpenFGA)."
              NAMESPACE=$(cat /var/run/secrets/kubernetes.io/serviceaccount/namespace)
              TOKEN=$(cat /var/run/secrets/kubernetes.io/serviceaccount/token)
            fi

            TIMESTAMP=$(date +%s)
            for DEP in "authorization-service" "authorization-service-listener" "authorization-service-worker"; do
              echo "Checking if deployment $DEP exists..."
              HTTP_CODE=$(curl -s -o /dev/null -w "%%{http_code}" --cacert /var/run/secrets/kubernetes.io/serviceaccount/ca.crt \
                   -H "Authorization: Bearer $TOKEN" \
                   "https://kubernetes.default.svc/api/v1/namespaces/$NAMESPACE/deployments/$DEP")
              
              if [ "$HTTP_CODE" = "200" ]; then
                echo "Triggering Rollout Restart on $DEP..."
                curl -s --cacert /var/run/secrets/kubernetes.io/serviceaccount/ca.crt \
                     -X PATCH \
                     -H "Authorization: Bearer $TOKEN" \
                     -H "Content-Type: application/merge-patch+json" \
                     -d "{\"spec\":{\"template\":{\"metadata\":{\"annotations\":{\"authorization-service.bootstrap/restartedAt\":\"$TIMESTAMP\"}}}}}" \
                     "https://kubernetes.default.svc/apis/apps/v1/namespaces/$NAMESPACE/deployments/$DEP"
              else
                echo "Deployment $DEP does not exist or is not deployed (HTTP $HTTP_CODE)."
              fi
            done

            echo "Bootstrapping orchestrator done!"
          EOT
          ]
        }
      }
    }
  }
}
