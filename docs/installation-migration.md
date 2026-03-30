# Installation Migration

`krateoctl` provides two commands for moving a legacy Krateo installation to the new `krateo.yaml` workflow:

- `krateoctl install migrate` for a manual migration
- `krateoctl install migrate-full` for an automated cutover

Both commands are designed for Krateo 2.7.0 installations managed by the installer controller.

If you are looking for the regular install or upgrade workflow, see [Install and Upgrade](install-upgrade.md).

## Which Command Should I Use?

Use `migrate` when you want to:

- generate `krateo.yaml`
- review the converted configuration before applying it
- run `install plan` and `install apply` yourself

Use `migrate-full` when you want `krateoctl` to:

- generate `krateo.yaml`
- apply the new configuration
- remove the legacy `KrateoPlatformOps` resource
- uninstall the old installer Helm releases

## Manual Migration

`krateoctl install migrate` reads a legacy `KrateoPlatformOps` resource from the cluster, converts it into the new configuration format, and writes the result to disk.

### Usage

```sh
krateoctl install migrate [FLAGS]
```

### Flags

- `--type` installation type to use for the generated defaults: `nodeport`, `loadbalancer`, or `ingress`
- `--namespace` namespace containing the legacy `KrateoPlatformOps` resource, default `krateo-system`
- `--name` legacy resource name, default `krateo`
- `--output` path for the generated file, default `krateo.yaml`
- `--force` overwrite the output file if it already exists
- `--debug` enable debug logging, or set `KRATEOCTL_DEBUG`

### What It Does

1. Connects to the cluster using the current kubeconfig.
2. Reads the legacy `KrateoPlatformOps` custom resource.
3. Converts its spec into a new `krateo.yaml` document.
4. Adds the default components definition for the selected installation type.
5. Writes the generated file to disk and stops.

### Typical Workflow

1. Run `krateoctl install migrate`.
2. Review the generated `krateo.yaml`.
3. Run `krateoctl install plan` to preview the new installation.
4. Run `krateoctl install apply` when you are ready to switch over.
5. Remove the old controller and legacy `KrateoPlatformOps` resource manually when the migration is complete.

### Inspect the Snapshot

After `install apply` or `migrate-full`, `krateoctl` stores the resolved installation snapshot as an `Installation` resource named `krateoctl` in the install namespace.

You can inspect it with:

```sh
kubectl get installation krateoctl -n krateo-system -o yaml
```

This is the easiest way to review what was actually persisted after the installation or migration finished.

### Examples

```sh
# Generate krateo.yaml from the default legacy resource
krateoctl install migrate
```

```sh
# Generate a file for a specific installation type
krateoctl install migrate --type ingress --output ./krateo.yaml
```

```sh
# Generate the file, then review and apply it manually
krateoctl install migrate --type nodeport
krateoctl install plan --config ./krateo.yaml --type nodeport
krateoctl install apply --config ./krateo.yaml --type nodeport
```

## Automated Migration

`krateoctl install migrate-full` performs the same conversion, then carries out the migration steps automatically.

### Usage

```sh
krateoctl install migrate-full [FLAGS]
```

### Flags

- `--type` installation type to use for the generated defaults: `nodeport`, `loadbalancer`, or `ingress`
- `--namespace` namespace containing the legacy `KrateoPlatformOps` resource, default `krateo-system`
- `--name` legacy resource name, default `krateo`
- `--output` optional path to save the generated `krateo.yaml`
- `--installer-namespace` namespace where the old installer is deployed, default the same as `--namespace`
- `--installer-release` Helm release name for the installer chart, default `installer`
- `--installer-crd-release` Helm release name for the installer CRD chart, default `installer-crd`
- `--force` overwrite the output file if it already exists
- `--debug` enable debug logging, or set `KRATEOCTL_DEBUG`

### What It Does

1. Reads the legacy `KrateoPlatformOps` resource from the cluster.
2. Converts it into a new `krateo.yaml` document.
3. Optionally writes the generated file to disk when `--output` is provided.
4. Scales the old installer down.
5. Applies the new configuration automatically.
6. Deletes the legacy `KrateoPlatformOps` resource.
7. Uninstalls the old installer Helm releases.

### Examples

```sh
# Run the full automatic migration
krateoctl install migrate-full --type nodeport
```

```sh
# Run the full migration and also save the generated file
krateoctl install migrate-full --type nodeport --output ./krateo.yaml
```

```sh
# Run the migration with custom Helm release names
krateoctl install migrate-full \
  --type ingress \
  --installer-release my-installer \
  --installer-crd-release my-installer-crd
```

## Notes

- `migrate` is the safer choice when you want to review the generated configuration first.
- `migrate-full` is the faster choice when you want `krateoctl` to handle the cutover end-to-end.
- If you are unsure which installation type to use, start with the one that matches your current environment: `nodeport`, `loadbalancer`, or `ingress`.
