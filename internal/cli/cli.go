package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/adrianmross/oci-identity-apps/internal/diagnose"
	"github.com/adrianmross/oci-identity-apps/internal/discovery"
	"github.com/adrianmross/oci-identity-apps/internal/handoff"
	"github.com/adrianmross/oci-identity-apps/internal/materialize"
	"github.com/adrianmross/oci-identity-apps/internal/planner"
	"github.com/adrianmross/oci-identity-apps/internal/validation"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func Run(args []string, stdout io.Writer, stderr io.Writer) int {
	return RunWithName("oci-idm", args, stdout, stderr)
}

func RunWithName(program string, args []string, stdout io.Writer, stderr io.Writer) int {
	if strings.TrimSpace(program) == "" {
		program = "oci-idm"
	}
	if len(args) == 0 {
		writeRootHelp(stdout, program)
		return 0
	}

	switch args[0] {
	case "help", "-h", "--help":
		writeRootHelp(stdout, program)
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
	case "discover":
		if err := runDiscover(args[1:], stdout); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	case "diagnose":
		if err := runDiagnose(args[1:], stdout); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	case "materialize":
		if err := runMaterialize(args[1:], stdout); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	case "apply":
		if err := runApply(args[1:], stdout); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	case "validate":
		if err := runValidate(args[1:], stdout); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	case "handoff":
		if err := runHandoff(args[1:], stdout); err != nil {
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
	resourceAppID := flags.String("resource-app-id", "", "target service/resource app id that defines the requested scope")
	baseAppName := flags.String("base-app-name", "", "service-created app name to document as source context")
	baseAppDisplayName := flags.String("base-app-display-name", "", "service-created app display name")
	appPrefix := flags.String("app-prefix", "", "prefix for generated companion app names")
	redirectURL := flags.String("redirect-url", planner.DefaultCLIRedirectURL, "loopback redirect URL for CLI auth-code flow")
	include := flags.String("include", "user,service,jwt", "comma list of apps to plan: user,service,jwt,jwt-service,jwt-user,workload")
	userClientType := flags.String("user-client-type", string(planner.ClientPublic), "user app client type: public or confidential")
	principalMode := flags.String("principal-mode", string(planner.PrincipalAuto), "service principal mode: auto, none, or same-name-user")
	principalEmailDomain := flags.String("principal-email-domain", "example.invalid", "email domain for generated same-name principal users")
	rolePreset := flags.String("role-preset", "none", "comma list of service role presets: none,obp-admin,obp-rest-client,obp-user,obp-ca-user")
	appRoleGrants := flags.String("app-role-grants", "", "comma list of target service app role grants as NAME=APP_ROLE_ID entries")
	certificateAlias := flags.String("certificate-alias", "", "certificate alias for JWT client assertion apps; defaults to <app-name>-cert")
	templateID := flags.String("template-id", "", "template id override for all planned apps")
	userTemplateID := flags.String("user-template-id", "", "template id for the user app")
	serviceTemplateID := flags.String("service-template-id", "", "template id for the service app")
	jwtTemplateID := flags.String("jwt-template-id", "", "template id for the JWT assertion app")
	accessTokenExpiry := flags.Int("access-token-expiry", 0, "optional access token expiry in seconds")
	refreshTokenExpiry := flags.Int("refresh-token-expiry", 0, "optional refresh token expiry in seconds")
	profile := flags.String("profile", "", "OCI CLI profile for generated Identity Domains commands; defaults from current oci-context")
	ociConfigPath := flags.String("oci-config-file", "", "OCI CLI config file for generated commands; defaults from current oci-context")
	region := flags.String("region", "", "OCI region for generated commands; defaults from current oci-context")
	useOCIContext := flags.Bool("oci-context", true, "read current oci-context and token-service defaults for omitted values")
	ociContextBin := flags.String("oci-context-bin", "oci-context", "oci-context binary used for defaults")
	ociContextService := flags.String("oci-context-service", "", "oci-context token service used for issuer/scope defaults; defaults to --service for non-generic services")
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
	parsedPrincipalMode, err := planner.ParsePrincipalMode(*principalMode)
	if err != nil {
		return err
	}
	grants, err := planner.ParseAppRoleGrants(*appRoleGrants)
	if err != nil {
		return err
	}
	presets, err := planner.ParseRolePresets(*rolePreset)
	if err != nil {
		return err
	}
	visited := collectVisitedFlags(flags)
	contextName := ""
	if *useOCIContext {
		serviceName := firstNonEmpty(*ociContextService, defaultOCIContextServiceName(*service))
		defaults := loadOCIContextDefaults(*ociContextBin, serviceName)
		contextName = defaults.ContextName
		if !explicitFlags(visited, "issuer") && strings.TrimSpace(*issuer) == "" {
			*issuer = defaults.Issuer
		}
		if !explicitFlags(visited, "scope") && strings.TrimSpace(*scope) == "" {
			*scope = defaults.Scope
		}
		if !explicitFlags(visited, "platform") && strings.TrimSpace(*platform) == "" && planner.ServiceKind(*service) == planner.ServiceOBP {
			*platform = defaults.Scope
		}
		if !explicitFlags(visited, "profile") && strings.TrimSpace(*profile) == "" {
			*profile = defaults.Profile
		}
		if !explicitFlags(visited, "oci-config-file") && strings.TrimSpace(*ociConfigPath) == "" {
			*ociConfigPath = defaults.OCIConfigPath
		}
		if !explicitFlags(visited, "region") && strings.TrimSpace(*region) == "" {
			*region = defaults.Region
		}
	}
	plan, err := planner.Build(planner.Options{
		Service:              planner.ServiceKind(*service),
		Platform:             *platform,
		Issuer:               *issuer,
		Scope:                *scope,
		IDCSEndpoint:         *idcsEndpoint,
		ResourceAppID:        *resourceAppID,
		BaseAppName:          *baseAppName,
		BaseAppDisplayName:   *baseAppDisplayName,
		AppPrefix:            *appPrefix,
		RedirectURL:          *redirectURL,
		Include:              includes,
		UserClientType:       clientType,
		PrincipalMode:        parsedPrincipalMode,
		PrincipalEmailDomain: *principalEmailDomain,
		RolePresets:          presets,
		AppRoleGrants:        grants,
		CertificateAlias:     *certificateAlias,
		TemplateID:           *templateID,
		UserTemplateID:       *userTemplateID,
		ServiceTemplateID:    *serviceTemplateID,
		JWTTemplateID:        *jwtTemplateID,
		AccessTokenExpiry:    *accessTokenExpiry,
		RefreshTokenExpiry:   *refreshTokenExpiry,
		OCIContext:           contextName,
		OCIProfile:           *profile,
		OCIConfigPath:        *ociConfigPath,
		OCIRegion:            *region,
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

func runApply(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("apply", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	planPath := flags.String("plan", "", "path to a JSON plan emitted by oci-identity-apps plan")
	outDir := flags.String("out", "", "directory for generated apply artifacts")
	execute := flags.Bool("execute", false, "execute OCI changes directly")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q", flags.Arg(0))
	}
	if *execute {
		return fmt.Errorf("--execute is intentionally not implemented; run materialize, review payloads, then run apply.sh explicitly")
	}
	if strings.TrimSpace(*planPath) == "" {
		return fmt.Errorf("--plan is required")
	}
	result, err := materialize.FromPlanFile(*planPath, *outDir)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "dry-run apply artifacts written to %s\n", result.OutDir)
	fmt.Fprintf(stdout, "review payloads, replace placeholders, then run %s\n", result.OutDir+"/apply.sh")
	return nil
}

func runDiscover(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("discover", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	issuer := flags.String("issuer", "", "OCI Identity Domains issuer URL")
	idcsEndpoint := flags.String("idcs-endpoint", "", "OCI Identity Domains base endpoint")
	query := flags.String("query", "", "service/app search text")
	appID := flags.String("app-id", "", "service/resource app id to inspect")
	profile := flags.String("profile", "", "optional OCI CLI profile to include in generated commands; defaults from current oci-context")
	ociConfigPath := flags.String("oci-config-file", "", "OCI CLI config file to include in generated commands; defaults from current oci-context")
	region := flags.String("region", "", "OCI region to include in generated commands; defaults from current oci-context")
	useOCIContext := flags.Bool("oci-context", true, "read current oci-context defaults for omitted values")
	ociContextBin := flags.String("oci-context-bin", "oci-context", "oci-context binary used for defaults")
	ociContextService := flags.String("oci-context-service", string(planner.ServiceOBP), "oci-context token service used for issuer defaults")
	format := flags.String("format", "json", "output format: json or text")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q", flags.Arg(0))
	}
	visited := collectVisitedFlags(flags)
	if *useOCIContext {
		defaults := loadOCIContextDefaults(*ociContextBin, *ociContextService)
		if !explicitFlags(visited, "issuer") && strings.TrimSpace(*issuer) == "" {
			*issuer = defaults.Issuer
		}
		if !explicitFlags(visited, "profile") && strings.TrimSpace(*profile) == "" {
			*profile = defaults.Profile
		}
		if !explicitFlags(visited, "oci-config-file") && strings.TrimSpace(*ociConfigPath) == "" {
			*ociConfigPath = defaults.OCIConfigPath
		}
		if !explicitFlags(visited, "region") && strings.TrimSpace(*region) == "" {
			*region = defaults.Region
		}
	}
	plan, err := discovery.Build(discovery.Options{
		Issuer:        *issuer,
		IDCSEndpoint:  *idcsEndpoint,
		Query:         *query,
		AppID:         *appID,
		Profile:       *profile,
		OCIConfigPath: *ociConfigPath,
		Region:        *region,
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
		fmt.Fprintf(stdout, "idcsEndpoint: %s\n", plan.IDCSEndpoint)
		for _, command := range plan.Commands {
			fmt.Fprintf(stdout, "%s: %s\n  %s\n", command.Key, command.Description, command.Command)
		}
		return nil
	default:
		return fmt.Errorf("unsupported format %q", *format)
	}
}

func runDiagnose(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("diagnose", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	service := flags.String("service", string(diagnose.ServiceGeneric), "service preset: generic or obp")
	issuer := flags.String("issuer", "", "OCI Identity Domains issuer URL")
	idcsEndpoint := flags.String("idcs-endpoint", "", "OCI Identity Domains base endpoint")
	resourceAppID := flags.String("resource-app-id", "", "target service/resource app id")
	candidateAppID := flags.String("candidate-app-id", "", "candidate OAuth client app id")
	knownGoodAppID := flags.String("known-good-app-id", "", "optional known-working OAuth client app id to compare")
	profile := flags.String("profile", "", "optional OCI CLI profile to include in generated commands; defaults from current oci-context")
	ociConfigPath := flags.String("oci-config-file", "", "OCI CLI config file to include in generated commands; defaults from current oci-context")
	region := flags.String("region", "", "OCI region to include in generated commands; defaults from current oci-context")
	useOCIContext := flags.Bool("oci-context", true, "read current oci-context and token-service defaults for omitted values")
	ociContextBin := flags.String("oci-context-bin", "oci-context", "oci-context binary used for defaults")
	ociContextService := flags.String("oci-context-service", "", "oci-context token service used for issuer defaults; defaults to --service for non-generic services")
	format := flags.String("format", "json", "output format: json or text")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q", flags.Arg(0))
	}
	visited := collectVisitedFlags(flags)
	if *useOCIContext {
		serviceName := firstNonEmpty(*ociContextService, defaultOCIContextServiceName(*service))
		defaults := loadOCIContextDefaults(*ociContextBin, serviceName)
		if !explicitFlags(visited, "issuer") && strings.TrimSpace(*issuer) == "" {
			*issuer = defaults.Issuer
		}
		if !explicitFlags(visited, "profile") && strings.TrimSpace(*profile) == "" {
			*profile = defaults.Profile
		}
		if !explicitFlags(visited, "oci-config-file") && strings.TrimSpace(*ociConfigPath) == "" {
			*ociConfigPath = defaults.OCIConfigPath
		}
		if !explicitFlags(visited, "region") && strings.TrimSpace(*region) == "" {
			*region = defaults.Region
		}
	}
	plan, err := diagnose.Build(diagnose.Options{
		Service:        diagnose.ServiceKind(*service),
		Issuer:         *issuer,
		IDCSEndpoint:   *idcsEndpoint,
		ResourceAppID:  *resourceAppID,
		CandidateAppID: *candidateAppID,
		KnownGoodAppID: *knownGoodAppID,
		Profile:        *profile,
		OCIConfigPath:  *ociConfigPath,
		Region:         *region,
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
		fmt.Fprintf(stdout, "idcsEndpoint: %s\n", plan.IDCSEndpoint)
		fmt.Fprintf(stdout, "resourceAppId: %s\n", plan.ResourceAppID)
		if plan.CandidateAppID != "" {
			fmt.Fprintf(stdout, "candidateAppId: %s\n", plan.CandidateAppID)
		}
		if plan.KnownGoodAppID != "" {
			fmt.Fprintf(stdout, "knownGoodAppId: %s\n", plan.KnownGoodAppID)
		}
		for _, command := range plan.Commands {
			fmt.Fprintf(stdout, "%s: %s\n  %s\n", command.Key, command.Description, command.Command)
		}
		for _, item := range plan.Interpretation {
			fmt.Fprintf(stdout, "note: %s\n", item)
		}
		return nil
	default:
		return fmt.Errorf("unsupported format %q", *format)
	}
}

func runMaterialize(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("materialize", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	planPath := flags.String("plan", "", "path to a JSON plan emitted by oci-identity-apps plan")
	outDir := flags.String("out", "", "directory for payload JSON files and helper scripts")
	format := flags.String("format", "text", "output format: text or json")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q", flags.Arg(0))
	}
	if strings.TrimSpace(*planPath) == "" {
		return fmt.Errorf("--plan is required")
	}
	result, err := materialize.FromPlanFile(*planPath, *outDir)
	if err != nil {
		return err
	}
	switch strings.ToLower(strings.TrimSpace(*format)) {
	case "json":
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(result)
	case "text":
		fmt.Fprintf(stdout, "wrote %d files to %s\n", len(result.Files), result.OutDir)
		for _, file := range result.Files {
			fmt.Fprintf(stdout, "  %s\n", file)
		}
		return nil
	default:
		return fmt.Errorf("unsupported format %q", *format)
	}
}

func runValidate(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("validate", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	planPath := flags.String("plan", "", "path to a JSON plan emitted by oci-identity-apps plan")
	format := flags.String("format", "json", "output format: json or text")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q", flags.Arg(0))
	}
	if strings.TrimSpace(*planPath) == "" {
		return fmt.Errorf("--plan is required")
	}
	report, err := validation.FromPlanFile(*planPath)
	if err != nil {
		return err
	}
	switch strings.ToLower(strings.TrimSpace(*format)) {
	case "json":
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(report)
	case "text":
		for _, check := range report.Checks {
			fmt.Fprintf(stdout, "%s: %s", check.Status, check.Key)
			if check.Message != "" {
				fmt.Fprintf(stdout, " - %s", check.Message)
			}
			fmt.Fprintln(stdout)
		}
		for _, command := range report.Commands {
			fmt.Fprintf(stdout, "manual: %s\n  %s\n", command.Key, command.Command)
		}
		return nil
	default:
		return fmt.Errorf("unsupported format %q", *format)
	}
}

func runHandoff(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("handoff", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	planPath := flags.String("plan", "", "path to a JSON plan emitted by oci-idm plan")
	target := flags.String("target", "oci-context", "handoff target: oci-context")
	format := flags.String("format", "json", "output format for --target oci-context: json, yaml, or commands")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q", flags.Arg(0))
	}
	if strings.TrimSpace(*planPath) == "" {
		return fmt.Errorf("--plan is required")
	}
	if strings.ToLower(strings.TrimSpace(*target)) != "oci-context" {
		return fmt.Errorf("unsupported handoff target %q", *target)
	}
	plan, err := readPlanFile(*planPath)
	if err != nil {
		return err
	}
	ociContext := handoff.ForOCIContext(plan)
	switch strings.ToLower(strings.TrimSpace(*format)) {
	case "json":
		data, err := handoff.JSON(ociContext)
		if err != nil {
			return err
		}
		_, err = stdout.Write(data)
		return err
	case "yaml", "yml":
		fmt.Fprint(stdout, handoff.TokenServicesYAML(ociContext))
		return nil
	case "commands", "sh", "shell":
		fmt.Fprint(stdout, handoff.TokenCommandsScript(ociContext))
		return nil
	default:
		return fmt.Errorf("unsupported format %q", *format)
	}
}

func readPlanFile(path string) (planner.Plan, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return planner.Plan{}, err
	}
	var plan planner.Plan
	if err := json.Unmarshal(data, &plan); err != nil {
		return planner.Plan{}, err
	}
	return plan, nil
}

func writeRootHelp(stdout io.Writer, program string) {
	fmt.Fprintf(stdout, `%s plans OCI Identity Domains apps, grants, and token-helper handoffs.

Usage:
  %s plan [options]
  %s discover [options]
  %s diagnose [options]
  %s materialize --plan plan.json --out ./idcs-artifacts
  %s handoff --plan plan.json --target oci-context --format yaml
  %s apply --plan plan.json --out ./idcs-artifacts
  %s validate --plan plan.json
  %s version

Plan options:
  --service generic|obp
  --issuer https://idcs-example.identity.oraclecloud.com
  --scope https://service.example.com
  --platform https://service.example.com
  --resource-app-id target-service-app-id
  --role-preset obp-admin
  --app-role-grants ADMIN=app-role-id,REST_CLIENT=app-role-id
  --principal-mode auto|none|same-name-user
  --principal-email-domain example.invalid
  --include user,service,jwt
    jwt expands to jwt-service,jwt-user,workload
  --oci-context=true
    read profile, region, config path, issuer, and scope defaults from current oci-context
  --oci-context-service obp
    token service name for issuer/scope defaults
  --format json|text
`, program, program, program, program, program, program, program, program, program)
}

func writeTextPlan(stdout io.Writer, plan planner.Plan) {
	fmt.Fprintf(stdout, "schema: %s\n", plan.SchemaVersion)
	fmt.Fprintf(stdout, "service: %s\n", plan.Target.Service)
	if plan.Target.OCIContext != "" {
		fmt.Fprintf(stdout, "ociContext: %s\n", plan.Target.OCIContext)
	}
	if plan.Target.OCIProfile != "" {
		fmt.Fprintf(stdout, "ociProfile: %s\n", plan.Target.OCIProfile)
	}
	fmt.Fprintf(stdout, "issuer: %s\n", plan.Target.Issuer)
	fmt.Fprintf(stdout, "scope: %s\n", plan.Target.Scope)
	fmt.Fprintf(stdout, "idcsEndpoint: %s\n", plan.Target.IDCSEndpoint)
	if plan.Target.ResourceAppID != "" {
		fmt.Fprintf(stdout, "resourceAppId: %s\n", plan.Target.ResourceAppID)
	}
	fmt.Fprintf(stdout, "redirectUrl: %s\n", plan.Target.RedirectURL)
	fmt.Fprintln(stdout, "apps:")
	for _, app := range plan.Apps {
		fmt.Fprintf(stdout, "  - %s: %s grants %s\n", app.Name, app.ClientType, strings.Join(app.AllowedGrants, ", "))
		for _, action := range app.OCIPreCreate {
			fmt.Fprintf(stdout, "    pre-create: %s\n", action.Command)
		}
		fmt.Fprintf(stdout, "    create: %s\n", app.OCICreateCommand)
		for _, action := range app.OCIPostCreate {
			fmt.Fprintf(stdout, "    post-create: %s\n", action.Command)
		}
	}
}

func collectVisitedFlags(flags *flag.FlagSet) map[string]bool {
	visited := map[string]bool{}
	flags.Visit(func(f *flag.Flag) {
		visited[f.Name] = true
	})
	return visited
}

func defaultOCIContextServiceName(service string) string {
	switch strings.ToLower(strings.TrimSpace(service)) {
	case string(planner.ServiceOBP):
		return string(planner.ServiceOBP)
	default:
		return ""
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
