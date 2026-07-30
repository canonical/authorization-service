# Copyright 2026 Canonical Ltd.
# SPDX-License-Identifier: Apache-2.0

resource "kubernetes_config_map" "cerberus_config" {
  metadata {
    name      = "authorization-service-config"
    namespace = kubernetes_namespace.cerberus.metadata[0].name
    labels = {
      app = "authorization-service"
    }
  }
  data = {
    GRPC_PORT              = "9091"
    HTTP_PORT              = "8070"
    SERVER_HOST            = "0.0.0.0"
    LOG_LEVEL              = "info"
    LOG_FORMAT             = "json"
    POSTGRES_HOST          = local.db_host
    POSTGRES_PORT          = tostring(local.db_port)
    POSTGRES_DB            = local.db_name
    POSTGRES_USER          = local.db_user
    OPENFGA_ADDRESS        = local.fga_address
    OPENFGA_STORE_ID       = local.fga_store_id
    OPENFGA_API_KEY        = local.fga_api_key
    OPENFGA_AUTHZ_MODEL_ID = local.fga_model_id # Injected dynamically by bootstrap Job if internal, otherwise passed directly
    VALKEY_ENABLED         = "false"
    DEV                    = "false"
    
    # Secure Token Service (STS)
    STS_ADDRESS                = local.sts_address
    STS_USE_TLS                = local.sts_use_tls
    STS_EAGER_CONNECTION_CHECK = local.sts_eager_connection_check
  }
}

resource "kubernetes_secret" "cerberus_secret" {
  metadata {
    name      = "authorization-service-secret"
    namespace = kubernetes_namespace.cerberus.metadata[0].name
    labels = {
      app = "authorization-service"
    }
  }
  data = {
    POSTGRES_PASSWORD = local.db_password
    DATABASE_URL      = local.db_url
  }
}

resource "kubernetes_service_account" "cerberus" {
  metadata {
    name      = "authorization-service"
    namespace = kubernetes_namespace.cerberus.metadata[0].name
    labels = {
      app = "authorization-service"
    }
  }
}

resource "kubernetes_service" "cerberus" {
  metadata {
    name      = "authorization-service"
    namespace = kubernetes_namespace.cerberus.metadata[0].name
    labels = {
      app = "authorization-service"
    }
  }
  spec {
    port {
      name        = "http"
      port        = 8070
      target_port = 8070
    }
    port {
      name        = "grpc"
      port        = 9091
      target_port = 9091
    }
    selector = {
      app = "authorization-service"
    }
    type = "ClusterIP"
  }
}

resource "kubernetes_deployment" "cerberus_server" {
  metadata {
    name      = "authorization-service"
    namespace = kubernetes_namespace.cerberus.metadata[0].name
    labels = {
      app = "authorization-service"
    }
  }
  spec {
    replicas = var.cerberus_replicas
    selector {
      match_labels = {
        app = "authorization-service"
      }
    }
    template {
      metadata {
        labels = {
          app = "authorization-service"
        }
      }
      spec {
        service_account_name = kubernetes_service_account.cerberus.metadata[0].name

        # 1. Block until postgres (internal or external) is ready to receive requests
        init_container {
          name  = "wait-for-postgres"
          image = "postgres:14-alpine"
          command = ["sh", "-c", "until pg_isready -h ${local.db_host} -p ${local.db_port} -U ${local.db_user}; do echo 'Waiting for PostgreSQL...'; sleep 2; done; echo 'PostgreSQL is ready!'"]
        }

        # 2. Apply Cerberus DB schema migrations (works for both internal & external databases)
        init_container {
          name              = "run-migrations"
          image             = var.cerberus_image
          image_pull_policy = var.cerberus_image_pull_policy
          command           = ["/usr/bin/app", "migrate"]
          env_from {
            config_map_ref {
              name = kubernetes_config_map.cerberus_config.metadata[0].name
            }
          }
          env_from {
            secret_ref {
              name = kubernetes_secret.cerberus_secret.metadata[0].name
            }
          }
        }

        # 3. Main server process
        container {
          name              = "authorization-service"
          image             = var.cerberus_image
          image_pull_policy = var.cerberus_image_pull_policy
          command           = ["/usr/bin/app", "serve"]
          port {
            name           = "http"
            container_port = 8070
          }
          port {
            name           = "grpc"
            container_port = 9091
          }
          env_from {
            config_map_ref {
              name = kubernetes_config_map.cerberus_config.metadata[0].name
            }
          }
          env_from {
            secret_ref {
              name = kubernetes_secret.cerberus_secret.metadata[0].name
            }
          }
          liveness_probe {
            http_get {
              path = "/healthz"
              port = 8070
            }
            initial_delay_seconds = 10
            period_seconds        = 10
          }
          readiness_probe {
            http_get {
              path = "/healthz"
              port = 8070
            }
            initial_delay_seconds = 5
            period_seconds        = 5
          }
        }
      }
    }
  }
}
