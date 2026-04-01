# Krateo 2.7.0 to 3.0.0 Migration Guide

Krateo 3.0.0 introduces a major architectural shift from a cluster-resident controller to a stateless CLI-based management model. The core goal of this migration is to move Krateo installation and upgrade management away from the in-cluster installer controller to the external `krateoctl` CLI tool, giving you explicit control over the full lifecycle through `plan` and `apply` workflows. This guide covers the migration from the legacy installer-based approach to the new `krateo.yaml` + `krateoctl` workflow.

## Table of Contents

- [What's Changed in 3.0.0](#whats-changed-in-30)
- [Prerequisites](#prerequisites)
- [Migration Steps](#migration-steps)
- [Secrets Configuration](#secrets-configuration)
- [Migration for Different Environments](#migration-for-different-environments)
- [Fresh Installations or Future Updates](#fresh-installations-or-future-updates)
- [About Pre-Upgrade Cleanup and Deployment Profiles](#about-pre-upgrade-cleanup-and-deployment-profiles)
- [Getting Help](#getting-help)

## What's Changed in 3.0.0

### Infrastructure Updates

#### Database Layer
- **CNPG (CloudNativePG):** Production-grade PostgreSQL on Kubernetes, replacing dedicated etcd
  - Kubernetes events now persisted in PostgreSQL
  - Subset of Krateo resources stored for frontend consumption
  - Improved query performance and scalability

- **Deviser:** New component for PostgreSQL database preparation, maintenance, and lifecycle management tasks

### Component Upgrades

#### Events Stack (Completely Rewritten)
- **Old:** eventsse and eventrouter components
- **New:** Ingester + Presenter architecture
  - Ingester: Collects and processes Kubernetes events
  - Presenter: Exposes events to frontend
  - OpenTelemetry metrics now available for observability
  - Transparent replacement with improved performance

#### Resources Stack (New)
- **Ingester + Presenter model:** Mirrors Events Stack pattern
- **Storage:** Stores defined Krateo resources in PostgreSQL
- **Performance:** Serves resources to frontend at significantly higher speed
- **UX Impact:** More responsive frontend, smoother user experience

#### Core Provider
- Enhanced certificate management with periodic reconciliation and retry logic
- Ensures consistent CA bundle propagation before composition definitions go ready
- Improved error handling and panic detection in composition operations
- Fixed race conditions in certificate synchronization
- Integrated optimized shared Helm library

#### Composition Dynamic Controller
- Safe release name option to disable random Helm suffix (configurable per composition)
- Refactored package structure for maintainability
- Improved event recorder throttling to reduce noise
- Integrated optimized shared Helm library

#### OASGen Provider
- Generated controllers emit fewer Kubernetes events through improved throttling
- Better event aggregation reduces cluster noise

#### Autopilot (Enhanced)
- Context caching and compression for improved agent precision and speed
- Multi-version support: now handles Krateo 2.6, 2.7, and 3.0
- Improved agent instructions and descriptions for higher-quality generated resources

#### Frontend (Significant Updates)
- Row-level actions inside Table widgets for direct resource manipulation
- Updated EventList and Notifications to new events format
- Cursor-based pagination for improved performance on large datasets
- Theme and logo customization support
- Fixed UI and logical issues: notifications display, login flows, form redirects, event logging

## Prerequisites

- kubectl configured and connected to your cluster
- **Krateo 2.7.0 is currently running in your cluster** with the installer controller
- Latest `krateoctl` binary (3.0.0+) installed locally
- Adequate cluster permissions to manage resources in your Krateo namespace (default: `krateo-system`)

> [!NOTE]
> These migration steps are **only for existing Krateo 2.7.0 installations** managed by the installer controller. For fresh Krateo 3.0.0 installations on a new cluster, use `krateoctl install apply` directly with your `krateo.yaml` configuration file instead of the migration commands.

## Migration Steps

> [!IMPORTANT]
> These steps apply **only when migrating from an existing Krateo 2.7.0 installation** with the installer controller running in your cluster.
>
> For **fresh Krateo 3.0.0 installations** on a new cluster, see [Install and Upgrade](install-upgrade.md) and use `krateoctl install apply` directly with your `krateo.yaml` file.

### 1. Prepare Your Environment

Ensure your current installation is running smoothly:

```bash
kubectl get krateoplatformops -n krateo-system
kubectl get pods -n krateo-system
```

### 2. Backup Current Configuration

Export your legacy configuration for reference:

```bash
kubectl get krateoplatformops krateo -n krateo-system -o yaml > krateo-2.7.0-backup.yaml
```

### 3. Generate Migration Plan

Preview what will be migrated before applying:

```bash
krateoctl install migrate --namespace krateo-system --output krateo.yaml
```

This generates a new `krateo.yaml` file with your configuration converted to the new format. Review it carefully:

```bash
cat krateo.yaml
```

### 4. Understand the Layered Configuration

In 3.0.0, configuration follows this precedence (highest to lowest):

1. CLI flags (`--set`, `--type`, shortcuts like `--openshift`)
2. `krateo-overrides.yaml` (optional user overrides)
3. `krateo.yaml` (base release profile)
4. Hardcoded defaults

If you need to customize the installation, create `krateo-overrides.yaml` instead of modifying `krateo.yaml` directly.

### 5. Generate the Full Migration Plan

Generate the complete migration configuration for your infrastructure type. If using OpenShift, add the `--profile openshift` flag:

```bash
krateoctl install migrate-full \
  --type <nodeport|loadbalancer|ingress> \
  --namespace krateo-system \
  --output krateo.yaml

# For OpenShift:
krateoctl install migrate-full \
  --type loadbalancer \
  --profile openshift \
  --namespace krateo-system \
  --output krateo.yaml
```

This generates the new `krateo.yaml` with all 3.0.0 components and configurations. Review the generated file:

```bash
cat krateo.yaml
# Make any customizations by creating krateo-overrides.yaml
```

### 6. Apply the Migration

Once you've reviewed the plan, apply the migration to your cluster:

```bash
krateoctl install apply \
  --namespace krateo-system
```

This will:
- Run the pre-upgrade cleanup job to remove deprecated 2.7.0 components
- Delete the legacy `KrateoPlatformOps` resource
- Uninstall old Helm releases and persistent volumes (e.g., etcd storage)
- Install all 3.0.0 components in correct dependency order
- Create the `Installation` resource snapshot

#### Deprecated Components Removed During Application

The pre-upgrade cleanup job automatically removes 2.7.0 components that are replaced in 3.0.0. See the [pre-upgrade cleanup script](../../releases/pre-upgrade.yaml) for details. Components removed:

| Removed Component | Replacement | Reason |
| --- | --- | --- |
| **eventsse** | Events Stack Ingester/Presenter | Rewritten for better performance and OpenTelemetry support |
| **eventrouter** | Events Stack Ingester/Presenter | Merged into new events system architecture |
| **eventsse-etcd** | CNPG PostgreSQL | Dedicated etcd replaced with managed PostgreSQL database |
| **sweeper** | Database maintenance (Deviser) | Rolled into new database lifecycle management |
| **finops-composition-definition-parser** | (Reimplemented) | Refactored as part of FinOps stack redesign |

The cleanup job also removes persistent volumes (e.g., `etcd-data-eventsse-etcd-0`) to ensure a clean migration path.

### 7. Verify Migration Success

Check that the migration completed successfully:

```bash
# Verify new Installation snapshot exists
kubectl get installation -n krateo-system

# Verify Krateo components are running
kubectl get pods -n krateo-system

# Verify no legacy resources remain
kubectl get krateoplatformops -n krateo-system || echo "Legacy resources cleaned up"

# Check the installation state snapshot
kubectl get installation krateoctl -n krateo-system -o yaml
```

## Secrets Configuration

Krateo 3.0.0 does not bootstrap production secrets. Secrets must be created and managed separately before or during the migration. See the full [Secrets Spec](secrets.md) for detailed requirements and architecture.

### Creating Required Secrets

Before running migration, create the required secrets in your install namespace. Krateo 3.0.0 requires the following secrets:

```bash
# Generate URL-safe random JWT signing key
JWT_SIGN_KEY=$(openssl rand -base64 32 | tr '+/' '-_' | tr -d '=')

# Generate URL-safe database password (use the same for both secrets)
DB_PASS=$(openssl rand -base64 32 | tr '+/' '-_' | tr -d '=')

# Create jwt-sign-key secret
kubectl create secret generic jwt-sign-key \
  --from-literal=JWT_SIGN_KEY="$JWT_SIGN_KEY" \
  -n krateo-system

# Create krateo-db secret (for stack components)
kubectl create secret generic krateo-db \
  --from-literal=DB_USER=krateo-db-user \
  --from-literal=DB_PASS="$DB_PASS" \
  -n krateo-system

# Create krateo-db-user secret (for CNPG/database)
# Must use the same password as krateo-db
kubectl create secret generic krateo-db-user \
  --from-literal=username=krateo-db-user \
  --from-literal=password="$DB_PASS" \
  -n krateo-system
```

**Important consistency rules:**
- `krateo-db` and `krateo-db-user` **must** use the same password value
- The `jwt-sign-key` should be generated once and kept stable across upgrades
- All secrets must be created in the same namespace as your Krateo installation
- Passwords are generated as URL-safe strings (no `/`, `+`, or `=` characters)

### List of Required Secrets

The three required secrets are:

1. **jwt-sign-key** - JWT signing key used by platform components
2. **krateo-db** - Database credentials for stack components (DB_USER, DB_PASS)
3. **krateo-db-user** - Database credentials for CNPG/database (username, password)

Refer to [Secrets Spec](secrets.md) for complete details on secret structure, naming, and management best practices.

## Migration for Different Environments

> [!NOTE]
> This section shows how to **migrate from Krateo 2.7.0 to 3.0.0** with different infrastructure types. The `--type` flag determines how your migrated Krateo will be exposed to users.
>
> For **fresh Krateo 3.0.0 installations** (not migration), see [Install and Upgrade](install-upgrade.md).

The `--type` flag during migration determines the deployment profile and networking configuration for your Krateo installation. Choose based on your infrastructure:

### NodePort (Kind, Local Development, Air-Gapped Clusters)

**Best for:** Local development with Kind, Minikube, or air-gapped Kubernetes deployments.

**Characteristics:**
- Services exposed on high-numbered ports (30000-32767) on each node
- No external load balancer required
- Direct node IP access needed

**Installation:**
```bash
# Step 1: Migrate from controller-based 2.7.0 to CLI-based management
krateoctl install migrate-full \
  --type nodeport \
  --namespace krateo-system \
  --output krateo.yaml

# Step 2: Apply to upgrade from 2.7.0 to 3.0.0
krateoctl install apply \
  --version 3.0.0 \
  --type nodeport \
  --namespace krateo-system
```

**Access Krateo after upgrade completes:**
```bash
# Get the node IP and service port
NODE_IP=$(kubectl get nodes -o jsonpath='{.items[0].status.addresses[?(@.type=="InternalIP")].address}')
SERVICE_PORT=$(kubectl get svc krateo-frontend -n krateo-system -o jsonpath='{.spec.ports[0].nodePort}')

# Access via: http://$NODE_IP:$SERVICE_PORT
echo "Access Krateo at: http://$NODE_IP:$SERVICE_PORT"
```

**For Kind clusters:**
```bash
# If using Kind, port-forward for local access
kubectl port-forward -n krateo-system svc/krateo-frontend 8080:80 &
# Access at: http://localhost:8080
```

### LoadBalancer (Cloud Providers: AWS, GCP, Azure)

**Best for:** Cloud-native Kubernetes clusters with external load balancer support (EKS, GKE, AKS).

**Characteristics:**
- Automatic external load balancer provisioning (IP or hostname)
- Cloud provider handles load balancing and routing
- No domain required (use cloud provider's assigned hostname)

**Installation:**
```bash
# Step 1: Migrate from controller-based 2.7.0 to CLI-based management
krateoctl install migrate-full \
  --type loadbalancer \
  --namespace krateo-system \
  --output krateo.yaml

# Step 2: Apply to upgrade from 2.7.0 to 3.0.0
krateoctl install apply \
  --version 3.0.0 \
  --type loadbalancer \
  --namespace krateo-system
```

**Access Krateo:**
```bash
# Get the external LoadBalancer IP/hostname
kubectl get svc krateo-frontend -n krateo-system

# External IP will be assigned by your cloud provider
# Access: http://<EXTERNAL-IP> or http://<EXTERNAL-HOSTNAME>
```

**Example outputs:**
```
# AWS/GCP/Azure
NAME              TYPE           CLUSTER-IP       EXTERNAL-IP
krateo-frontend   LoadBalancer   10.0.0.100       a1b2c3d4-123456789.elb.amazonaws.com
```

### Ingress (Cloud Providers with Custom Domains)

**Best for:** Production deployments using custom domains, certificate management via cert-manager, and HTTP/HTTPS routing.

**Characteristics:**
- Uses Kubernetes Ingress resource for routing
- Support for custom domains and TLS certificates
- Requires Ingress Controller (nginx-ingress, traefik, etc.)
- More control over routing and SSL/TLS

**Installation:**
```bash
# Step 1: Migrate from controller-based 2.7.0 to CLI-based management
krateoctl install migrate-full \
  --type ingress \
  --namespace krateo-system \
  --output krateo.yaml

# Step 2: Apply to upgrade from 2.7.0 to 3.0.0
krateoctl install apply \
  --version 3.0.0 \
  --type ingress \
  --namespace krateo-system
```

**Configure your domain:**

#### Option 1: Local Environment (without domain registration)

#### Option 2: Production/Cloud Environment (with domain registration)

### OpenShift (with OpenShift Profile)

**Best for:** OpenShift Container Platform with native security, RBAC, and route management.

**Installation:**
OpenShift requires combining a `--type` with the `--profile openshift` flag:

```bash
# Step 1: Migrate from controller-based 2.7.0 to CLI-based management
krateoctl install migrate-full \
  --type loadbalancer \
  --namespace krateo-system \
  --output krateo.yaml

# Step 2: Apply to upgrade from 2.7.0 to 3.0.0
krateoctl install apply \
  --version 3.0.0 \
  --type loadbalancer \
  --profile openshift \
  --namespace krateo-system
```

This applies the OpenShift-specific profile which includes:
- Restricted security context for pod security policies
- OpenShift RBAC configurations
- Routes for service exposure (handles networking automatically)
- OpenShift-native networking and security

## Fresh Installations or Future Updates

After successful migration, upgrading to future 3.x versions is straightforward:

```bash
# Preview the upgrade
krateoctl install plan --version 3.0.0 --namespace krateo-system --type <nodeport|loadbalancer|ingress> --diff-installed --diff-format table [--profile openshift]

# Apply the upgrade
krateoctl install apply --version 3.0.0 --namespace krateo-system --type <nodeport|loadbalancer|ingress> [--profile openshift] 
```

See [Install and Upgrade](install-upgrade.md) for detailed instructions.

## About Pre-Upgrade Cleanup and Deployment Profiles

The migration process uses standardized deployment configurations that handle both initial installations and version upgrades. These configurations are maintained in the [Krateo releases repository](https://github.com/krateoplatformops/releases).

### Pre-Upgrade Cleanup Process

Before applying the 3.0.0 configuration, the `pre-upgrade.yaml` script runs as a Kubernetes Job to safely remove deprecated components:

**What it removes:**
- Helm releases for: `eventsse`, `eventrouter`, `eventsse-etcd`, `sweeper`, `finops-composition-definition-parser`
- Persistent volumes associated with deprecated components (e.g., etcd data volumes from 2.7.0)

**Why it's needed:**
- Prevents version conflicts between old and new event/resource systems
- Clears storage allocated to deprecated components
- Ensures clean state before installing 3.0.0 components

**How it works:**
```bash
# Pre-upgrade runs automatically during migrate-full, but you can also run it manually:
kubectl apply -f pre-upgrade.yaml
kubectl wait --for=condition=complete job/uninstall-old-components -n krateo-system
```

### Deployment Configuration Profiles and Types

The 3.0.0 installation uses two separate configuration mechanisms:

**Type (`--type`)** determines the networking/service exposure method:
- **LoadBalancer**: For cloud environments (GCP, AWS, Azure) using external LoadBalancer services
- **NodePort**: For local development and air-gapped environments
- **Ingress**: For production with custom domains and Ingress Controllers

**Profile (`--profile`)** applies environment-specific configurations and overrides:
- **openshift**: Enables OpenShift-specific security contexts, RBAC, Routes, and pod security policies. Use with any `--type`.
- **monitoring**: Adds enhanced observability and metrics collection (optional)
- **debug**: Enables verbose logging and debugging output (optional)

Example usage:
```bash
# Standard cloud deployment
krateoctl install migrate-full --type loadbalancer ...

# OpenShift deployment
krateoctl install migrate-full --type loadbalancer --profile openshift ...

# Local development with monitoring
krateoctl install migrate-full --type nodeport --profile monitoring ...
```

Each combination specifies the complete `KrateoPlatformOps` manifest with all 20+ components, their versions, and environment-specific settings. See the [releases repository](https://github.com/krateoplatformops/releases) for available profiles and detailed documentation.

## Getting Help

- Review existing [migration documentation](installation-migration.md)
- Check [install and upgrade guide](install-upgrade.md)
- Consult [secrets configuration](secrets.md)
- Enable debug logging: `KRATEOCTL_DEBUG=1 krateoctl install migrate-full`