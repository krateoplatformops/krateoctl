# Krateo 2.7.0 to 3.0.0 Migration Guide

Krateo 3.0.0 introduces a major architectural shift from a cluster-resident controller to a stateless CLI-based management model. This guide covers the migration from the legacy installer-based approach to the new `krateo.yaml` workflow.

## What's Changed in 3.0.0

### Management Architecture
- **From:** Installer controller running inside the cluster (KrateoPlatformOps resource)
- **To:** External stateless CLI manager (`krateoctl`) that converges state and exits

### Configuration Model
- **From:** Single KrateoPlatformOps CRD with inline values
- **To:** Layered configuration model with `krateo.yaml`, optional `krateo-overrides.yaml`, and CLI flags

### State Management & Persistence
- **From:** In-cluster controller managing lifecycle; dedicated etcd for events storage
- **To:** Installation state stored as `Installation` custom resource snapshot; new CNPG-based PostgreSQL for events and resource persistence

### CLI & Workflow
- **From:** Limited, controller-driven operations
- **To:** Explicit `plan` → `apply` workflow with full preview capabilities via `krateoctl install` commands

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
- Krateo 2.7.0 is currently running in your cluster
- Latest `krateoctl` binary (3.0.0+) installed locally
- Adequate cluster permissions to manage resources in your Krateo namespace (default: `krateo-system`)

## Migration Steps

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

Generate the complete migration configuration for your infrastructure type:

```bash
krateoctl install migrate-full \
  --type <nodeport|loadbalancer|ingress> \
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

## Future Updates (3.0.0+)

After successful migration, upgrading to future 3.x versions is straightforward:

```bash
# Preview the upgrade
krateoctl install plan --version 3.0.0 --namespace krateo-system --type <nodeport|loadbalancer|ingress> --diff-installed --diff-format table

# Apply the upgrade
krateoctl install apply --version 3.0.0 --namespace krateo-system --type <nodeport|loadbalancer|ingress> 
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

### Deployment Configuration Profiles

The 3.0.0 installation uses profile-based configuration for different infrastructure types:

- **LoadBalancer profile**: For cloud environments (GCP, AWS, Azure) using external LoadBalancer services
- **OpenShift profile**: For OpenShift clusters with native security and service policies
- **NodePort/Ingress profiles**: For on-premise and hybrid environments

Each profile specifies the complete `KrateoPlatformOps` manifest with all 20+ components, their versions, and environment-specific settings. The appropriate profile is automatically selected based on your `--type` flag during `krateoctl install migrate-full`. See the [releases repository](https://github.com/krateoplatformops/releases) for available configuration profiles and detailed documentation.

## Getting Help

- Review existing [migration documentation](installation-migration.md)
- Check [install and upgrade guide](install-upgrade.md)
- Consult [secrets configuration](secrets.md)
- Enable debug logging: `KRATEOCTL_DEBUG=1 krateoctl install migrate-full`