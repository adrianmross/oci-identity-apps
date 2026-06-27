package planner

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

const (
	SchemaVersion                          = "oci-identity-apps.plan.v1"
	IDCSAppSchema                          = "urn:ietf:params:scim:schemas:oracle:idcs:App"
	DefaultWebAppTemplateID                = "CustomWebAppTemplateId"
	DefaultPublicAppTemplateID             = "CustomBrowserMobileTemplateId"
	DefaultCLIRedirectURL                  = "http://127.0.0.1:8180/callback"
	AuthorizationCodeGrant                 = "authorization_code"
	RefreshTokenGrant                      = "refresh_token"
	ClientCredentialsGrant                 = "client_credentials"
	JWTBearerGrant                         = "urn:ietf:params:oauth:grant-type:jwt-bearer"
	ServiceGeneric             ServiceKind = "generic"
	ServiceOBP                 ServiceKind = "obp"
	AppUser                    AppKind     = "user"
	AppService                 AppKind     = "service"
	AppJWT                     AppKind     = "jwt"
	ClientPublic               ClientType  = "public"
	ClientConfidential         ClientType  = "confidential"
)

type ServiceKind string
type AppKind string
type ClientType string

type Options struct {
	Service            ServiceKind
	Platform           string
	Issuer             string
	Scope              string
	IDCSEndpoint       string
	BaseAppName        string
	BaseAppDisplayName string
	AppPrefix          string
	RedirectURL        string
	Include            []AppKind
	UserClientType     ClientType
	TemplateID         string
	UserTemplateID     string
	ServiceTemplateID  string
	JWTTemplateID      string
	AccessTokenExpiry  int
	RefreshTokenExpiry int
}

type Plan struct {
	SchemaVersion       string              `json:"schemaVersion"`
	Target              Target              `json:"target"`
	BaseCloudServiceApp BaseCloudServiceApp `json:"baseCloudServiceApp,omitempty"`
	Apps                []AppPlan           `json:"apps"`
	Apply               ApplyPlan           `json:"apply"`
	Validation          ValidationPlan      `json:"validation"`
	SourceReferences    []string            `json:"sourceReferences"`
}

type Target struct {
	Service            ServiceKind        `json:"service"`
	Platform           string             `json:"platform,omitempty"`
	Issuer             string             `json:"issuer"`
	Scope              string             `json:"scope"`
	IDCSEndpoint       string             `json:"idcsEndpoint"`
	IDCSAdminURL       string             `json:"idcsAdminUrl"`
	IDCSEndpointSource string             `json:"idcsEndpointSource"`
	RedirectURL        string             `json:"redirectUrl"`
	UserClientType     ClientType         `json:"userClientType"`
	TemplateIDs        map[AppKind]string `json:"templateIds"`
}

type BaseCloudServiceApp struct {
	Name             string   `json:"name,omitempty"`
	DisplayName      string   `json:"displayName,omitempty"`
	ExpectedBehavior []string `json:"expectedBehavior,omitempty"`
}

type AppPlan struct {
	Key                  AppKind        `json:"key"`
	Name                 string         `json:"name"`
	DisplayName          string         `json:"displayName"`
	Purpose              string         `json:"purpose"`
	ClientType           ClientType     `json:"clientType"`
	AllowedGrants        []string       `json:"allowedGrants"`
	RedirectURIs         []string       `json:"redirectUris"`
	AllowedScopes        []string       `json:"allowedScopes"`
	OCICreatePayloadFile string         `json:"ociCreatePayloadFile"`
	OCICreateCommand     string         `json:"ociCreateCommand"`
	OCICreatePayload     AppCreateInput `json:"ociCreatePayload"`
	RequiredPostCreate   []string       `json:"requiredPostCreate"`
	Usage                []string       `json:"usage"`
}

type ApplyPlan struct {
	Commands []string `json:"commands"`
}

type ValidationPlan struct {
	BeforeApply []string `json:"beforeApply"`
	AfterApply  []string `json:"afterApply"`
}

type AppCreateInput struct {
	Schemas            []string       `json:"schemas"`
	DisplayName        string         `json:"displayName"`
	Name               string         `json:"name"`
	Active             bool           `json:"active"`
	BasedOnTemplate    TemplateRef    `json:"basedOnTemplate"`
	IsOAuthClient      bool           `json:"isOAuthClient"`
	ClientType         ClientType     `json:"clientType"`
	AllowedGrants      []string       `json:"allowedGrants"`
	AllowedScopes      []AllowedScope `json:"allowedScopes"`
	RedirectURIs       []string       `json:"redirectUris,omitempty"`
	AllURLSchemes      *bool          `json:"allUrlSchemesAllowed,omitempty"`
	BypassConsent      *bool          `json:"bypassConsent,omitempty"`
	AllowOffline       *bool          `json:"allowOffline,omitempty"`
	AccessTokenExpiry  *int           `json:"accessTokenExpiry,omitempty"`
	RefreshTokenExpiry *int           `json:"refreshTokenExpiry,omitempty"`
}

type TemplateRef struct {
	Value string `json:"value"`
}

type AllowedScope struct {
	FQS string `json:"fqs"`
}

func Build(options Options) (Plan, error) {
	service := options.Service
	if service == "" {
		service = ServiceGeneric
	}
	if service != ServiceGeneric && service != ServiceOBP {
		return Plan{}, fmt.Errorf("unsupported service %q", service)
	}

	issuer, err := required(options.Issuer, "issuer")
	if err != nil {
		return Plan{}, err
	}
	scope := strings.TrimSpace(options.Scope)
	if scope == "" {
		if service == ServiceOBP {
			scope = strings.TrimSpace(options.Platform)
		}
		if scope == "" {
			return Plan{}, errors.New("scope is required unless --service obp and --platform is set")
		}
	}

	redirectURL := firstNonEmpty(options.RedirectURL, DefaultCLIRedirectURL)
	userClientType := options.UserClientType
	if userClientType == "" {
		userClientType = ClientPublic
	}
	if userClientType != ClientPublic && userClientType != ClientConfidential {
		return Plan{}, fmt.Errorf("unsupported user client type %q", userClientType)
	}

	idcsEndpoint, err := normalizeEndpoint(firstNonEmpty(options.IDCSEndpoint, issuer))
	if err != nil {
		return Plan{}, err
	}
	templateIDs, err := resolveTemplateIDs(options, userClientType)
	if err != nil {
		return Plan{}, err
	}
	includes := uniqueIncludes(options.Include)
	prefix := normalizePrefix(firstNonEmpty(
		options.AppPrefix,
		options.BaseAppName,
		options.BaseAppDisplayName,
		inferPrefix(options.Platform, scope),
	))

	ctx := buildContext{
		service:            service,
		prefix:             prefix,
		issuer:             issuer,
		scope:              scope,
		platform:           strings.TrimSpace(options.Platform),
		redirectURL:        redirectURL,
		idcsEndpoint:       idcsEndpoint,
		templateIDs:        templateIDs,
		userClientType:     userClientType,
		accessTokenExpiry:  positivePtr(options.AccessTokenExpiry),
		refreshTokenExpiry: positivePtr(options.RefreshTokenExpiry),
	}

	apps := make([]AppPlan, 0, len(includes))
	for _, include := range includes {
		apps = append(apps, buildApp(include, ctx))
	}

	commands := make([]string, 0, len(apps))
	for _, app := range apps {
		commands = append(commands, app.OCICreateCommand)
	}

	return Plan{
		SchemaVersion: SchemaVersion,
		Target: Target{
			Service:            service,
			Platform:           strings.TrimSpace(options.Platform),
			Issuer:             issuer,
			Scope:              scope,
			IDCSEndpoint:       idcsEndpoint,
			IDCSAdminURL:       idcsEndpoint + "/admin/v1/",
			IDCSEndpointSource: endpointSource(options.IDCSEndpoint),
			RedirectURL:        redirectURL,
			UserClientType:     userClientType,
			TemplateIDs:        templateIDs,
		},
		BaseCloudServiceApp: baseCloudServiceApp(service, options),
		Apps:                apps,
		Apply:               ApplyPlan{Commands: commands},
		Validation:          validationPlan(service),
		SourceReferences: []string{
			"OCI CLI: oci identity-domains app create",
			"Oracle Identity Domains: OAuth client applications and allowed grants",
			"Oracle Identity Domains: SCIM App resource",
		},
	}, nil
}

func ParseIncludes(value string) ([]AppKind, error) {
	if strings.TrimSpace(value) == "" {
		return []AppKind{AppUser, AppService, AppJWT}, nil
	}
	parts := strings.Split(value, ",")
	includes := make([]AppKind, 0, len(parts))
	for _, part := range parts {
		key := AppKind(strings.TrimSpace(part))
		if key == "" {
			continue
		}
		switch key {
		case AppUser, AppService, AppJWT:
			includes = append(includes, key)
		default:
			return nil, fmt.Errorf("unsupported app include %q", key)
		}
	}
	return uniqueIncludes(includes), nil
}

func ParseClientType(value string) (ClientType, error) {
	switch strings.TrimSpace(value) {
	case "":
		return "", nil
	case string(ClientPublic):
		return ClientPublic, nil
	case string(ClientConfidential):
		return ClientConfidential, nil
	default:
		return "", fmt.Errorf("unsupported client type %q", value)
	}
}

type buildContext struct {
	service            ServiceKind
	prefix             string
	issuer             string
	scope              string
	platform           string
	redirectURL        string
	idcsEndpoint       string
	templateIDs        map[AppKind]string
	userClientType     ClientType
	accessTokenExpiry  *int
	refreshTokenExpiry *int
}

func buildApp(kind AppKind, ctx buildContext) AppPlan {
	switch kind {
	case AppUser:
		return appPlan(appInput{
			kind:               AppUser,
			name:               ctx.prefix + "-cli-user",
			displayName:        title(ctx.prefix) + " CLI User Auth",
			purpose:            "MFA-capable local human authorization-code flow for a CLI token helper.",
			clientType:         ctx.userClientType,
			grants:             []string{AuthorizationCodeGrant, RefreshTokenGrant},
			redirectURIs:       []string{ctx.redirectURL},
			scope:              ctx.scope,
			templateID:         ctx.templateIDs[AppUser],
			idcsEndpoint:       ctx.idcsEndpoint,
			allowOffline:       true,
			bypassConsent:      true,
			accessExpiry:       ctx.accessTokenExpiry,
			refreshExpiry:      ctx.refreshTokenExpiry,
			requiredPostCreate: userPostCreate(ctx),
			usage: []string{
				"Configure your token helper with the issuer, generated client id, scope, and redirect URL.",
				"Run the helper's authorization-code flow once to cache a refresh token.",
			},
		})
	case AppService:
		return appPlan(appInput{
			kind:          AppService,
			name:          ctx.prefix + "-service",
			displayName:   title(ctx.prefix) + " Service Client",
			purpose:       "Confidential OAuth client for non-human client-credentials automation.",
			clientType:    ClientConfidential,
			grants:        []string{ClientCredentialsGrant},
			scope:         ctx.scope,
			templateID:    ctx.templateIDs[AppService],
			idcsEndpoint:  ctx.idcsEndpoint,
			bypassConsent: true,
			accessExpiry:  ctx.accessTokenExpiry,
			requiredPostCreate: []string{
				"Store the generated client id and secret in a secret manager or CI secret store.",
				"Grant only the target service roles required by this automation identity.",
			},
			usage: []string{
				"Use grant_type=client_credentials with the generated client id, client secret, and requested scope.",
			},
		})
	default:
		return appPlan(appInput{
			kind:          AppJWT,
			name:          ctx.prefix + "-jwt-assertion",
			displayName:   title(ctx.prefix) + " JWT Assertion",
			purpose:       "Confidential OAuth client for trusted JWT bearer assertion exchange.",
			clientType:    ClientConfidential,
			grants:        []string{JWTBearerGrant},
			scope:         ctx.scope,
			templateID:    ctx.templateIDs[AppJWT],
			idcsEndpoint:  ctx.idcsEndpoint,
			bypassConsent: true,
			accessExpiry:  ctx.accessTokenExpiry,
			requiredPostCreate: []string{
				"Register the assertion signing certificate or trust material required by the identity domain.",
				"Map the asserted subject to an identity that has only the target service roles it needs.",
			},
			usage: []string{
				"Use a token helper that exchanges the signed assertion and emits the final access token.",
			},
		})
	}
}

type appInput struct {
	kind               AppKind
	name               string
	displayName        string
	purpose            string
	clientType         ClientType
	grants             []string
	redirectURIs       []string
	scope              string
	templateID         string
	idcsEndpoint       string
	allowOffline       bool
	bypassConsent      bool
	accessExpiry       *int
	refreshExpiry      *int
	requiredPostCreate []string
	usage              []string
}

func appPlan(input appInput) AppPlan {
	payloadFile := input.name + ".json"
	payload := AppCreateInput{
		Schemas:         []string{IDCSAppSchema},
		DisplayName:     input.displayName,
		Name:            input.name,
		Active:          true,
		BasedOnTemplate: TemplateRef{Value: input.templateID},
		IsOAuthClient:   true,
		ClientType:      input.clientType,
		AllowedGrants:   append([]string{}, input.grants...),
		AllowedScopes:   []AllowedScope{{FQS: input.scope}},
		RedirectURIs:    append([]string{}, input.redirectURIs...),
	}
	if hasNonHTTPSRedirect(input.redirectURIs) {
		payload.AllURLSchemes = boolPtr(true)
	}
	if input.bypassConsent {
		payload.BypassConsent = boolPtr(true)
	}
	if input.allowOffline {
		payload.AllowOffline = boolPtr(true)
	}
	payload.AccessTokenExpiry = input.accessExpiry
	payload.RefreshTokenExpiry = input.refreshExpiry

	return AppPlan{
		Key:                  input.kind,
		Name:                 input.name,
		DisplayName:          input.displayName,
		Purpose:              input.purpose,
		ClientType:           input.clientType,
		AllowedGrants:        append([]string{}, input.grants...),
		RedirectURIs:         append([]string{}, input.redirectURIs...),
		AllowedScopes:        []string{input.scope},
		OCICreatePayloadFile: payloadFile,
		OCICreateCommand:     "oci identity-domains app create --endpoint " + shellQuote(input.idcsEndpoint) + " --from-json file://" + payloadFile,
		OCICreatePayload:     payload,
		RequiredPostCreate:   append([]string{}, input.requiredPostCreate...),
		Usage:                append([]string{}, input.usage...),
	}
}

func userPostCreate(ctx buildContext) []string {
	steps := []string{
		"Copy the generated client id into your token helper configuration.",
	}
	if ctx.userClientType == ClientConfidential {
		steps = append(steps, "Store the generated client secret outside source control.")
	} else {
		steps = append(steps, "No client secret is required when using PKCE with a public client.")
	}
	steps = append(steps, "Assign the app to users or groups if the identity domain enforces application grants.")
	return steps
}

func baseCloudServiceApp(service ServiceKind, options Options) BaseCloudServiceApp {
	if service != ServiceOBP {
		return BaseCloudServiceApp{}
	}
	return BaseCloudServiceApp{
		Name:        strings.TrimSpace(options.BaseAppName),
		DisplayName: strings.TrimSpace(options.BaseAppDisplayName),
		ExpectedBehavior: []string{
			"Treat the generated Oracle Blockchain Platform CloudGate app as service-owned and non-mutated.",
			"Do not use a CloudGate callback for CLI authorization-code handoff because CloudGate consumes the code.",
			"Create companion OAuth clients for local user auth, service automation, and JWT assertion flows.",
		},
	}
}

func validationPlan(service ServiceKind) ValidationPlan {
	before := []string{
		"Confirm the OCI Identity Domains endpoint is the identity-domain base URL.",
		"Confirm the requested scope belongs to the target service resource.",
		"Review every payload before applying it with OCI CLI.",
	}
	after := []string{
		"Grant the app to the intended users, groups, or automation identities.",
		"Store generated secrets only in an approved secret manager.",
		"Run a read-only target-service validation before granting write or admin roles.",
	}
	if service == ServiceOBP {
		after = append(after,
			"Grant Oracle Blockchain Platform administrator access for deploy or console-admin APIs.",
			"Grant REST_USER and REST proxy enrollment for REST proxy chaincode calls.",
		)
	}
	return ValidationPlan{BeforeApply: before, AfterApply: after}
}

func resolveTemplateIDs(options Options, userClientType ClientType) (map[AppKind]string, error) {
	defaultUser := DefaultWebAppTemplateID
	if userClientType == ClientPublic {
		defaultUser = DefaultPublicAppTemplateID
	}
	values := map[AppKind]string{
		AppUser:    firstNonEmpty(options.UserTemplateID, options.TemplateID, defaultUser),
		AppService: firstNonEmpty(options.ServiceTemplateID, options.TemplateID, DefaultWebAppTemplateID),
		AppJWT:     firstNonEmpty(options.JWTTemplateID, options.TemplateID, DefaultWebAppTemplateID),
	}
	for key, value := range values {
		if strings.TrimSpace(value) == "" {
			return nil, fmt.Errorf("template id for %s is required", key)
		}
		values[key] = strings.TrimSpace(value)
	}
	return values, nil
}

func uniqueIncludes(includes []AppKind) []AppKind {
	if len(includes) == 0 {
		includes = []AppKind{AppUser, AppService, AppJWT}
	}
	seen := map[AppKind]bool{}
	out := make([]AppKind, 0, len(includes))
	for _, include := range includes {
		if seen[include] {
			continue
		}
		seen[include] = true
		out = append(out, include)
	}
	return out
}

func normalizeEndpoint(value string) (string, error) {
	trimmed, err := required(value, "idcs endpoint")
	if err != nil {
		return "", err
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", err
	}
	if parsed.Scheme != "https" || parsed.Host == "" {
		return "", fmt.Errorf("identity domain endpoint must be an https URL")
	}
	return parsed.Scheme + "://" + parsed.Host, nil
}

func endpointSource(value string) string {
	if strings.TrimSpace(value) == "" {
		return "derived_from_issuer"
	}
	return "option"
}

func inferPrefix(platform string, scope string) string {
	for _, candidate := range []string{platform, scope} {
		if parsed, err := url.Parse(candidate); err == nil && parsed.Hostname() != "" {
			return strings.Split(parsed.Hostname(), ".")[0]
		}
	}
	return "oci-service"
}

var unsafeName = regexp.MustCompile(`[^A-Za-z0-9-]+`)

func normalizePrefix(value string) string {
	normalized := strings.TrimSuffix(strings.TrimSpace(value), "_APPID")
	normalized = unsafeName.ReplaceAllString(normalized, "-")
	normalized = strings.Trim(normalized, "-")
	normalized = strings.ToLower(normalized)
	if normalized == "" {
		return "oci-service"
	}
	return normalized
}

func title(value string) string {
	parts := strings.Split(value, "-")
	for i, part := range parts {
		if part == "" {
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + part[1:]
	}
	return strings.Join(parts, " ")
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func required(value string, name string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", fmt.Errorf("%s is required", name)
	}
	return trimmed, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func boolPtr(value bool) *bool {
	return &value
}

func hasNonHTTPSRedirect(redirectURIs []string) bool {
	for _, redirectURI := range redirectURIs {
		if strings.TrimSpace(redirectURI) != "" && !strings.HasPrefix(strings.ToLower(strings.TrimSpace(redirectURI)), "https://") {
			return true
		}
	}
	return false
}

func positivePtr(value int) *int {
	if value <= 0 {
		return nil
	}
	return &value
}
