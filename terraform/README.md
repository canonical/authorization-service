# Cerberus Authorization Service Terraform Module

This directory contains the production-grade Terraform configuration to deploy the **Cerberus (Authorization Service)** on **Canonical Kubernetes** (MicroK8s, Charmed Kubernetes, etc.). 

It supports highly modular deployment topologies, automating the orchestration, migration, database seeding, and background pipelines required to run a fine-grained, secure authorization plane at scale.

---

## Architecture Features

The Terraform configurations split Cerberus into three separate, scale-independent Kubernetes deployments:
1. **API Server (`authorization-service`)**: Serves synchronous gRPC and HTTP/REST transcoding requests (`app serve`).
2. **Kafka Listener (`authorization-service-listener`)**: Purely background, event-driven worker consuming permission updates from federated services (`app listen`). Deployed conditionally when Kafka is enabled.
3. **Background Worker & Reaper (`authorization-service-worker`)**: Purely background processor executing asynchronous tuple application to OpenFGA and thread-safe stale permission reaping (`app worker`). Deployed when Kafka is enabled or explicitly requested.

### Consolidated Bootstrapping Sequence
To support secure, distroless and chiselled rock container images containing no standard shell or diagnostic utilities, the system runs an unconditional, coordinated **`cerberus-bootstrap`** Job in the cluster prior to starting the workloads. The Job sequences operations via clean, lightweight init containers:
- `wait-for-dependencies`: Queries and waits until PostgreSQL and OpenFGA services are fully responsive.
- `seed-fga-store`: Seeds the OpenFGA Store schema (if deploying internal OpenFGA).
- `run-migrations`: Executes database schema migrations using `app migrate`.
- `write-fga-model`: Compiles and registers the OpenFGA authorization model.
- `ensure-topics`: Creates required Kafka topics idempotently using `app ensure-topics`.
- `run-seed`: Installs default route configurations in the PostgreSQL database.
- `patch-config` (Main container): Queries the compiled OpenFGA Model ID, dynamically patches the Kubernetes ConfigMap, and triggers a rolling rollout restart (`kubectl rollout restart`) across all three active workloads.

---

## Prerequisites

- **Canonical Kubernetes** is installed and running.
- Your terminal has cluster access configured (typically verified via `kubectl get nodes`).
- A default `StorageClass` is configured in the cluster (e.g. `ceph-rbd` or `cephfs` via Juju charms). You can supply a custom class name via the `storage_class_name` variable.

---

## Configuration

You can customise the deployment by passing variables to Terraform via a `terraform.tfvars` file or `-var` CLI flags.

### Input Variables

| Name | Type | Default | Description |
|------|------|---------|-------------|
| `kubeconfig_path` | `string` | `~/.kube/config` | Path to your kubeconfig. |
| `namespace` | `string` | `cerberus` | Kubernetes namespace to deploy all resources into. |
| `storage_class_name` | `string` | `null` | StorageClass to use for persistent volumes (e.g. `'ceph-rbd'`). |
| `deploy_postgres` | `bool` | `true` | Toggle PostgreSQL deployment. If false, external PG is used. |
| `deploy_openfga` | `bool` | `true` | Toggle OpenFGA deployment. If false, external OpenFGA is used. |
| `cerberus_image` | `string` | `authorization-service:latest` | Cerberus image (Docker or Rock). |
| `cerberus_image_pull_policy` | `string` | `IfNotPresent` | Image pull policy. |
| `cerberus_replicas` | `number` | `1` | Number of instances of the Cerberus server (serve) to run. |
| `sts_address` | `string` | `secure-token-service.default.svc.cluster.local:9090` | Network endpoint of the pre-deployed STS. |
| `sts_use_tls` | `bool` | `false` | Enable TLS for connecting to STS. |
| `sts_eager_connection_check` | `bool` | `false` | Enable startup connection check to STS. |
| `kafka_enabled` | `bool` | `false` | Whether to enable Kafka-based permission-update event ingestion. |
| `deploy_kafka` | `bool` | `false` | Whether to deploy a lightweight, single-node KRaft-based Kafka instance inside the cluster. |
| `external_kafka_brokers` | `list(string)` | `[]` | Addresses of external Kafka brokers. (Required if `kafka_enabled` is true and `deploy_kafka` is false). |
| `kafka_federated_services` | `list(string)` | `["service-a", "service-b"]` | List of federated service slugs whose permissions topics are to be created and listened to. |
| `kafka_consumer_group` | `string` | `"authz-listener"` | The Kafka consumer group ID for the Cerberus listener. |
| `kafka_topic_partitions` | `number` | `1` | The default number of partitions for auto-created Kafka topics. |
| `kafka_topic_replication_factor` | `number` | `1` | The default replication factor for auto-created Kafka topics. |
| `worker_enabled` | `bool` | `false` | Whether to deploy the permission-update background worker (which includes the in-process reaper). |
| `cerberus_listener_replicas` | `number` | `1` | Number of instances of the Kafka listener to run. |
| `cerberus_worker_replicas` | `number` | `1` | Number of instances of the background worker to run. |

---

## Configuration Examples (`terraform.tfvars`)

### Case 1: Simple Deployment with internal PostgreSQL & OpenFGA (No Kafka)
Deploy the core Cerberus HTTP/gRPC server, automatic database setup, and OpenFGA instance locally without deploying or connecting to Kafka:

```hcl
namespace          = "cerberus-core"
storage_class_name = "ceph-rbd"

sts_address = "sts.identity.svc.cluster.local:9090"

deploy_postgres = true
deploy_openfga  = true
kafka_enabled   = false
```

### Case 2: Full Pipeline Deployment with Internal Kafka (Topology B)
Deploy all architectural components inside the namespace, including a lightweight self-contained Kafka instance utilizing **KRaft mode (Zookeeperless)**:

```hcl
namespace          = "cerberus"
storage_class_name = "ceph-rbd"

# Target pre-deployed STS
sts_address = "sts.identity.svc.cluster.local:9090"

# Deploy local databases & identity stores
deploy_postgres = true
deploy_openfga  = true

# Deploy & enable local Kafka ingestion pipeline
kafka_enabled    = true
deploy_kafka     = true
worker_enabled   = true

kafka_federated_services = ["orders-service", "billing-service", "inventory-service"]

# Customise replicas for workload scaling
cerberus_replicas          = 2
cerberus_listener_replicas = 1
cerberus_worker_replicas   = 1
```

### Case 3: Enterprise Production Deployment with External Dependencies (Topology C)
Skip PostgreSQL, OpenFGA, and Kafka container deployments entirely, routing all traffic and pipelines to existing, highly-available external infrastructure:

```hcl
namespace = "cerberus-prod"

# Toggle off internal dependencies
deploy_postgres = false
deploy_openfga  = false
deploy_kafka    = false

# Target pre-deployed STS
sts_address = "sts.prod.identity.svc.cluster.local:9090"
sts_use_tls = true

# Enable Kafka pipelines
kafka_enabled  = true
worker_enabled = true

# Point directly to your pre-existing external PostgreSQL instance
external_postgres_host     = "postgres-ha.prod-db.svc.cluster.local"
external_postgres_port     = 5432
external_postgres_db       = "cerberus_db"
external_postgres_user     = "cerberus_app"
external_postgres_password = "MyHighlySecurePassword123"

# Point directly to your pre-existing external OpenFGA workspace & compiled model
external_openfga_address                = "openfga-ha.prod-fga.svc.cluster.local:8081"
external_openfga_store_id               = "01HM7G4J4B79J56K6M8Z2P2V9A"
external_openfga_authorization_model_id = "01HM7G5Z1A95V59W7P9F5N5M2B"
external_openfga_api_key                = "InternalSecretApiKey42"

# Point directly to your enterprise-wide production Kafka Brokers
external_kafka_brokers   = ["kafka-0.prod-stream.svc.cluster.local:9092", "kafka-1.prod-stream.svc.cluster.local:9092"]
kafka_federated_services = ["identity", "payment", "catalogue"]
```

---

## Quick Start

1. **Change Directory**:
   ```bash
   cd terraform
   ```
2. **Create custom config file (`terraform.tfvars`)** based on one of the examples above.
3. **Initialize Terraform**:
   ```bash
   terraform init
   ```
4. **Deploy**:
   ```bash
   terraform apply
   ```

---

## Verification & Usage

### 1. View Outputs
Once the apply finishes, Terraform prints service URLs and configuration details:
```bash
terraform output
```

### 2. Check Service Health Locally
```bash
kubectl port-forward svc/authorization-service 8070:8070 -n <your-namespace>
```
Run:
```bash
curl http://localhost:8070/healthz
```
*Response:*
```json
{
  "status": "SERVING"
}
```

### 3. Check Kafka Listener Logs
If Kafka is enabled, verify that the consumer is running and connected successfully:
```bash
kubectl logs deployment/authorization-service-listener -n <your-namespace>
```

---

## Security Considerations

- **Secrets Management**: Sensitive parameters (passwords, connection strings, OpenFGA keys) are safely stored as Kubernetes `Secret` resources and are not exposed as clear text in ConfigMaps.
- **Bare/Chiselled Image Compatibility**: Because production rock images do not contain shell/curl utilities, the bootstrapping Job isolates utility calls to dedicated standard helper images (`postgres:alpine` and `curlimages/curl`), allowing Cerberus's deployments to run highly secure and locked-down distroless rock containers.
- **RBAC Minimisation**: The `cerberus-bootstrapper` ServiceAccount is strictly bound via RBAC roles to only allow read, patch, and update operations for ConfigMaps and Deployments within its own namespace.
