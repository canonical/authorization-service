# Copyright 2026 Canonical Ltd.
# SPDX-License-Identifier: Apache-2.0

resource "kubernetes_job" "openfga_migration" {
  count = var.deploy_openfga ? 1 : 0

  metadata {
    name      = "openfga-migration"
    namespace = kubernetes_namespace.authorization-service.metadata[0].name
    labels = {
      app = "openfga-migration"
    }
  }
  spec {
    template {
      metadata {
        labels = {
          app = "openfga-migration"
        }
      }
      spec {
        restart_policy = "OnFailure"
        init_container {
          name  = "wait-for-postgres"
          image = "postgres:14-alpine"
          env {
            name  = "PGPASSWORD"
            value = local.db_password
          }
          command = ["sh", "-c", "until pg_isready -h ${local.db_host} -p ${local.db_port} -U ${local.db_user}; do echo 'Waiting for Postgres...'; sleep 2; done; echo 'Postgres is ready!'"]
        }
        container {
          name    = "migrate"
          image   = var.openfga_image
          command = ["/openfga", "migrate"]
          env {
            name  = "OPENFGA_DATASTORE_ENGINE"
            value = "postgres"
          }
          env {
            name  = "OPENFGA_DATASTORE_URI"
            value = "postgresql://${local.db_user}:${local.db_password}@${local.db_host}:${local.db_port}/openfga?sslmode=disable"
          }
        }
      }
    }
    backoff_limit = 4
  }
  depends_on = [
    kubernetes_stateful_set.postgres
  ]
}

resource "kubernetes_service" "openfga" {
  count = var.deploy_openfga ? 1 : 0

  metadata {
    name      = "openfga"
    namespace = kubernetes_namespace.authorization-service.metadata[0].name
    labels = {
      app = "openfga"
    }
  }
  spec {
    port {
      name        = "http"
      port        = 8080
      target_port = 8080
    }
    port {
      name        = "grpc"
      port        = 8081
      target_port = 8081
    }
    selector = {
      app = "openfga"
    }
    type = "ClusterIP"
  }
}

resource "kubernetes_deployment" "openfga" {
  count = var.deploy_openfga ? 1 : 0

  metadata {
    name      = "openfga"
    namespace = kubernetes_namespace.authorization-service.metadata[0].name
    labels = {
      app = "openfga"
    }
  }
  spec {
    replicas = 1
    selector {
      match_labels = {
        app = "openfga"
      }
    }
    template {
      metadata {
        labels = {
          app = "openfga"
        }
      }
      spec {
        init_container {
          name  = "wait-for-migration"
          image = "postgres:14-alpine"
          env {
            name  = "PGPASSWORD"
            value = local.db_password
          }
          command = ["sh", "-c", "until psql -h ${local.db_host} -p ${local.db_port} -U ${local.db_user} -d openfga -c 'SELECT * FROM migration_version;' >/dev/null 2>&1; do echo 'Waiting for database migrations to complete...'; sleep 2; done; echo 'Migrations completed!'"]
        }
        container {
          name  = "openfga"
          image = var.openfga_image
          args  = ["run"]
          port {
            name           = "http"
            container_port = 8080
          }
          port {
            name           = "grpc"
            container_port = 8081
          }
          env {
            name  = "OPENFGA_DATASTORE_ENGINE"
            value = "postgres"
          }
          env {
            name  = "OPENFGA_DATASTORE_URI"
            value = "postgresql://${local.db_user}:${local.db_password}@${local.db_host}:${local.db_port}/openfga?sslmode=disable"
          }
          env {
            name  = "OPENFGA_AUTHN_PRESHARED_KEYS"
            value = "42"
          }
          env {
            name  = "OPENFGA_LOG_FORMAT"
            value = "text"
          }
          env {
            name  = "OPENFGA_HTTP_ADDR"
            value = "0.0.0.0:8080"
          }
          env {
            name  = "OPENFGA_GRPC_ADDR"
            value = "0.0.0.0:8081"
          }
          liveness_probe {
            http_get {
              path = "/healthz"
              port = 8080
            }
            initial_delay_seconds = 5
            period_seconds        = 5
          }
        }
      }
    }
  }
  depends_on = [
    kubernetes_job.openfga_migration
  ]
}
