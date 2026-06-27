package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/adrianmross/oci-identity-apps/internal/planner"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func Run(args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) == 0 {
		writeRootHelp(stdout)
		return 0
	}

	switch args[0] {
	case "help", "-h", "--help":
		writeRootHelp(stdout)
		return 0
	case "version", "-v", "--version":
		fmt.Fprintln(stdout, versionString())
		return 0
	case "plan":
		if err := runPlan(args[1:], stdout); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	default:
		fmt.Fprintf(stderr, "unknown command %q\n", args[0])
		return 1
	}
}

func versionString() string {
	parts := []string{version}
	if commit != "" && commit != "none" {
		parts = append(parts, "commit="+commit)
	}
	if date != "" && date != "unknown" {
		parts = append(parts, "built="+date)
	}
	return strings.Join(parts, " ")
}

func runPlan(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("plan", flag.ContinueOnError)
	flags.SetOutput(io.Discard)

	service := flags.String("service", string(planner.ServiceGeneric), "service preset: generic or obp")
	platform := flags.String("platform", "", "target service platform URL")
	issuer := flags.String("issuer", "", "OCI Identity Domains issuer URL")
	scope := flags.String("scope", "", "OAuth scope; defaults to --platform for --service obp")
	idcsEndpoint := flags.String("idcs-endpoint", "", "OCI Identity Domains base endpoint")
	baseAppName := flags.String("base-app-name", "", "service-created app name to document as source context")
	baseAppDisplayName := flags.String("base-app-display-name", "", "service-created app display name")
	appPrefix := flags.String("app-prefix", "", "prefix for generated companion app names")
	redirectURL := flags.String("redirect-url", planner.DefaultCLIRedirectURL, "loopback redirect URL for CLI auth-code flow")
	include := flags.String("include", "user,service,jwt", "comma list of apps to plan: user,service,jwt,jwt-service,jwt-user,workload")
	userClientType := flags.String("user-client-type", string(planner.ClientPublic), "user app client type: public or confidential")
	templateID := flags.String("template-id", "", "template id override for all planned apps")
	userTemplateID := flags.String("user-template-id", "", "template id for the user app")
	serviceTemplateID := flags.String("service-template-id", "", "template id for the service app")
	jwtTemplateID := flags.String("jwt-template-id", "", "template id for the JWT assertion app")
	accessTokenExpiry := flags.Int("access-token-expiry", 0, "optional access token expiry in seconds")
	refreshTokenExpiry := flags.Int("refresh-token-expiry", 0, "optional refresh token expiry in seconds")
	format := flags.String("format", "json", "output format: json or text")

	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q", flags.Arg(0))
	}

	includes, err := planner.ParseIncludes(*include)
	if err != nil {
		return err
	}
	clientType, err := planner.ParseClientType(*userClientType)
	if err != nil {
		return err
	}
	plan, err := planner.Build(planner.Options{
		Service:            planner.ServiceKind(*service),
		Platform:           *platform,
		Issuer:             *issuer,
		Scope:              *scope,
		IDCSEndpoint:       *idcsEndpoint,
		BaseAppName:        *baseAppName,
		BaseAppDisplayName: *baseAppDisplayName,
		AppPrefix:          *appPrefix,
		RedirectURL:        *redirectURL,
		Include:            includes,
		UserClientType:     clientType,
		TemplateID:         *templateID,
		UserTemplateID:     *userTemplateID,
		ServiceTemplateID:  *serviceTemplateID,
		JWTTemplateID:      *jwtTemplateID,
		AccessTokenExpiry:  *accessTokenExpiry,
		RefreshTokenExpiry: *refreshTokenExpiry,
	})
	if err != nil {
		return err
	}

	switch strings.ToLower(strings.TrimSpace(*format)) {
	case "json":
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(plan)
	case "text":
		writeTextPlan(stdout, plan)
		return nil
	default:
		return fmt.Errorf("unsupported format %q", *format)
	}
}

func writeRootHelp(stdout io.Writer) {
	fmt.Fprint(stdout, `oci-identity-apps plans OCI Identity Domains OAuth applications.

Usage:
  oci-identity-apps plan [options]
  oci-identity-apps version

Plan options:
  --service generic|obp
  --issuer https://idcs-example.identity.oraclecloud.com
  --scope https://service.example.com
  --platform https://service.example.com
  --include user,service,jwt
    jwt expands to jwt-service,jwt-user,workload
  --format json|text
`)
}

func writeTextPlan(stdout io.Writer, plan planner.Plan) {
	fmt.Fprintf(stdout, "schema: %s\n", plan.SchemaVersion)
	fmt.Fprintf(stdout, "service: %s\n", plan.Target.Service)
	fmt.Fprintf(stdout, "issuer: %s\n", plan.Target.Issuer)
	fmt.Fprintf(stdout, "scope: %s\n", plan.Target.Scope)
	fmt.Fprintf(stdout, "idcsEndpoint: %s\n", plan.Target.IDCSEndpoint)
	fmt.Fprintf(stdout, "redirectUrl: %s\n", plan.Target.RedirectURL)
	fmt.Fprintln(stdout, "apps:")
	for _, app := range plan.Apps {
		fmt.Fprintf(stdout, "  - %s: %s grants %s\n", app.Name, app.ClientType, strings.Join(app.AllowedGrants, ", "))
		fmt.Fprintf(stdout, "    create: %s\n", app.OCICreateCommand)
	}
}
