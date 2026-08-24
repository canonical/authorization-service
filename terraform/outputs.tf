# Copyright 2026 Canonical Ltd.
# SPDX-License-Identifier: Apache-2.0

output "namespace" {
  value       = kubernetes_namespace.authorization-service.metadata[0].name
  description = "The namespace where the application is deployed."
}

output "postgres_service" {
  value       = "${local.db_host}:${local.db_port}"
  description = "The active PostgreSQL database endpoint used by Authorization-service."
}

output "postgres_password" {
  value       = local.db_password
  sensitive   = true
  description = "The PostgreSQL database password (randomly generated or custom supplied)."
}

output "openfga_address" {
  value       = local.fga_address
  description = "The active OpenFGA endpoint address used by Authorization-service."
}

output "openfga_store_id" {
  value       = local.fga_store_id
  description = "The active OpenFGA Store ID used by Authorization-service."
}

output "sts_address" {
  value       = local.sts_address
  description = "The Secure Token Service (STS) endpoint address used by Authorization-service."
}

output "authorization-service_http" {
  value       = "http://authorization-service.${kubernetes_namespace.authorization-service.metadata[0].name}.svc.cluster.local:8070"
  description = "The internal HTTP REST endpoint for the Authorization-service service."
}

output "authorization-service_grpc" {
  value       = "authorization-service.${kubernetes_namespace.authorization-service.metadata[0].name}.svc.cluster.local:9091"
  description = "The internal gRPC endpoint for the Authorization-service service."
}

output "kafka_brokers" {
  value       = local.kafka_brokers
  description = "The active Kafka broker endpoints used by Authorization-service."
}
