# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is the **Governance Policy Addon Controller** for Open Cluster Management (OCM). It manages installations of policy addons on managed clusters using ManifestWorks. The controller manages four main policy addons:

- **config-policy-controller**: Configuration Policy Controller
- **cert-policy-controller**: Certificate Policy Controller
- **governance-policy-framework**: Governance Policy Framework Addon
- **governance-standalone-hub-templating**: Standalone templating for policies

## Development Commands

### Building and Testing
- `make build` - Build the controller binary
- `make test` - Run unit tests (excludes e2e tests)
- `make test-coverage` - Run tests with coverage reporting
- `make vet` - Run go vet against code
- `make build-images` - Build Docker images with fmt, vet, and generate

### Development Environment Setup
- `make kind-bootstrap-cluster` - Bootstrap Kind clusters and load images
- `make kind-bootstrap-cluster-dev` - Bootstrap Kind clusters for local development (no image loading)
- `make kind-deploy-controller` - Deploy controller to Kind cluster (includes OCM setup)
- `make kind-deploy-addons` - Deploy basic ManagedClusterAddons to all managed clusters
- `make wait-for-work-agent` - Wait for klusterlet work agent to start

### Local Development
- `make kind-run-local` - Run the controller locally against Kind cluster
- `make kind-load-image` - Build and load Docker image into Kind
- `make kind-regenerate-controller` - Refresh/redeploy the controller on Kind

### Cleanup
- `make kind-bootstrap-delete-clusters` - Delete clusters created from bootstrap
- `make clean` - Clean up generated files and Kind clusters

### E2E Testing
- `make e2e-test` - Run E2E tests (requires Kind cluster setup)
- `make e2e-debug` - Collect debug logs from deployed clusters

### Management and Troubleshooting
- `make manifests` - Generate RBAC, CRD, and webhook manifests using controller-gen
- `make generate` - Generate DeepCopy methods using controller-gen
- `make install-resources` - Deploy RBAC and service account to cluster
- Annotation `policy-addon-pause=true` on ManagedClusterAddOn - pauses automatic updates for testing

## Architecture

### Main Entry Point
- `main.go` - CLI with controller subcommand, sets up logging and addon manager

### Addon Structure
Each policy addon follows a consistent pattern in `pkg/addon/`:
- `agent_addon.go` - Implements addon-framework interfaces
- `manifests/hubpermissions/` - RBAC for hub cluster permissions
- `manifests/managedclusterchart/` - Helm chart deployed to managed clusters via ManifestWork

### Core Components
- **AddonManager** - OCM addon-framework manager that orchestrates all policy addons
- **PolicyAgentAddon** - Wrapper that adds pause functionality and common behavior
- **Agent Registration** - Handles CSR approval and permission configuration
- **Helm Charts** - Templates for deploying controllers to managed clusters

### Key Annotations
- `policy-addon-pause` - Pauses addon automatic updates
- `log-level` - Sets logging level (integer, higher = more verbose)
- `addon.open-cluster-management.io/values` - Helm values override (JSON)
- `addon.open-cluster-management.io/on-multicluster-hub` - For self-managed hubs
- `prometheus-metrics-enabled` - Enable/disable Prometheus metrics

### Configuration Sources
- Environment variables for default images (see each addon's agent_addon.go)
- `values.yaml` files in each addon's managedclusterchart directory
- AddonDeploymentConfig for deployment customization
- Addon annotations for runtime configuration

## Test Requirements

Before submitting PRs, run:
```shell
make test
```

The project uses KinD for integration testing. E2E tests are run against Kind clusters and should be executed locally when possible to thoroughly test changes.