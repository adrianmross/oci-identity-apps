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

By default, `oci-idm` reads the current `oci-context` for OCI CLI defaults:

- `oci-context export -f json` supplies the current context name, profile, and
  region.
- `oci-context paths -o json` supplies the OCI config file path.
- `oci-context auth service list -o json` supplies OAuth issuer and scope
  defaults when a matching token service exists, such as `obp`.

Explicit flags always win. Use `--oci-context=false` to disable this defaulting,
`--oci-context-bin` when testing another binary, and `--oci-context-service`
when a generic service should read issuer/scope from a named token service.

Inspect the resolved defaults before planning:

```bash
oci-idm defaults --service obp --format text
```

For OBP after selecting your OCI context and importing an `oci-context` token
service, the shortest planning command is:

```bash
oci-context use oabcs1

oci-idm plan \
  --service obp \
  --resource-app-id example-resource-app-id \
  --base-app-name example-obp_APPID \
  --include user,jwt-service \
  --role-preset obp-admin \
  --app-role-grants ADMIN=example-admin-role-id,REST_CLIENT=example-rest-client-role-id \
  --principal-mode auto \
  --principal-email-domain example.invalid \
  --format json > idm-plan.json
```

Discover the service-created resource app:

```bash
oci-idm discover \
  --query example-oabcs \
  --format text
```

Inspect the resource app once you know its id:

```bash
oci-idm discover \
  --app-id example-resource-app-id \
  --format text
```

Diagnose a generated client app against a known-good app:

```bash
oci-idm diagnose \
  --service obp \
  --resource-app-id example-resource-app-id \
  --candidate-app-id generated-client-app-id \
  --known-good-app-id known-working-client-app-id \
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
  --principal-mode auto \
  --principal-email-domain example.invalid \
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

Run a local readiness report over the current `oci-context` defaults and a
plan:

```bash
oci-idm doctor --plan idm-plan.json --format text
```

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

To let `oci-idm` materialize the handoff file and import it directly:

```bash
oci-idm handoff \
  --plan idm-plan.json \
  --import \
  --out ./idm-artifacts
```

Use `--dry-run` to preview the `oci-context auth service import` result.

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

## Service Principals

Some Oracle cloud services do not authorize a service-account token only by
checking the OAuth client app's granted roles. Instead, the target service reads
the access-token subject, resolves that subject as an OCI Identity Domains user,
and then checks the user's service application roles.

For client-credentials flows, OCI Identity Domains commonly sets the token
subject to the OAuth client id. For those services, the required pattern is:

- create the OAuth client app
- create or reuse a user whose `userName` exactly matches the OAuth client id
- grant service application roles to that user with `ADMINISTRATOR_TO_USER`
- mint the token with the OAuth client, then validate the target service call

`oci-idm` models this with `--principal-mode`:

- `auto`: default; enables `same-name-user` for known services such as OBP and
  otherwise resolves to `none`
- `none`: create app resources and app-role grants only
- `same-name-user`: also create a same-name principal user and user-role grants

OCI Identity Domains requires a primary email for users. By default, generated
principal users use `<client-id>@example.invalid`. Set
`--principal-email-domain` to an approved internal domain or edit the
materialized `*-principal-user.json` payload before applying it.

For a generic service that uses this subject-to-user authorization pattern:

```bash
oci-idm plan \
  --service generic \
  --issuer https://idcs-example.identity.oraclecloud.com \
  --scope https://service.example.com/.default \
  --resource-app-id service-resource-app-id \
  --include jwt-service \
  --principal-mode same-name-user \
  --principal-email-domain svc.example.com \
  --app-role-grants SERVICE_ADMIN=service-admin-role-id \
  --format json > idm-plan.json
```

For OBP/OBPCS REST proxy OAuth, Oracle documents that client-credentials tokens
use the client id as the token subject. OBP then looks up application roles for
that subject as a user. See Oracle's OBP OAuth authentication documentation:
<https://docs.oracle.com/en/cloud/paas/blockchain-cloud/restoci/UseOAuth.html>.
This is why OBP service-account plans use `same-name-user` in `auto` mode.

## Diagnosis

`oci-idm diagnose` emits safe OCI CLI commands for comparing a service/resource
app, a candidate OAuth client app, and an optional known-good client app. It
checks the surfaces that matter for CLI and automation handoff:

- service app metadata and app-role projections
- direct `Grant` resources for the candidate and known-good app
- `granted-app-roles` projected onto the candidate app
- same-name user lookup for services that resolve token subjects as users
- user `Grant` resources for the candidate and known-good principal user
- `AccountMgmtInfo` rows for the service/resource app

For OBP/OBPCS, a token can mint successfully and the candidate app can show the
expected app-side `granted-app-roles`, while OBPCS still returns
`OBP_ADMIN_FORBIDDEN` with `Failed to get application role for user`. In that
case, check whether a same-name user exists and has `ADMINISTRATOR_TO_USER`
grants for the OBP `ADMIN` and `REST_CLIENT` app roles.

## Apply Model

By default, `apply` remains a dry-run convenience wrapper around
materialization:

```bash
oci-idm apply --plan idm-plan.json --out ./idm-artifacts
```

For reviewed plans, `apply --execute --confirm` can run the OCI Identity
Domains changes directly. The executor is intentionally conservative:

- it searches for existing apps by name before creating them
- it searches for existing same-name principal users before creating them
- it resolves created or reused app/user ids before creating grants
- it searches for existing matching grants before creating new grants
- it fails closed when generated payloads still contain placeholders

```bash
oci-idm apply \
  --plan idm-plan.json \
  --out ./idm-apply \
  --execute \
  --confirm \
  --format text
```

JWT service plans still need real certificate material before direct execution.
Materialize first, replace placeholders such as
`<x509-base64-der-certificate>` and preset role ids, then rerun direct apply.

## Compatibility

The old command name continues to work:

```bash
oci-identity-apps version
oci-identity-apps plan --issuer ... --scope ...
```

New documentation and package names use `oci-idm`.
