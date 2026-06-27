package diagnose

import (
	"fmt"
	"net/url"
	"strings"
)

const SchemaVersion = "oci-idm.diagnose.v1"

type ServiceKind string

const (
	ServiceGeneric ServiceKind = "generic"
	ServiceOBP     ServiceKind = "obp"
)

type Options struct {
	Service        ServiceKind
	Issuer         string
	IDCSEndpoint   string
	ResourceAppID  string
	CandidateAppID string
	KnownGoodAppID string
	Profile        string
	OCIConfigPath  string
	Region         string
}

type Plan struct {
	SchemaVersion  string      `json:"schemaVersion"`
	Service        ServiceKind `json:"service"`
	IDCSEndpoint   string      `json:"idcsEndpoint"`
	ResourceAppID  string      `json:"resourceAppId"`
	CandidateAppID string      `json:"candidateAppId,omitempty"`
	KnownGoodAppID string      `json:"knownGoodAppId,omitempty"`
	Profile        string      `json:"profile,omitempty"`
	OCIConfigPath  string      `json:"ociConfigPath,omitempty"`
	Region         string      `json:"region,omitempty"`
	Commands       []Command   `json:"commands"`
	Checklist      []string    `json:"checklist"`
	Interpretation []string    `json:"interpretation"`
}

type Command struct {
	Key         string `json:"key"`
	Description string `json:"description"`
	Command     string `json:"command"`
}

func Build(options Options) (Plan, error) {
	service := options.Service
	if service == "" {
		service = ServiceGeneric
	}
	if service != ServiceGeneric && service != ServiceOBP {
		return Plan{}, fmt.Errorf("unsupported service %q", service)
	}
	endpoint, err := normalizeEndpoint(firstNonEmpty(options.IDCSEndpoint, options.Issuer))
	if err != nil {
		return Plan{}, err
	}
	resourceAppID := strings.TrimSpace(options.ResourceAppID)
	if resourceAppID == "" {
		return Plan{}, fmt.Errorf("--resource-app-id is required")
	}
	candidateAppID := strings.TrimSpace(options.CandidateAppID)
	knownGoodAppID := strings.TrimSpace(options.KnownGoodAppID)
	defaults := commandDefaults{
		Profile:       strings.TrimSpace(options.Profile),
		OCIConfigPath: strings.TrimSpace(options.OCIConfigPath),
		Region:        strings.TrimSpace(options.Region),
	}
	commands := []Command{
		appGetCommand("get-resource-app", "Inspect the service/resource app, including role projections and account-management references.", endpoint, defaults, resourceAppID),
		grantsForResourceCommand(endpoint, defaults, resourceAppID),
		accountMgmtForResourceCommand(endpoint, defaults, resourceAppID),
	}
	if candidateAppID != "" {
		commands = append(commands,
			appGetCommand("get-candidate-app", "Inspect the generated or candidate OAuth client app.", endpoint, defaults, candidateAppID),
			grantsForGranteeCommand("search-grants-for-candidate", "Find direct service app-role grants assigned to the candidate app.", endpoint, defaults, candidateAppID),
			userByNameCommand("search-same-name-user-for-candidate", "Find a same-name user for services that authorize client-credentials tokens by token sub/client_id.", endpoint, defaults, "<candidate-client-id-or-app-name>"),
			grantsForGranteeCommand("search-grants-for-candidate-user", "Find direct service app-role grants assigned to the same-name candidate user.", endpoint, defaults, "<candidate-user-id>"),
		)
	}
	if knownGoodAppID != "" {
		commands = append(commands,
			appGetCommand("get-known-good-app", "Inspect a known-working client app for comparison.", endpoint, defaults, knownGoodAppID),
			grantsForGranteeCommand("search-grants-for-known-good", "Find direct service app-role grants assigned to the known-working app.", endpoint, defaults, knownGoodAppID),
			userByNameCommand("search-same-name-user-for-known-good", "Find the same-name user for a known-working client app.", endpoint, defaults, "<known-good-client-id-or-app-name>"),
			grantsForGranteeCommand("search-grants-for-known-good-user", "Find direct service app-role grants assigned to the known-good same-name user.", endpoint, defaults, "<known-good-user-id>"),
		)
	}

	checklist := []string{
		"The candidate app can mint a token for the target service scope.",
		"The candidate has Grant resources whose app.value is the resource app id.",
		"The candidate app get output shows granted-app-roles for the required role ids.",
		"Services that authorize token subjects as users have a userName equal to token sub/client_id.",
		"The same-name user has direct app-role grants for the required role ids.",
		"The token claims use the expected issuer, audience, scope, sub, and client_id.",
	}
	interpretation := []string{
		"If token minting fails, fix OAuth grants, scopes, certificate/client-secret, or assertion audience first.",
		"If Grant resources are missing or granted-app-roles is empty, create or repair the Identity Domains app-role grants.",
		"If token minting succeeds and granted-app-roles is correct but the target service still rejects authorization, the missing mapping is service-side rather than an oci-context token issue.",
	}
	if service == ServiceOBP {
		interpretation = append(interpretation,
			"For OBP/OBPCS, HTTP 403 with OBP_ADMIN_FORBIDDEN and 'Failed to get application role for user' after successful token minting indicates OBPCS does not know that principal as an application-role subject.",
			"For OBP/OBPCS client-credentials tokens, create or reuse a user whose userName equals the OAuth client id, then grant OBP application roles to that user with ADMINISTRATOR_TO_USER.",
			"OCI Identity Domains AccountMgmtInfo is searchable/gettable but not creatable through the generated OCI CLI; do not assume resource app accounts can be patched as the OBPCS principal registry.",
		)
	}

	return Plan{
		SchemaVersion:  SchemaVersion,
		Service:        service,
		IDCSEndpoint:   endpoint,
		ResourceAppID:  resourceAppID,
		CandidateAppID: candidateAppID,
		KnownGoodAppID: knownGoodAppID,
		Profile:        defaults.Profile,
		OCIConfigPath:  defaults.OCIConfigPath,
		Region:         defaults.Region,
		Commands:       commands,
		Checklist:      checklist,
		Interpretation: interpretation,
	}, nil
}

func appGetCommand(key, description, endpoint string, defaults commandDefaults, appID string) Command {
	return Command{
		Key:         key,
		Description: description,
		Command: identityDomainsCommand(endpoint, defaults, "app", "get",
			"--app-id "+shellQuote(appID),
			"--attributes 'id,name,displayName,active,clientType,isOAuthClient,isOAuthResource,allowedGrants,allowedScopes,redirectUris,certificates,accounts,grants,appRoles,grantedAppRoles,userRoles,basedOnTemplate,serviceTypeURN,audience,trustScope,tags'"),
	}
}

func grantsForResourceCommand(endpoint string, defaults commandDefaults, resourceAppID string) Command {
	return Command{
		Key:         "search-grants-for-resource-app",
		Description: "Find all direct grantees for the service/resource app roles.",
		Command: identityDomainsCommand(endpoint, defaults, "grants", "search",
			"--schemas '[\"urn:ietf:params:scim:api:messages:2.0:SearchRequest\"]'",
			"--filter "+shellQuote("app.value eq \""+resourceAppID+"\""),
			"--attributes '[\"id\",\"app\",\"entitlement\",\"grantee\",\"grantMechanism\",\"isFulfilled\"]'",
			"--count 1000"),
	}
}

func grantsForGranteeCommand(key, description, endpoint string, defaults commandDefaults, appID string) Command {
	return Command{
		Key:         key,
		Description: description,
		Command: identityDomainsCommand(endpoint, defaults, "grants", "search",
			"--schemas '[\"urn:ietf:params:scim:api:messages:2.0:SearchRequest\"]'",
			"--filter "+shellQuote("grantee.value eq \""+appID+"\""),
			"--attributes '[\"id\",\"app\",\"entitlement\",\"grantee\",\"grantMechanism\",\"isFulfilled\"]'",
			"--count 1000"),
	}
}

func userByNameCommand(key, description, endpoint string, defaults commandDefaults, userName string) Command {
	return Command{
		Key:         key,
		Description: description,
		Command: identityDomainsCommand(endpoint, defaults, "users", "search",
			"--schemas '[\"urn:ietf:params:scim:api:messages:2.0:SearchRequest\"]'",
			"--filter "+shellQuote("userName eq \""+userName+"\""),
			"--attributes '[\"id\",\"userName\",\"displayName\",\"active\",\"roles\"]'",
			"--count 10"),
	}
}

func accountMgmtForResourceCommand(endpoint string, defaults commandDefaults, resourceAppID string) Command {
	return Command{
		Key:         "search-account-mgmt-for-resource-app",
		Description: "Inspect AccountMgmtInfo rows for the resource app; these are service-managed references, not app-role grants.",
		Command: identityDomainsCommand(endpoint, defaults, "account-mgmt-infos", "search",
			"--schemas '[\"urn:ietf:params:scim:api:messages:2.0:SearchRequest\"]'",
			"--filter "+shellQuote("app.value eq \""+resourceAppID+"\""),
			"--attributes '[\"id\",\"uid\",\"name\",\"active\",\"accountType\",\"isAccount\",\"app\",\"owner\",\"resourceType\",\"objectClass\",\"syncSituation\",\"syncResponse\"]'",
			"--count 1000"),
	}
}

func normalizeEndpoint(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", fmt.Errorf("issuer or idcs endpoint is required")
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

type commandDefaults struct {
	Profile       string
	OCIConfigPath string
	Region        string
}

func identityDomainsCommand(endpoint string, defaults commandDefaults, resource string, action string, args ...string) string {
	parts := []string{
		"oci identity-domains",
		resource,
		action,
		"--endpoint " + shellQuote(endpoint),
	}
	if defaults.Profile != "" {
		parts = append(parts, "--profile "+shellQuote(defaults.Profile))
	}
	if defaults.OCIConfigPath != "" {
		parts = append(parts, "--config-file "+shellQuote(defaults.OCIConfigPath))
	}
	if defaults.Region != "" {
		parts = append(parts, "--region "+shellQuote(defaults.Region))
	}
	parts = append(parts, args...)
	return strings.Join(parts, " ")
}
