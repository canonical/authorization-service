# Copyright 2026 Canonical Ltd.
# SPDX-License-Identifier: Apache-2.0

variable "kubeconfig_path" {
  type        = string
  default     = "~/.kube/config"
  description = "Path to the kubeconfig file for the Canonical Kubernetes cluster."
}

variable "namespace" {
  type        = string
  default     = "cerberus"
  description = "Kubernetes namespace where Cerberus and its dependencies will be deployed."
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
# Cerberus App Configuration
# -----------------------------------------------------------------------------

variable "cerberus_image" {
  type        = string
  default     = "authorization-service:latest"
  description = "Cerberus container image (can be a Docker image or Rock image)."
}

variable "cerberus_image_pull_policy" {
  type        = string
  default     = "IfNotPresent"
  description = "Pull policy for the Cerberus container image."
}

variable "cerberus_replicas" {
  type        = number
  default     = 1
  description = "Number of replicas for the main Cerberus server deployment."
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
  description = "Whether Cerberus should eagerly check connection to STS upon startup."
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
  default     = "cerberus"
  description = "Database name of the external PostgreSQL instance for Cerberus."
}

variable "external_postgres_user" {
  type        = string
  default     = "cerberus"
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
