# Copyright 2026 Canonical Ltd.
# SPDX-License-Identifier: Apache-2.0

resource "kubernetes_service" "kafka" {
  count = var.deploy_kafka ? 1 : 0

  metadata {
    name      = "kafka"
    namespace = kubernetes_namespace.cerberus.metadata[0].name
    labels = {
      app = "kafka"
    }
  }
  spec {
    port {
      port        = 9092
      target_port = 9092
      name        = "plaintext"
    }
    selector = {
      app = "kafka"
    }
    type = "ClusterIP"
  }
}

resource "kubernetes_deployment" "kafka" {
  count = var.deploy_kafka ? 1 : 0

  metadata {
    name      = "kafka"
    namespace = kubernetes_namespace.cerberus.metadata[0].name
    labels = {
      app = "kafka"
    }
  }
  spec {
    replicas = 1
    selector {
      match_labels = {
        app = "kafka"
      }
    }
    template {
      metadata {
        labels = {
          app = "kafka"
        }
      }
      spec {
        container {
          name  = "kafka"
          image = "bitnami/kafka:3.4.1"
          port {
            container_port = 9092
          }
          env {
            name  = "KAFKA_CFG_NODE_ID"
            value = "1"
          }
          env {
            name  = "KAFKA_CFG_PROCESS_ROLES"
            value = "broker,controller"
          }
          env {
            name  = "KAFKA_CFG_CONTROLLER_QUORUM_VOTERS"
            value = "1@127.0.0.1:9093"
          }
          env {
            name  = "KAFKA_CFG_LISTENERS"
            value = "PLAINTEXT://:9092,CONTROLLER://:9093"
          }
          env {
            name  = "KAFKA_CFG_ADVERTISED_LISTENERS"
            value = "PLAINTEXT://kafka:9092"
          }
          env {
            name  = "KAFKA_CFG_LISTENER_SECURITY_PROTOCOL_MAP"
            value = "CONTROLLER:PLAINTEXT,PLAINTEXT:PLAINTEXT"
          }
          env {
            name  = "KAFKA_CFG_CONTROLLER_LISTENER_NAMES"
            value = "CONTROLLER"
          }
          env {
            name  = "ALLOW_PLAINTEXT_LISTENER"
            value = "yes"
          }
          volume_mount {
            name       = "kafka-data"
            mount_path = "/bitnami/kafka"
          }
        }
        volume {
          name = "kafka-data"
          empty_dir {}
        }
      }
    }
  }
}
