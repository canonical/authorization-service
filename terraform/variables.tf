# Copyright 2026 Canonical Ltd.
# SPDX-License-Identifier: Apache-2.0

variable "kubeconfig_path" {
  type        = string
  default     = "~/.kube/config"
  description = "Path to the kubeconfig file for the Canonical Kubernetes cluster."
}

variable "namespace" {
  type        = string
  default     = "authorization-service"
  description = "Kubernetes namespace where Authorization-service and its dependencies will be deployed."
}

variable "storage_class_name" {
  type        = string
  default     = null
  description = "StorageClass to use for PostgreSQL and stateful volumes (e.g. 'ceph-rbd'). If null, the cluster default storage class is used."
}

# -----------------------------------------------------------------------------
# Deployment Component Toggles
# -----------------------------------------------------------------------------

variable "deploy_postgres" {
  type        = bool
  default     = true
  description = "Whether to deploy the PostgreSQL database StatefulSet inside the cluster. If false, external PostgreSQL connection details must be supplied."
}

variable "deploy_openfga" {
  type        = bool
  default     = true
  description = "Whether to deploy the OpenFGA instance and run its schema migration/bootstrapping inside the cluster. If false, external OpenFGA details must be supplied."
}

# -----------------------------------------------------------------------------
# Authorization-service App Configuration
# -----------------------------------------------------------------------------

variable "authorization-service_image" {
  type        = string
  default     = "authorization-service:latest"
  description = "Authorization-service container image (can be a Docker image or Rock image)."
}

variable "authorization-service_image_pull_policy" {
  type        = string
  default     = "IfNotPresent"
  description = "Pull policy for the Authorization-service container image."
}

variable "authorization-service_replicas" {
  type        = number
  default     = 1
  description = "Number of replicas for the main Authorization-service server deployment."
}

# -----------------------------------------------------------------------------
# Secure Token Service (STS) Configuration
# -----------------------------------------------------------------------------

variable "sts_address" {
  type        = string
  default     = "secure-token-service.default.svc.cluster.local:9090"
  description = "The network address (hostname:port) of the Secure Token Service (STS)."
}

variable "sts_use_tls" {
  type        = bool
  default     = false
  description = "Whether to connect to the STS service using TLS."
}

variable "sts_eager_connection_check" {
  type        = bool
  default     = false
  description = "Whether Authorization-service should eagerly check connection to STS upon startup."
}

# -----------------------------------------------------------------------------
# External Authorization & Ory Hydra Configuration
# -----------------------------------------------------------------------------

variable "ext_authz_service_jwk_set_url" {
  type        = string
  default     = "http://secure-token-service.default.svc.cluster.local:8080/.well-known/jwks.json"
  description = "The JSON Web Key Set (JWKS) URL to verify internal STS tokens."
}

variable "ext_authz_service_hydra_jwk_set_url" {
  type        = string
  default     = "http://hydra.default.svc.cluster.local:4444/.well-known/jwks.json"
  description = "The Ory Hydra JWKS URL to verify incoming machine OAuth2 client credentials access tokens."
}

variable "ext_authz_service_hydra_issuer" {
  type        = string
  default     = "http://hydra.default.svc.cluster.local:4444/"
  description = "The expected token issuer claim (iss) for incoming Ory Hydra machine access tokens."
}

# -----------------------------------------------------------------------------
# PostgreSQL Database Settings (Internal or External)
# -----------------------------------------------------------------------------

variable "postgres_password" {
  type        = string
  default     = ""
  sensitive   = true
  description = "PostgreSQL password for internal database. If left empty, a secure password will be randomly generated."
}

variable "external_postgres_host" {
  type        = string
  default     = ""
  description = "Hostname/IP of the external PostgreSQL instance (Required if deploy_postgres is false)."
}

variable "external_postgres_port" {
  type        = number
  default     = 5432
  description = "Port of the external PostgreSQL instance."
}

variable "external_postgres_db" {
  type        = string
  default     = "authorization-service"
  description = "Database name of the external PostgreSQL instance for Authorization-service."
}

variable "external_postgres_user" {
  type        = string
  default     = "authorization-service"
  description = "Username of the external PostgreSQL instance."
}

variable "external_postgres_password" {
  type        = string
  default     = ""
  sensitive   = true
  description = "Password of the external PostgreSQL instance."
}

# -----------------------------------------------------------------------------
# OpenFGA Settings (Internal or External)
# -----------------------------------------------------------------------------

variable "openfga_image" {
  type        = string
  default     = "openfga/openfga:v1.14.1"
  description = "Container image to use for the OpenFGA server."
}

variable "external_openfga_address" {
  type        = string
  default     = ""
  description = "Endpoint address of the external OpenFGA instance (gRPC target, e.g., 'openfga.external.svc:8081') (Required if deploy_openfga is false)."
}

variable "external_openfga_store_id" {
  type        = string
  default     = ""
  description = "Pre-configured Store ID in the external OpenFGA workspace (Required if deploy_openfga is false)."
}

variable "external_openfga_authz_model_id" {
  type        = string
  default     = ""
  description = "Pre-configured Authorization Model ID in the external OpenFGA workspace (Required if deploy_openfga is false)."
}

variable "external_openfga_api_key" {
  type        = string
  default     = "42"
  sensitive   = true
  description = "API key for authentication with the external OpenFGA instance."
}

# -----------------------------------------------------------------------------
# Kafka Configuration
# -----------------------------------------------------------------------------

variable "kafka_enabled" {
  type        = bool
  default     = false
  description = "Whether to enable Kafka-based permission-update event ingestion."
}

variable "deploy_kafka" {
  type        = bool
  default     = false
  description = "Whether to deploy a lightweight, single-node KRaft-based Kafka instance inside the cluster."
}

variable "external_kafka_brokers" {
  type        = list(string)
  default     = []
  description = "Addresses of external Kafka brokers. (Required if kafka_enabled is true and deploy_kafka is false)."
}

variable "kafka_federated_services" {
  type        = list(string)
  default     = ["service-a", "service-b"]
  description = "List of federated service slugs whose permissions topics are to be created and listened to."
}

variable "kafka_consumer_group" {
  type        = string
  default     = "authz-listener"
  description = "The Kafka consumer group ID for the Authorization-service listener."
}

variable "kafka_topic_partitions" {
  type        = number
  default     = 1
  description = "The default number of partitions for auto-created Kafka topics."
}

variable "kafka_topic_replication_factor" {
  type        = number
  default     = 1
  description = "The default replication factor for auto-created Kafka topics."
}

# -----------------------------------------------------------------------------
# Worker Configuration
# -----------------------------------------------------------------------------

variable "worker_enabled" {
  type        = bool
  default     = false
  description = "Whether to deploy the permission-update background worker (which includes the in-process reaper)."
}

# -----------------------------------------------------------------------------
# Replicas & Scale
# -----------------------------------------------------------------------------

variable "authorization-service_listener_replicas" {
  type        = number
  default     = 1
  description = "Number of instances of the Kafka listener to run."
}

variable "authorization-service_worker_replicas" {
  type        = number
  default     = 1
  description = "Number of instances of the background worker to run."
}
