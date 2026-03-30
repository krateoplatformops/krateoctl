# Secrets Spec

`krateoctl` expects installation secrets to be managed outside the normal install flow.

Recommended approach:

- store the secret material in Vault
- sync the Vault data into Kubernetes with your preferred secret management tool

Manual creation is also supported, but `krateoctl` does not document or rely on a bootstrap flag for it.

## Scope

The secrets in this spec should exist in the install namespace before you run `krateoctl install apply` or `krateoctl install migrate-full`.

The default install namespace is `krateo-system`, unless you override it.

## Required Secrets

### `jwt-sign-key`

- type: `Opaque`
- secret key: `JWT_SIGN_KEY`
- value: a random signing key

This secret is used by the platform JWT-related components.

### `krateo-db`

- type: `Opaque`
- secret keys:
  - `DB_USER`: `krateo-db-user`
  - `DB_PASS`: shared database password

This secret is used by the stack components that need database access.

### `krateo-db-user`

- type: `Opaque`
- secret keys:
  - `username`: `krateo-db-user`
  - `password`: shared database password

This secret is used by the CNPG/database side of the stack.

## Consistency Rules

- `krateo-db` and `krateo-db-user` must use the same password value.
- The `jwt-sign-key` should be generated once and kept stable across upgrades.
- All secrets must be created in the same namespace that `install apply` uses.

## Manual Creation Example

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: jwt-sign-key
  namespace: krateo-system
type: Opaque
stringData:
  JWT_SIGN_KEY: replace-with-a-random-base64-key
---
apiVersion: v1
kind: Secret
metadata:
  name: krateo-db
  namespace: krateo-system
type: Opaque
stringData:
  DB_USER: krateo-db-user
  DB_PASS: replace-with-a-shared-db-password
---
apiVersion: v1
kind: Secret
metadata:
  name: krateo-db-user
  namespace: krateo-system
type: Opaque
stringData:
  username: krateo-db-user
  password: replace-with-a-shared-db-password
```

## Notes

- If you use Vault, keep the Kubernetes Secret names and keys exactly as listed here.
- Production installs should manage these secrets independently of `krateoctl`.
