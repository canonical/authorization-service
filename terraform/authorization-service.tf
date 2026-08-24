# Copyright 2026 Canonical Ltd.
# SPDX-License-Identifier: Apache-2.0

resource "kubernetes_config_map" "authorization-service_config" {
  metadata {
    name      = "authorization-service-config"
    namespace = kubernetes_namespace.authorization-service.metadata[0].name
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

    # Kafka configuration (asynchronous pipeline)
    KAFKA_ENABLED                  = var.kafka_enabled ? "true" : "false"
    KAFKA_BROKERS                  = join(",", local.kafka_brokers)
    KAFKA_FEDERATED_SERVICES       = join(",", var.kafka_federated_services)
    KAFKA_CONSUMER_GROUP           = var.kafka_consumer_group
    KAFKA_TOPIC_PARTITIONS         = tostring(var.kafka_topic_partitions)
    KAFKA_TOPIC_REPLICATION_FACTOR = tostring(var.kafka_topic_replication_factor)

    # Worker configuration
    WORKER_ENABLED = local.worker_enabled ? "true" : "false"
  }
}

resource "kubernetes_secret" "authorization-service_secret" {
  metadata {
    name      = "authorization-service-secret"
    namespace = kubernetes_namespace.authorization-service.metadata[0].name
    labels = {
      app = "authorization-service"
    }
  }
  data = {
    POSTGRES_PASSWORD = local.db_password
    DATABASE_URL      = local.db_url
  }
}

resource "kubernetes_service_account" "authorization-service" {
  metadata {
    name      = "authorization-service"
    namespace = kubernetes_namespace.authorization-service.metadata[0].name
    labels = {
      app = "authorization-service"
    }
  }
}

resource "kubernetes_service" "authorization-service" {
  metadata {
    name      = "authorization-service"
    namespace = kubernetes_namespace.authorization-service.metadata[0].name
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

# 1. Main API Server Deployment (app serve)
resource "kubernetes_deployment" "authorization-service_server" {
  metadata {
    name      = "authorization-service"
    namespace = kubernetes_namespace.authorization-service.metadata[0].name
    labels = {
      app = "authorization-service"
    }
  }
  spec {
    replicas = var.authorization-service_replicas
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
        service_account_name = kubernetes_service_account.authorization-service.metadata[0].name

        # Wait until database is ready
        init_container {
          name  = "wait-for-postgres"
          image = "postgres:14-alpine"
          command = ["sh", "-c", "until pg_isready -h ${local.db_host} -p ${local.db_port} -U ${local.db_user}; do echo 'Waiting for PostgreSQL...'; sleep 2; done; echo 'PostgreSQL is ready!'"]
        }

        # Main API server process
        container {
          name              = "authorization-service"
          image             = var.authorization-service_image
          image_pull_policy = var.authorization-service_image_pull_policy
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
              name = kubernetes_config_map.authorization-service_config.metadata[0].name
            }
          }
          env_from {
            secret_ref {
              name = kubernetes_secret.authorization-service_secret.metadata[0].name
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

# 2. Kafka Ingestion Listener Deployment (app listen)
resource "kubernetes_deployment" "authorization-service_listener" {
  count = var.kafka_enabled ? 1 : 0

  metadata {
    name      = "authorization-service-listener"
    namespace = kubernetes_namespace.authorization-service.metadata[0].name
    labels = {
      app = "authorization-service-listener"
    }
  }
  spec {
    replicas = var.authorization-service_listener_replicas
    selector {
      match_labels = {
        app = "authorization-service-listener"
      }
    }
    template {
      metadata {
        labels = {
          app = "authorization-service-listener"
        }
      }
      spec {
        service_account_name = kubernetes_service_account.authorization-service.metadata[0].name

        # Wait until database is ready
        init_container {
          name  = "wait-for-postgres"
          image = "postgres:14-alpine"
          command = ["sh", "-c", "until pg_isready -h ${local.db_host} -p ${local.db_port} -U ${local.db_user}; do echo 'Waiting for PostgreSQL...'; sleep 2; done; echo 'PostgreSQL is ready!'"]
        }

        # Background listener process
        container {
          name              = "authorization-service-listener"
          image             = var.authorization-service_image
          image_pull_policy = var.authorization-service_image_pull_policy
          command           = ["/usr/bin/app", "listen"]
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
      }
    }
  }
}

# 3. Permission Application Worker Deployment (app worker + in-process reaper)
resource "kubernetes_deployment" "authorization-service_worker" {
  count = local.worker_enabled ? 1 : 0

  metadata {
    name      = "authorization-service-worker"
    namespace = kubernetes_namespace.authorization-service.metadata[0].name
    labels = {
      app = "authorization-service-worker"
    }
  }
  spec {
    replicas = var.authorization-service_worker_replicas
    selector {
      match_labels = {
        app = "authorization-service-worker"
      }
    }
    template {
      metadata {
        labels = {
          app = "authorization-service-worker"
        }
      }
      spec {
        service_account_name = kubernetes_service_account.authorization-service.metadata[0].name

        # Wait until database is ready
        init_container {
          name  = "wait-for-postgres"
          image = "postgres:14-alpine"
          command = ["sh", "-c", "until pg_isready -h ${local.db_host} -p ${local.db_port} -U ${local.db_user}; do echo 'Waiting for PostgreSQL...'; sleep 2; done; echo 'PostgreSQL is ready!'"]
        }

        # Background worker & in-process reaper process
        container {
          name              = "authorization-service-worker"
          image             = var.authorization-service_image
          image_pull_policy = var.authorization-service_image_pull_policy
          command           = ["/usr/bin/app", "worker"]
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
      }
    }
  }
}
