# Install and Upgrade

`krateoctl install plan` and `krateoctl install apply` cover the main install and upgrade workflows for Krateo.

- `plan` previews what `krateoctl` would do, without talking to the cluster.
- `apply` executes the workflow against the cluster.

## Table Of Contents

- [Release Source](#release-source)
- [Installation Snapshot](#installation-snapshot)
- [Plan Command](#plan-command)
- [Apply Command](#apply-command)
- [Upgrade Flow](#upgrade-flow)
- [Notes](#notes)

## When To Use Them

Use these commands when you want to:

- install Krateo from a local `krateo.yaml`
- upgrade to a tagged release version
- compare the computed plan with the last stored installation snapshot
- run type-specific workflows for `nodeport`, `loadbalancer`, or `ingress`

## Release Source

When you pass `--version`, `krateoctl` switches to remote mode and fetches files from the releases repository instead of the local filesystem.

The default repository is:

- [https://github.com/krateoplatformops/releases](https://github.com/krateoplatformops/releases)

This repository is used as a versioned source of installation assets. `krateoctl` builds raw GitHub URLs from it and looks for files such as:

- `krateo.yaml`
- `krateo-overrides.yaml`
- `krateo-overrides.<profile>.yaml`
- `pre-upgrade.yaml`
- `pre-upgrade.<type>.yaml`
- `post-upgrade.yaml`
- `post-upgrade.<type>.yaml`

If you maintain your own release repository, you can point `--repository` at a GitHub repo with the same layout.

## Installation Snapshot

`krateoctl` saves the resolved installation state as an `Installation` custom resource in the `krateo.io/v1` API group.

The snapshot contains:

- the resolved `componentsDefinition`
- the computed `steps`
- the `installationVersion` used to build it

By default, the snapshot is stored with the name `krateoctl` in the install namespace.

### How It Is Used

- `krateoctl install apply` saves the snapshot after a successful apply.
- `krateoctl install plan --diff-installed` loads the stored snapshot and compares it with the newly computed plan.
- `krateoctl install migrate-full` also saves the snapshot as part of the automatic migration flow.

### Why It Exists

The snapshot gives `krateoctl` a durable record of what was last computed and applied. That makes it easier to:

- compare a new release against the current state
- detect drift between the installed workflow and the newly computed one
- keep a versioned record of the installation logic that produced the cluster state

### Notes

- The snapshot resource is namespaced.
- The CRD is installed automatically by `krateoctl install apply` and the migration flow if it is missing.
- If the snapshot is not present, `plan --diff-installed` reports that it could not find one and continues without a diff.

## Secrets

Secrets are managed separately from the install workflow. The recommended approach is to store them in Vault and sync them into Kubernetes.

See the full [Secrets Spec](secrets.md) for the required names, keys, and namespace rules.

## Plan Command

`krateoctl install plan` is the command that loads the configuration, computes the workflow, and prints the result as multi-document YAML or as a diff summary.

### Usage

```sh
krateoctl install plan [FLAGS]
```

### Key Flags

- `--version` release tag to fetch from the releases repository
- `--repository` custom GitHub repository URL for release assets, default `https://github.com/krateoplatformops/releases`
- `--config` local configuration file, default `krateo.yaml`
- `--profile` optional profile name, such as `dev` or `prod`
- `--namespace` namespace where the installation snapshot is stored
- `--type` file variant to use, such as `nodeport`, `loadbalancer`, or `ingress`
- `--diff-installed` compare the computed plan against the stored installation snapshot
- `--diff-format` choose how diffs are rendered; use `table` for a per-step summary view
- `--output` emit the computed plan as YAML to stdout
- `--skip-validation` skip configuration validation
- `--debug` enable debug logging, or set `KRATEOCTL_DEBUG`

### How It Works

1. Loads the main config from `krateo.yaml` or from the releases repository when `--version` is set.
2. Applies `krateo-overrides.yaml` and profile-specific overrides if present.
3. Selects type-specific files first, then falls back to the generic ones.
4. Computes the workflow steps.
5. Optionally compares the plan against the last saved installation snapshot.

### Examples

```sh
# Preview the local installation config
krateoctl install plan
```

```sh
# Preview a specific release version from the default releases repository
krateoctl install plan --version v1.0.0
```

```sh
# Preview from a custom releases repository
krateoctl install plan --version v1.0.0 --repository https://github.com/myorg/krateo-releases
```

```sh
# Compare the plan against the stored snapshot
krateoctl install plan --version v1.0.0 --diff-installed
```

```sh
# Show a step-by-step diff summary in a table
krateoctl install plan --diff-format table
```

## Apply Command

`krateoctl install apply` is the command that executes the computed workflow against the cluster.

### Usage

```sh
krateoctl install apply [FLAGS]
```

### Key Flags

- `--version` release tag to fetch from the releases repository
- `--repository` custom GitHub repository URL for release assets, default `https://github.com/krateoplatformops/releases`
- `--config` local configuration file, default `krateo.yaml`
- `--namespace` target namespace
- `--type` file variant to use, such as `nodeport`, `loadbalancer`, or `ingress`
- `--profile` optional profile name
- `--skip-validation` skip configuration validation
- `--debug` enable debug logging, or set `KRATEOCTL_DEBUG`

### What `apply` Does

1. Loads the configuration in local or remote mode.
2. Ensures the Installation CRD exists.
3. Applies any `pre-upgrade` manifests first.
4. Runs the main workflow steps.
5. Applies any `post-upgrade` manifests after the workflow completes.
6. Saves the resulting installation snapshot.

### Examples

```sh
# Apply from the local configuration
krateoctl install apply
```

```sh
# Apply a tagged release from the default releases repository
krateoctl install apply --version v1.0.0
```

```sh
# Apply a tagged release and use a custom repository
krateoctl install apply --version v1.0.0 --repository https://github.com/myorg/krateo-releases
```

```sh
# Apply with a type-specific layout
krateoctl install apply --config ./krateo.yaml --type ingress
```

## Upgrade Flow

For a normal upgrade, the recommended sequence is:

1. Run `krateoctl install plan --version <tag> --diff-installed` to compare the target release with the stored installation snapshot.
2. Review the diff and confirm the upgrade path looks correct.
3. Run `krateoctl install apply --version <tag>` to perform the upgrade.
4. Keep `--repository` pointed at `https://github.com/krateoplatformops/releases` unless you mirror the release assets elsewhere.
5. Use `--type` to match the target environment layout, such as `nodeport`, `loadbalancer`, or `ingress`.
6. Use `--diff-installed` when you want to compare the computed plan against the stored snapshot before applying.

## Notes

- Local mode uses files from disk and is the right fit when you are developing or testing a config change.
- Remote mode uses the releases repository and is the right fit when you are upgrading to an existing tagged release.
