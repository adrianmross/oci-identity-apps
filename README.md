# oci-idm

`oci-idm` plans OCI Identity Domains apps, grants, and token-helper handoffs for
CLI and automation use.

The tool is intentionally review-first. It emits public OCI CLI payloads,
helper scripts, and `oci-context` handoff files. It does not store secrets or
create resources unless you explicitly run the generated OCI CLI commands.

`oci-identity-apps` remains a compatibility command for the original app-focused
name.

## Why This Exists

Oracle cloud services often create service-owned web applications in OCI
Identity Domains. Those generated apps are useful source context, but they are
not always the right clients for local CLI login, service accounts, JWT
assertions, or workload federation.

`oci-idm` plans companion identity resources for common flows:

- `user`: Authorization Code + Refresh Token for local CLI login helpers.
- `service`: Client Credentials for non-human automation.
- `jwt-service`: service-account Client Credentials with JWT client assertion.
- `jwt-user`: JWT Bearer assertion exchange for user or subject representation.
- `workload`: token exchange for trusted workload identity JWTs.

The `jwt` include expands to `jwt-service,jwt-user,workload`.

## Install

Homebrew:

```bash
brew install adrianmross/tap/oci-idm
```

npm:

```bash
npm install -g @adrianmross/oci-idm
```

GitHub Packages requires npm authentication, even for public packages:

```bash
export GITHUB_TOKEN="<github-token>"
npm config set @adrianmross:registry https://npm.pkg.github.com
npm config set //npm.pkg.github.com/:_authToken "$GITHUB_TOKEN"
npm install -g @adrianmross/oci-idm
```

The npm package builds the Go CLI during install, so Go 1.25.6 or newer must be
available on `PATH`.

## Basic Flow

Discover the service-created resource app:

```bash
oci-idm discover \
  --issuer https://idcs-example.identity.oraclecloud.com \
  --query example-oabcs \
  --format text
```

Inspect the resource app once you know its id:

```bash
oci-idm discover \
  --issuer https://idcs-example.identity.oraclecloud.com \
  --app-id example-resource-app-id \
  --format text
```

Plan companion apps and service role grants:

```bash
oci-idm plan \
  --service obp \
  --issuer https://idcs-example.identity.oraclecloud.com \
  --platform https://example-oabcs.blockchain.ocp.oraclecloud.com:7443/restproxy \
  --resource-app-id example-resource-app-id \
  --base-app-name example-obp_APPID \
  --include user,jwt-service \
  --role-preset obp-admin \
  --app-role-grants ADMIN=example-admin-role-id,REST_CLIENT=example-rest-client-role-id \
  --format json > idm-plan.json
```

Materialize reviewed payloads and helper scripts:

```bash
oci-idm materialize --plan idm-plan.json --out ./idm-artifacts
```

The output directory contains:

- `plan.json`
- OCI Identity Domains payload JSON files
- `apply.sh`
- `validate.sh`
- `cleanup.sh`
- `oci-context.handoff.json`
- `oci-context-token-services.yml`
- `oci-context-token-commands.sh`

## oci-context Handoff

Preview the token-service handoff without materializing files:

```bash
oci-idm handoff \
  --plan idm-plan.json \
  --target oci-context \
  --format yaml
```

Merge the generated `token_services` entries into a global or project
`oci-context` config, then validate with `oci-context auth token`.

For a planned OBP authorization-code app:

```bash
oci-context auth token \
  --service obp \
  --flow authorization-code \
  --redirect-url http://127.0.0.1:8180/callback \
  --format raw
```

For a planned OBP JWT service app:

```bash
export EXAMPLE_OBP_SERVICE_JWT_PRIVATE_KEY_FILE=./example-obp-service-jwt.key

oci-context auth token \
  --service obp-jwt-service \
  --flow jwt-client-credentials \
  --no-login \
  --format raw
```

For OCI Identity Domains JWT client assertion, the generated handoff sets
`jwt_audience: https://identity.oraclecloud.com/`. Current `oci-context`
versions also retry that audience automatically when an OCI Identity Domain
rejects the generic token-endpoint audience.

## Role Presets

The built-in OBP role presets are:

- `obp-admin`: `ADMIN` and `REST_CLIENT`
- `obp-rest-client`: `REST_CLIENT`
- `obp-user`: `USER`
- `obp-ca-user`: `CA_USER`

Use `--app-role-grants NAME=APP_ROLE_ID` to provide custom grants or override
preset placeholders.

## Apply Model

`apply` is still a dry-run convenience wrapper around materialization:

```bash
oci-idm apply --plan idm-plan.json --out ./idm-artifacts
```

It refuses `--execute`. Review payloads, replace placeholders such as
`<created-app-id>`, then run the generated scripts yourself.

## Compatibility

The old command name continues to work:

```bash
oci-identity-apps version
oci-identity-apps plan --issuer ... --scope ...
```

New documentation and package names use `oci-idm`.
