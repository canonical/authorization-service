# Copyright 2026 Canonical Ltd.
# SPDX-License-Identifier: Apache-2.0

resource "kubernetes_namespace" "authorization-service" {
  metadata {
    name = var.namespace
    labels = {
      "app.kubernetes.io/managed-by" = "terraform"
      "app.kubernetes.io/name"       = "authorization-service"
    }
  }
}

resource "random_password" "postgres_password" {
  count   = var.deploy_postgres && var.postgres_password == "" ? 1 : 0
  length  = 16
  special = false
}

locals {
  # Resolve Database connection parameters
  db_host     = var.deploy_postgres ? "postgres" : var.external_postgres_host
  db_port     = var.deploy_postgres ? 5432 : var.external_postgres_port
  db_name     = var.deploy_postgres ? "authorization-service" : var.external_postgres_db
  db_user     = var.deploy_postgres ? "authorization-service" : var.external_postgres_user
  db_password = var.deploy_postgres ? (var.postgres_password != "" ? var.postgres_password : random_password.postgres_password[0].result) : var.external_postgres_password
  db_url      = "postgresql://${local.db_user}:${local.db_password}@${local.db_host}:${local.db_port}/${local.db_name}?sslmode=disable"

  openfga_db_url = var.deploy_postgres ? "postgresql://authorization-service:${local.db_password}@postgres:5432/openfga?sslmode=disable" : ""

  # Resolve OpenFGA parameters
  fga_address  = var.deploy_openfga ? "http://openfga:8081" : var.external_openfga_address
  fga_store_id = var.deploy_openfga ? "01GP1254CHWJC1MNGVB0WDG1T0" : var.external_openfga_store_id
  fga_api_key  = var.deploy_openfga ? "42" : var.external_openfga_api_key
  fga_model_id = var.deploy_openfga ? "" : var.external_openfga_authz_model_id
  
  # Resolve STS parameters
  sts_address              = var.sts_address
  sts_use_tls              = var.sts_use_tls ? "true" : "false"
  sts_eager_connection_check = var.sts_eager_connection_check ? "true" : "false"

  # Resolve Kafka parameters
  kafka_brokers = var.deploy_kafka ? ["kafka:9092"] : var.external_kafka_brokers
  worker_enabled = var.kafka_enabled || var.worker_enabled
}
