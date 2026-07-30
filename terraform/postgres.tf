# Copyright 2026 Canonical Ltd.
# SPDX-License-Identifier: Apache-2.0

resource "kubernetes_secret" "postgres_secret" {
  count = var.deploy_postgres ? 1 : 0

  metadata {
    name      = "postgres-secret"
    namespace = kubernetes_namespace.cerberus.metadata[0].name
    labels = {
      app = "postgres"
    }
  }
  data = {
    postgres-password = local.db_password
    postgres-user     = "cerberus"
  }
}

resource "kubernetes_config_map" "postgres_init" {
  count = var.deploy_postgres ? 1 : 0

  metadata {
    name      = "postgres-init-scripts"
    namespace = kubernetes_namespace.cerberus.metadata[0].name
    labels = {
      app = "postgres"
    }
  }
  data = {
    "init.sql" = <<-EOT
      CREATE DATABASE openfga;
      CREATE DATABASE cerberus;
      GRANT ALL PRIVILEGES ON DATABASE openfga TO cerberus;
      GRANT ALL PRIVILEGES ON DATABASE cerberus TO cerberus;
    EOT
  }
}

resource "kubernetes_service" "postgres" {
  count = var.deploy_postgres ? 1 : 0

  metadata {
    name      = "postgres"
    namespace = kubernetes_namespace.cerberus.metadata[0].name
    labels = {
      app = "postgres"
    }
  }
  spec {
    port {
      port        = 5432
      target_port = 5432
    }
    selector = {
      app = "postgres"
    }
    type = "ClusterIP"
  }
}

resource "kubernetes_stateful_set" "postgres" {
  count = var.deploy_postgres ? 1 : 0

  metadata {
    name      = "postgres"
    namespace = kubernetes_namespace.cerberus.metadata[0].name
    labels = {
      app = "postgres"
    }
  }
  spec {
    replicas     = 1
    service_name = kubernetes_service.postgres[0].metadata[0].name
    selector {
      match_labels = {
        app = "postgres"
      }
    }
    template {
      metadata {
        labels = {
          app = "postgres"
        }
      }
      spec {
        container {
          name  = "postgres"
          image = "postgres:14-alpine"
          port {
            container_port = 5432
          }
          env {
            name = "POSTGRES_USER"
            value_from {
              secret_key_ref {
                name = kubernetes_secret.postgres_secret[0].metadata[0].name
                key  = "postgres-user"
              }
            }
          }
          env {
            name = "POSTGRES_PASSWORD"
            value_from {
              secret_key_ref {
                name = kubernetes_secret.postgres_secret[0].metadata[0].name
                key  = "postgres-password"
              }
            }
          }
          env {
            name  = "PGDATA"
            value = "/var/lib/postgresql/data/pgdata"
          }
          volume_mount {
            name       = "postgres-data"
            mount_path = "/var/lib/postgresql/data"
          }
          volume_mount {
            name       = "init-scripts"
            mount_path = "/docker-entrypoint-initdb.d"
          }
          liveness_probe {
            exec {
              command = ["pg_isready", "-U", "cerberus"]
            }
            initial_delay_seconds = 10
            period_seconds        = 5
            timeout_seconds       = 5
            failure_threshold     = 5
          }
        }
        volume {
          name = "init-scripts"
          config_map {
            name = kubernetes_config_map.postgres_init[0].metadata[0].name
          }
        }
      }
    }
    volume_claim_template {
      metadata {
        name = "postgres-data"
      }
      spec {
        access_modes       = ["ReadWriteOnce"]
        storage_class_name = var.storage_class_name
        resources {
          requests = {
            storage = "5Gi"
          }
        }
      }
    }
  }
}
