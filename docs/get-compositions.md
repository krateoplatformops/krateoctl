# Get Compositions

`krateoctl get compositions` lists or fetches composition resources with automatic version discovery across the CRDs in the `composition.krateo.io` group.

## Table Of Contents

- [What It Supports](#what-it-supports)
- [Flags](#flags)
- [How Lookup Works](#how-lookup-works)
- [Defaults](#defaults)
- [Examples](#examples)
- [Output Notes](#output-notes)

## Usage

```sh
krateoctl get compositions [NAME | TYPE/NAME] [FLAGS]
```

## What It Supports

- list all discovered compositions
- fetch a single composition by name
- fetch a composition explicitly by CRD plural and name with `TYPE/NAME`
- filter results by namespace, label selector, or field selector
- render output as table, YAML, JSON, or name-only

## Flags

- `-n`, `--namespace` namespace to query
- `-A` list across all namespaces
- `-o` output format: `table`, `yaml`, `json`, or `name`
- `-l` label selector
- `--field-selector` field selector for filtering

## How Lookup Works

`krateoctl` discovers every CRD in the `composition.krateo.io` group that is marked with the `compositions` category, then uses the best served version for each resource automatically.

When you request a single resource:

- `NAME` looks up a composition by name across all discovered composition kinds
- `TYPE/NAME` lets you choose the CRD plural explicitly, for example `githubscaffoldinglifecycles/my-composition`

If the same name exists in more than one composition kind, `krateoctl` returns an ambiguity error and asks you to use `TYPE/NAME`.

## Defaults

- If you do not pass `--namespace`, `krateoctl` uses your current kubeconfig namespace.
- If you do not pass `-o`, the command prints a table.
- If you use `-A`, you cannot also request a single named resource.

## Examples

```sh
# List compositions in the current namespace
krateoctl get compositions
```

```sh
# List compositions in a specific namespace
krateoctl get compositions -n krateo-system
```

```sh
# List compositions across every namespace
krateoctl get compositions -A
```

```sh
# Fetch a composition by name
krateoctl get compositions my-composition
```

```sh
# Fetch a composition by explicit CRD plural and name
krateoctl get compositions githubscaffoldinglifecycles/my-composition
```

```sh
# Render the result as YAML
krateoctl get compositions my-composition -o yaml
```

## Output Notes

- `table` output shows namespace, name, version, and kind.
- `name` output prints `kind/name`.
- `yaml` and `json` output omit Kubernetes managed fields for readability.
