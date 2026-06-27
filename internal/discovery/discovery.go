package discovery

import (
	"fmt"
	"net/url"
	"strings"
)

const SchemaVersion = "oci-idm.discovery.v1"

type Options struct {
	Issuer        string
	IDCSEndpoint  string
	Query         string
	AppID         string
	Profile       string
	OCIConfigPath string
	Region        string
}

type Plan struct {
	SchemaVersion string    `json:"schemaVersion"`
	IDCSEndpoint  string    `json:"idcsEndpoint"`
	Query         string    `json:"query,omitempty"`
	AppID         string    `json:"appId,omitempty"`
	Profile       string    `json:"profile,omitempty"`
	OCIConfigPath string    `json:"ociConfigPath,omitempty"`
	Region        string    `json:"region,omitempty"`
	Commands      []Command `json:"commands"`
	NextSteps     []string  `json:"nextSteps"`
}

type Command struct {
	Key         string `json:"key"`
	Description string `json:"description"`
	Command     string `json:"command"`
}

func Build(options Options) (Plan, error) {
	endpoint, err := normalizeEndpoint(firstNonEmpty(options.IDCSEndpoint, options.Issuer))
	if err != nil {
		return Plan{}, err
	}
	defaults := commandDefaults{
		Profile:       strings.TrimSpace(options.Profile),
		OCIConfigPath: strings.TrimSpace(options.OCIConfigPath),
		Region:        strings.TrimSpace(options.Region),
	}
	query := strings.TrimSpace(options.Query)
	appID := strings.TrimSpace(options.AppID)
	commands := []Command{}
	if query != "" {
		filter := fmt.Sprintf("displayName co %q or name co %q", query, query)
		commands = append(commands, Command{
			Key:         "search-apps",
			Description: "Find service/resource apps and existing companion OAuth apps by display name or app name.",
			Command: identityDomainsCommand(endpoint, defaults, "apps", "search",
				"--schemas '[\"urn:ietf:params:scim:api:messages:2.0:SearchRequest\"]'",
				"--filter "+shellQuote(filter),
				"--attributes '[\"id\",\"displayName\",\"name\",\"clientType\",\"allowedGrants\",\"allowedScopes\",\"scopes\",\"userRoles\",\"grantedAppRoles\",\"certificates\",\"isOAuthClient\",\"isOAuthResource\"]'",
				"--count 50"),
		})
	}
	if appID != "" {
		commands = append(commands,
			Command{
				Key:         "get-resource-app",
				Description: "Inspect the service/resource app for scopes, app roles, and Cloud service metadata.",
				Command: identityDomainsCommand(endpoint, defaults, "app", "get",
					"--app-id "+shellQuote(appID),
					"--attributes 'id,displayName,name,allowedGrants,allowedScopes,scopes,userRoles,grantedAppRoles,accounts,audience,serviceTypeURN,isOAuthClient,isOAuthResource'"),
			},
			Command{
				Key:         "search-grants-for-resource-app",
				Description: "Find grants where this service/resource app is the granted server app.",
				Command: identityDomainsCommand(endpoint, defaults, "grants", "search",
					"--schemas '[\"urn:ietf:params:scim:api:messages:2.0:SearchRequest\"]'",
					"--filter "+shellQuote("app.value eq \""+appID+"\""),
					"--attributes '[\"id\",\"app\",\"entitlement\",\"grantee\",\"grantMechanism\",\"isFulfilled\"]'",
					"--count 100"),
			},
		)
	}
	return Plan{
		SchemaVersion: SchemaVersion,
		IDCSEndpoint:  endpoint,
		Query:         query,
		AppID:         appID,
		Profile:       strings.TrimSpace(options.Profile),
		OCIConfigPath: strings.TrimSpace(options.OCIConfigPath),
		Region:        strings.TrimSpace(options.Region),
		Commands:      commands,
		NextSteps: []string{
			"Use the service/resource app id as --resource-app-id.",
			"Use scopes[].fqs or allowedScopes[].fqs as --scope or --platform.",
			"Use userRoles/grantedAppRoles values as --app-role-grants NAME=APP_ROLE_ID.",
			"Use role presets for expected roles, then override preset placeholders with discovered app-role ids.",
		},
	}, nil
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
