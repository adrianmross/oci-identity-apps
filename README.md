# oci-identity-apps

`oci-identity-apps` plans OCI Identity Domains OAuth applications for CLI and
automation use.

The first version is deliberately dry-run only. It emits public, reviewable OCI
CLI `identity-domains app create --from-json` payloads and the matching create
commands. It does not create resources, store secrets, or read private
configuration.

## Why This Exists

Oracle cloud services often create service-owned web applications in OCI
Identity Domains. Those generated apps can be useful source context, but they
are not always the right OAuth clients for CLI handoff or automation.

This tool plans companion OAuth clients for common flows:

- `user`: Authorization Code + Refresh Token for local CLI login helpers.
- `service`: Client Credentials for non-human automation.
- `jwt-service`: service-account Client Credentials with JWT client assertion.
- `jwt-user`: JWT Bearer assertion exchange for user or subject representation.
- `workload`: token exchange for trusted workload identity JWTs.

The `jwt` include is a convenience alias that expands to
`jwt-service,jwt-user,workload`.

The default service mode is generic. The `obp` preset adds Oracle Blockchain
Platform notes, including the common CloudGate callback limitation and OBP
post-create role checks.

The tool can also plan the service-app wiring that OCI Identity Domains needs
after an OAuth client exists:

- scope ownership through `idOfDefiningApp`
- OAuth client certificate registration for JWT client assertion
- direct app-role grants from a service/resource app to a companion OAuth app

## Install

Homebrew:

```bash
brew install adrianmross/tap/oci-identity-apps
```

npmjs:

```bash
npm install -g @adrianmross/oci-identity-apps
```

GitHub Packages requires npm authentication, even for public packages. Use a
GitHub token with `read:packages` for install, or `write:packages` for publish:

```bash
export GITHUB_TOKEN="<github-token>"
npm config set @adrianmross:registry https://npm.pkg.github.com
npm config set //npm.pkg.github.com/:_authToken "$GITHUB_TOKEN"
npm install -g @adrianmross/oci-identity-apps
```

The npm package builds the Go CLI during install, so Go 1.25.6 or newer must be
available on `PATH`.

From source:

```bash
go install github.com/adrianmross/oci-identity-apps/cmd/oci-identity-apps@latest
```

## Generic Service Plan

```bash
oci-identity-apps plan \
  --issuer https://idcs-example.identity.oraclecloud.com \
  --scope https://service.example.com/.default \
  --app-prefix example-service \
  --include user,service,jwt \
  --format json > identity-app-plan.json
```

Use a narrower include list when you only need one identity style:

```bash
oci-identity-apps plan \
  --issuer https://idcs-example.identity.oraclecloud.com \
  --scope https://service.example.com/.default \
  --app-prefix example-service \
  --include jwt-service \
  --format json > service-account-jwt-plan.json
```

## Oracle Blockchain Platform Plan

```bash
oci-identity-apps plan \
  --service obp \
  --issuer https://idcs-example.identity.oraclecloud.com \
  --platform https://example-oabcs.blockchain.ocp.oraclecloud.com:7443/restproxy \
  --resource-app-id example-resource-app-id \
  --base-app-name example-obp_APPID \
  --redirect-url http://127.0.0.1:8180/callback \
  --format json > obp-identity-app-plan.json
```

For the OBP preset, `--scope` defaults to `--platform`.

To plan a JWT service-account app that can be granted OBP roles, pass the
resource app id and the target app-role ids from the service-created OBP app:

```bash
oci-identity-apps plan \
  --service obp \
  --issuer https://idcs-example.identity.oraclecloud.com \
  --platform https://example-oabcs.blockchain.ocp.oraclecloud.com:7443/restproxy \
  --resource-app-id example-resource-app-id \
  --base-app-name example-obp_APPID \
  --include jwt-service \
  --app-role-grants ADMIN=example-admin-role-id,REST_CLIENT=example-rest-client-role-id \
  --format json > obp-jwt-service-plan.json
```

That plan includes:

- an `ociPreCreate` certificate registration payload for
  `oci identity-domains o-auth-client-certificate create`
- an app create payload whose allowed scope includes `idOfDefiningApp`
- `ociPostCreate` grant payloads for
  `oci identity-domains grant create`

After the app is created, replace `<created-app-id>` in the grant payloads with
the `data.id` returned by `oci identity-domains app create`.

For OCI Identity Domains JWT client assertion, some domains require the local
client assertion `aud` claim to be `https://identity.oraclecloud.com/` rather
than the token endpoint URL. With `oci-context`, use:

```bash
oci-context auth token \
  --service obp \
  --flow jwt-client-credentials \
  --token-endpoint https://idcs-example.identity.oraclecloud.com/oauth2/v1/token \
  --client-id example-obp-service-jwt \
  --scope https://example-oabcs.blockchain.ocp.oraclecloud.com:7443/restproxy \
  --private-key-file ./example-obp-service-jwt.key \
  --key-id example-obp-service-jwt-cert \
  --jwt-audience https://identity.oraclecloud.com/ \
  --no-login \
  --format raw
```

## Output

Each planned app includes:

- app name and display name
- OAuth client type
- allowed grants
- allowed scopes
- optional certificate registration payloads
- redirect URIs when relevant
- OCI CLI create payload filename
- OCI CLI create command
- optional app-role grant payloads
- post-create checks

Review and save each `ociCreatePayload` as its listed JSON file before running
the emitted `oci identity-domains app create` command.

## Public Safety

This repository should remain safe for public use:

- no real tenancy OCIDs
- no real identity-domain names
- no real users, groups, emails, tokens, or secrets
- no private service hosts or internal organization names
- examples use placeholders or reserved example domains

If a workflow needs live environment details, pass them at runtime instead of
committing them.
