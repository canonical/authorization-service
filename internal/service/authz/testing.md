# Testing the Authorization Service Locally

Follow these steps in order to set up a complete local testing environment for the authorization service with OpenFGA integration.

## 1. Set up Kubernetes and Istio

If you need to test external authorization with Istio, follow the complete development setup guide at [k8s/dev-setup/development.md](../../k8s/dev-setup/development.md). This guide covers:
- Setting up Istio Ambient Mesh
- Configuring Skaffold profiles (istio, dev-setup, fake-webserver)
- Setting up networking aliases
- Testing the service through the Kubernetes Gateway

For basic unit and integration testing without Istio, you can skip this step.

## 2. Start Required Docker Services

Navigate to the `docker/dependencies` directory and start only the essential services needed for testing:

```bash
cd docker/dependencies
docker-compose up openfga migrateopenfga postgres insert-hardcoded-store
```

This will start:
- **postgres**: Database backend for OpenFGA (on port 5432)
- **migrateopenfga**: Runs database migrations for OpenFGA
- **openfga**: OpenFGA authorization engine (HTTP on port 8082, gRPC on port 8081)
- **insert-hardcoded-store**: Seeds the store ID `01GP1254CHWJC1MNGVB0WDG1T0` into OpenFGA

Wait for all services to become healthy before proceeding to the next step.

## 3. Run Database Migrations

Once the Docker services are running and healthy, initialize the application database schema:

```bash
./bin/authz-service migrate --dsn postgresql://authorization-service:password@localhost:5432/authorization-service up
```

This command will:
- Connect to the PostgreSQL database
- Apply all pending migrations
- Prepare the database for the authorization service

## 4. Write the Authorization Model to OpenFGA

Deploy the embedded DSL authorization model to the OpenFGA instance:

```bash
export STORE_ID="01GP1254CHWJC1MNGVB0WDG1T0"
./bin/authz-service authz write-model "$STORE_ID"
```

This command will:
- Read the embedded modular manifest file `fga.mod`
- Programmatically load and compile all core and federated schema files (such as `core/core.fga` and any modules under `services/`) into a single authorization model
- Write the compiled model to the specified OpenFGA store

Verify the model was written successfully by checking the output logs for the model ID.

## 5. Seed the Database with Test Rules

Populate the application database with test authorization rules:

```bash
psql postgresql://authorization-service:password@localhost:5432/authorization-service < testdata/manual/rules_seed.sql
```

This will:
- Connect to the application database
- Execute all SQL statements in the seed file
- Create sample rules for testing authorization queries

## 6. Seed OpenFGA with Test Tuples

Populate OpenFGA with relationship tuples that define user-object-relation bindings:

```bash
# Note: Implement a seeding mechanism using the tuples from testdata/manual/tuples.yaml
# You can use the OpenFGA API or SDK to load these tuples:
# POST /stores/{store_id}/write
# with the tuples from the YAML file
```

The `testdata/manual/tuples.yaml` file contains the relationship data to be written to OpenFGA. Use the OpenFGA gRPC or HTTP API to write these tuples to the store created in step 4.
