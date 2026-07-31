# Cerberus Terraform Deployment on Canonical Kubernetes

This directory contains the standard, production-ready **Terraform** configuration to deploy **Cerberus** (Authorization Service) and its core dependencies (**PostgreSQL**, **OpenFGA**) on **Canonical Kubernetes** (Canonical K8s).

It includes full integration with the Canonical internal **Secure Token Service (STS)** and supports highly flexible deployment topologies.

---

## Supported Deployment Topologies

This configuration supports three distinct topologies depending on your existing infrastructure context:

### 🚀 Topology A: Fully Self-Contained (Default)
Spins up everything in your cluster: PostgreSQL, OpenFGA, compiles/seeds the FGA models, and starts Cerberus pointing to an internal or external STS.

### 🌐 Topology B: Pre-deployed STS (Case 1)
STS is already deployed and maintained. You need to deploy Cerberus, PostgreSQL, and OpenFGA, and point Cerberus to the external STS instance.
- **How to configure**: Set the `sts_address` and `sts_use_tls` variables to target your pre-deployed STS.

### 🧬 Topology C: App-Only / External Dependencies (Case 2)
STS, PostgreSQL, and OpenFGA are already deployed (managed by cloud providers or other operations teams). You want to deploy **only** Cerberus, skipping internal database or OpenFGA pod creation, and connect Cerberus directly to these pre-existing resources.
- **How to configure**: Set `deploy_postgres = false` and `deploy_openfga = false`, and provide connection details (`external_postgres_*`, `external_openfga_*`). This automatically disables internal pod creation and skips the bootstrapping Job.

---

## Prerequisites

- **Canonical Kubernetes** is installed and running.
- Your terminal has cluster access configured (typically verified via `kubectl get nodes`).
- A default `StorageClass` is configured in the cluster (e.g. `ceph-rbd` or `cephfs` via Juju charms). You can supply a custom class name via the `storage_class_name` variable.

---

## Configuration

You can customize the deployment by passing variables to Terraform via a `terraform.tfvars` file or `-var` CLI flags.

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
| `cerberus_replicas` | `number` | `1` | Number of instances of Cerberus server to run. |
| `sts_address` | `string` | `secure-token-service.default.svc.cluster.local:9090` | Network endpoint of the pre-deployed STS. |
| `sts_use_tls` | `bool` | `false` | Enable TLS for connecting to STS. |
| `sts_eager_connection_check` | `bool` | `false` | Enable startup connection check to STS. |

---

## Configuration Examples (`terraform.tfvars`)

### Case 1: STS is already deployed, deploy everything else for Cerberus (Topology B)

Deploy Cerberus, PostgreSQL, and OpenFGA inside your namespace, and configure Cerberus to point to your pre-existing STS instance at IP `10.10.20.15:9090` using TLS:

```hcl
namespace          = "cerberus"
storage_class_name = "ceph-rbd"

# Target pre-deployed STS
sts_address                = "10.10.20.15:9090"
sts_use_tls                = true
sts_eager_connection_check = true

# Use standard internal DB & OpenFGA (will be auto-deployed and bootstrapped)
deploy_postgres = true
deploy_openfga  = true
```

### Case 2: STS and all other components are already deployed, deploy Cerberus only (Topology C)

Skip PostgreSQL and OpenFGA container deployments entirely, and deploy **only** Cerberus, connecting it to your pre-existing external databases, OpenFGA store, and STS service:

```hcl
namespace = "cerberus-prod"

# Toggle off internal dependencies & automated bootstrapping Job
deploy_postgres = false
deploy_openfga  = false

# Target pre-deployed STS
sts_address = "sts.prod.identity.svc.cluster.local:9090"
sts_use_tls = true

# Point directly to your pre-existing external PostgreSQL instance
external_postgres_host     = "postgres-ha.prod-db.svc.cluster.local"
external_postgres_port     = 5432
external_postgres_db       = "cerberus_db"
external_postgres_user     = "cerberus_app"
external_postgres_password = "MyHighlySecurePassword123"

# Point directly to your pre-existing external OpenFGA workspace & compiled model
external_openfga_address        = "openfga-ha.prod-fga.svc.cluster.local:8081"
external_openfga_store_id       = "01HM7G4J4B79J56K6M8Z2P2V9A"
external_openfga_authorization_model_id = "01HM7G5Z1A95V59W7P9F5N5M2B"
external_openfga_api_key        = "InternalSecretApiKey42"
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
---

## Security Considerations

- **Secrets Management**: Sensitive parameters (passwords, connection strings, OpenFGA keys) are safely stored as Kubernetes `Secret` resources and are not exposed as clear text in ConfigMaps.
- **Bare/Chiselled Image Compatibility**: Because production rock images do not contain shell/curl utilities, the bootstrapping Job isolates utility calls to dedicated standard helper images (`postgres:alpine` and `curlimages/curl`), allowing Cerberus's deployment to run highly secure and locked-down distroless rock containers.
