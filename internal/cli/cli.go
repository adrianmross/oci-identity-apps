package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/adrianmross/oci-idm/internal/applyexec"
	"github.com/adrianmross/oci-idm/internal/diagnose"
	"github.com/adrianmross/oci-idm/internal/discovery"
	"github.com/adrianmross/oci-idm/internal/doctor"
	"github.com/adrianmross/oci-idm/internal/handoff"
	"github.com/adrianmross/oci-idm/internal/materialize"
	"github.com/adrianmross/oci-idm/internal/planner"
	"github.com/adrianmross/oci-idm/internal/validation"
)

var (
	version     = "dev"
	commit      = "none"
	date        = "unknown"
	stdinReader = io.Reader(os.Stdin)
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
	case "get":
		if err := runGet(args[1:], stdout); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	case "describe":
		if err := runDescribe(args[1:], stdout); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	case "plan":
		commandArgs, err := stripResourceArg(args[1:], "app", "apps", "identity-app", "identity-apps")
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		if err := runPlan(commandArgs, stdout); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	case "defaults", "context":
		if err := runDefaults(args[1:], stdout); err != nil {
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
		commandArgs, err := stripResourceArg(args[1:], "app", "apps", "service-app", "service-apps", "identity-app", "identity-apps")
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		if err := runDiagnose(commandArgs, stdout); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	case "materialize":
		commandArgs, err := stripResourceArg(args[1:], "plan", "plans")
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		if err := runMaterialize(commandArgs, stdout); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	case "apply":
		commandArgs, err := stripResourceArg(args[1:], "plan", "plans")
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		if err := runApply(commandArgs, stdout); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	case "validate":
		commandArgs, err := stripResourceArg(args[1:], "plan", "plans")
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		if err := runValidate(commandArgs, stdout); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	case "doctor":
		commandArgs, err := stripResourceArg(args[1:], "plan", "plans", "context", "contexts")
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		if err := runDoctor(commandArgs, stdout); err != nil {
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

func runGet(args []string, stdout io.Writer) error {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return fmt.Errorf("get requires a resource: defaults, context, service-apps, or apps")
	}
	resource := strings.ToLower(strings.TrimSpace(args[0]))
	commandArgs := args[1:]
	switch resource {
	case "defaults", "default", "context", "contexts":
		return runDefaults(commandArgs, stdout)
	case "app", "apps", "service-app", "service-apps", "resource-app", "resource-apps", "identity-app", "identity-apps":
		return runDiscover(commandArgs, stdout)
	default:
		return fmt.Errorf("unsupported get resource %q", args[0])
	}
}

func runDescribe(args []string, stdout io.Writer) error {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return fmt.Errorf("describe requires a resource: service-app or app")
	}
	resource := strings.ToLower(strings.TrimSpace(args[0]))
	commandArgs := args[1:]
	switch resource {
	case "app", "apps", "service-app", "service-apps", "resource-app", "resource-apps", "identity-app", "identity-apps":
		return runDiscover(commandArgs, stdout)
	default:
		return fmt.Errorf("unsupported describe resource %q", args[0])
	}
}

func stripResourceArg(args []string, allowed ...string) ([]string, error) {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return args, nil
	}
	resource := strings.ToLower(strings.TrimSpace(args[0]))
	for _, value := range allowed {
		if resource == value {
			return args[1:], nil
		}
	}
	return nil, fmt.Errorf("unsupported resource %q", args[0])
}

func runDefaults(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("defaults", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	service := flags.String("service", string(planner.ServiceOBP), "service preset used for token-service defaults")
	ociContextService := flags.String("oci-context-service", "", "oci-context token service name; defaults from --service")
	ociContextBin := flags.String("oci-context-bin", "oci-context", "oci-context binary used for defaults")
	var output string
	addOutputFlags(flags, &output, "json", "output format: json or text")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q", flags.Arg(0))
	}
	defaults := resolvedDoctorDefaults(*ociContextBin, *service, *ociContextService)
	return printDefaults(stdout, defaults, output)
}

func addOutputFlags(flags *flag.FlagSet, output *string, defaultValue string, usage string) {
	flags.StringVar(output, "output", defaultValue, usage)
	flags.StringVar(output, "o", defaultValue, usage+" (shorthand)")
	flags.StringVar(output, "format", defaultValue, usage+" (alias)")
}

func addFileFlags(flags *flag.FlagSet, path *string, usage string) {
	flags.StringVar(path, "file", "", usage)
	flags.StringVar(path, "f", "", usage+" (shorthand)")
	flags.StringVar(path, "plan", "", usage+" (alias)")
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
	var output string
	addOutputFlags(flags, &output, "json", "output format: json, text, oci-context-yaml, oci-context-json, commands, ochain-env, ochain-dotenv, or ochain-json")
	tokenService := flags.String("token-service", "", "token service name for OChain output")

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

	return printPlanOutput(stdout, plan, output, *tokenService)
}

func printPlanOutput(stdout io.Writer, plan planner.Plan, output string, tokenService string) error {
	ociContext := handoff.ForOCIContext(plan)
	switch strings.ToLower(strings.TrimSpace(output)) {
	case "json":
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(plan)
	case "text":
		writeTextPlan(stdout, plan)
		return nil
	case "oci-context-json", "context-json":
		data, err := handoff.JSON(ociContext)
		if err != nil {
			return err
		}
		_, err = stdout.Write(data)
		return err
	case "oci-context-yaml", "token-services-yaml":
		fmt.Fprint(stdout, handoff.TokenServicesYAML(ociContext))
		return nil
	case "oci-context-commands", "commands", "sh", "shell":
		fmt.Fprint(stdout, handoff.TokenCommandsScript(ociContext))
		return nil
	case "ochain-env":
		env, err := handoff.OChainEnv(ociContext, tokenService)
		if err != nil {
			return err
		}
		fmt.Fprint(stdout, env)
		return nil
	case "ochain-dotenv":
		env, err := handoff.OChainDotenv(ociContext, tokenService)
		if err != nil {
			return err
		}
		fmt.Fprint(stdout, env)
		return nil
	case "ochain-json":
		data, err := handoff.OChainJSON(ociContext, tokenService)
		if err != nil {
			return err
		}
		_, err = stdout.Write(data)
		return err
	default:
		return fmt.Errorf("unsupported output %q", output)
	}
}

func runApply(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("apply", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var planPath string
	addFileFlags(flags, &planPath, "path to a JSON plan emitted by oci-idm plan, or - for stdin")
	outDir := flags.String("out", "", "directory for generated apply artifacts")
	execute := flags.Bool("execute", false, "execute OCI changes directly")
	confirm := flags.Bool("confirm", false, "required with --execute")
	var output string
	addOutputFlags(flags, &output, "text", "output format: text or json")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q", flags.Arg(0))
	}
	if strings.TrimSpace(planPath) == "" {
		return fmt.Errorf("-f/--file is required")
	}
	if *execute {
		if !*confirm {
			return fmt.Errorf("--execute requires --confirm")
		}
		plan, err := readPlanFile(planPath)
		if err != nil {
			return err
		}
		result, err := applyexec.Execute(plan, *outDir, applyexec.Runner(runCommand))
		if err != nil {
			return err
		}
		return printApplyResult(stdout, result, output)
	}
	result, err := materialize.FromPlanFile(planPath, *outDir)
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
	var output string
	addOutputFlags(flags, &output, "json", "output format: json or text")
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
	switch strings.ToLower(strings.TrimSpace(output)) {
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
		return fmt.Errorf("unsupported output %q", output)
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
	var output string
	addOutputFlags(flags, &output, "json", "output format: json or text")
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
	switch strings.ToLower(strings.TrimSpace(output)) {
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
		return fmt.Errorf("unsupported output %q", output)
	}
}

func runMaterialize(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("materialize", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var planPath string
	addFileFlags(flags, &planPath, "path to a JSON plan emitted by oci-idm plan")
	outDir := flags.String("out", "", "directory for payload JSON files and helper scripts")
	var output string
	addOutputFlags(flags, &output, "text", "output format: text or json")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q", flags.Arg(0))
	}
	if strings.TrimSpace(planPath) == "" {
		return fmt.Errorf("-f/--file is required")
	}
	result, err := materialize.FromPlanFile(planPath, *outDir)
	if err != nil {
		return err
	}
	switch strings.ToLower(strings.TrimSpace(output)) {
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
		return fmt.Errorf("unsupported output %q", output)
	}
}

func runValidate(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("validate", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var planPath string
	addFileFlags(flags, &planPath, "path to a JSON plan emitted by oci-idm plan")
	var output string
	addOutputFlags(flags, &output, "json", "output format: json or text")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q", flags.Arg(0))
	}
	if strings.TrimSpace(planPath) == "" {
		return fmt.Errorf("-f/--file is required")
	}
	report, err := validation.FromPlanFile(planPath)
	if err != nil {
		return err
	}
	switch strings.ToLower(strings.TrimSpace(output)) {
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
		return fmt.Errorf("unsupported output %q", output)
	}
}

func runDoctor(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("doctor", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var planPath string
	addFileFlags(flags, &planPath, "optional JSON plan emitted by oci-idm plan, or - for stdin")
	service := flags.String("service", string(planner.ServiceOBP), "service preset used for token-service defaults")
	ociContextService := flags.String("oci-context-service", "", "oci-context token service name; defaults from --service")
	ociContextBin := flags.String("oci-context-bin", "oci-context", "oci-context binary used for defaults")
	var output string
	addOutputFlags(flags, &output, "json", "output format: json or text")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q", flags.Arg(0))
	}
	defaults := resolvedDoctorDefaults(*ociContextBin, *service, *ociContextService)
	var report doctor.Report
	var err error
	if strings.TrimSpace(planPath) != "" {
		report, err = doctor.FromPlanFile(planPath, defaults)
	} else {
		report = doctor.FromDefaults(defaults)
	}
	if err != nil {
		return err
	}
	return printDoctorReport(stdout, report, output)
}

func runHandoff(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("handoff", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var planPath string
	addFileFlags(flags, &planPath, "path to a JSON plan emitted by oci-idm plan, or - for stdin")
	target := flags.String("target", "oci-context", "handoff target: oci-context or ochain")
	var output string
	addOutputFlags(flags, &output, "json", "output format: json, yaml, commands, env, or dotenv")
	importToOCIContext := flags.Bool("import", false, "import generated token services into oci-context")
	importDryRun := flags.Bool("dry-run", false, "preview oci-context import changes without writing config")
	outDir := flags.String("out", "", "directory for generated handoff artifacts when using --import")
	ociContextBin := flags.String("oci-context-bin", "oci-context", "oci-context binary used for --import")
	tokenService := flags.String("token-service", "", "token service name for OChain handoff output")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q", flags.Arg(0))
	}
	if strings.TrimSpace(planPath) == "" {
		return fmt.Errorf("-f/--file is required")
	}
	normalizedTarget := strings.ToLower(strings.TrimSpace(*target))
	normalizedOutput := strings.ToLower(strings.TrimSpace(output))
	if normalizedTarget == "oci-context" && strings.HasPrefix(normalizedOutput, "ochain") {
		normalizedTarget = "ochain"
	}
	if normalizedTarget != "oci-context" && normalizedTarget != "ochain" {
		return fmt.Errorf("unsupported handoff target %q", *target)
	}
	if *importToOCIContext {
		if normalizedTarget != "oci-context" {
			return fmt.Errorf("--import only supports --target oci-context")
		}
		result, err := materialize.FromPlanFile(planPath, *outDir)
		if err != nil {
			return err
		}
		file := filepath.Join(result.OutDir, "oci-context-token-services.yml")
		args := []string{"auth", "service", "import", "--file", file}
		if *importDryRun {
			args = append(args, "--dry-run")
		}
		out, err := runCommand(*ociContextBin, args...)
		if err != nil {
			return err
		}
		fmt.Fprint(stdout, string(out))
		return nil
	}
	plan, err := readPlanFile(planPath)
	if err != nil {
		return err
	}
	ociContext := handoff.ForOCIContext(plan)
	if normalizedTarget == "ochain" {
		return printOChainHandoff(stdout, ociContext, normalizedOutput, *tokenService)
	}
	switch normalizedOutput {
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
		return fmt.Errorf("unsupported output %q", output)
	}
}

func printOChainHandoff(stdout io.Writer, value handoff.OCIContext, output string, tokenService string) error {
	switch output {
	case "env", "sh", "shell", "export", "exports", "ochain-env", "ochain":
		env, err := handoff.OChainEnv(value, tokenService)
		if err != nil {
			return err
		}
		fmt.Fprint(stdout, env)
		return nil
	case "dotenv", "ochain-dotenv":
		env, err := handoff.OChainDotenv(value, tokenService)
		if err != nil {
			return err
		}
		fmt.Fprint(stdout, env)
		return nil
	case "json", "ochain-json":
		data, err := handoff.OChainJSON(value, tokenService)
		if err != nil {
			return err
		}
		_, err = stdout.Write(data)
		return err
	default:
		return fmt.Errorf("unsupported output %q for target ochain", output)
	}
}

func readPlanFile(path string) (planner.Plan, error) {
	var data []byte
	var err error
	if strings.TrimSpace(path) == "-" {
		data, err = io.ReadAll(stdinReader)
	} else {
		data, err = os.ReadFile(path)
	}
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
  %s get defaults [options]
  %s get service-apps [options]
  %s describe service-app [options]
  %s plan apps [options]
  %s plan apps [options] -o oci-context-yaml
  %s plan apps [options] -o ochain-env
  %s diagnose apps [options]
  %s doctor plan -f plan.json
  %s materialize plan -f plan.json --out ./idcs-artifacts
  %s handoff -f plan.json --target oci-context -o yaml
  %s handoff -f plan.json --import --out ./idcs-artifacts
  %s apply plan -f plan.json --out ./idcs-artifacts
  %s apply plan -f plan.json --execute --confirm
  %s validate plan -f plan.json
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
  -o, --output json|text|oci-context-yaml|oci-context-json|commands|ochain-env|ochain-dotenv|ochain-json

Pipe contracts:
  plan-consuming commands accept -f - for stdin
  plan apps -o oci-context-yaml can pipe into oci-context auth service import --file -
  plan apps -o ochain-env emits OCHAIN_TOKEN_COMMAND
  handoff remains available for saved plan files
`, program, program, program, program, program, program, program, program, program, program, program, program, program, program, program, program)
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

func resolvedDoctorDefaults(bin string, service string, serviceOverride string) doctor.Defaults {
	serviceName := firstNonEmpty(serviceOverride, defaultOCIContextServiceName(service), service)
	defaults := loadOCIContextDefaults(bin, serviceName)
	return doctor.Defaults{
		ContextName:   defaults.ContextName,
		Profile:       defaults.Profile,
		Region:        defaults.Region,
		OCIConfigPath: defaults.OCIConfigPath,
		ServiceName:   serviceName,
		Issuer:        defaults.Issuer,
		Scope:         defaults.Scope,
	}
}

func printDefaults(stdout io.Writer, defaults doctor.Defaults, output string) error {
	switch strings.ToLower(strings.TrimSpace(output)) {
	case "json":
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(defaults)
	case "text":
		fmt.Fprintf(stdout, "context: %s\n", defaults.ContextName)
		fmt.Fprintf(stdout, "profile: %s\n", defaults.Profile)
		fmt.Fprintf(stdout, "region: %s\n", defaults.Region)
		fmt.Fprintf(stdout, "ociConfigPath: %s\n", defaults.OCIConfigPath)
		fmt.Fprintf(stdout, "service: %s\n", defaults.ServiceName)
		fmt.Fprintf(stdout, "issuer: %s\n", defaults.Issuer)
		fmt.Fprintf(stdout, "scope: %s\n", defaults.Scope)
		return nil
	default:
		return fmt.Errorf("unsupported output %q", output)
	}
}

func printDoctorReport(stdout io.Writer, report doctor.Report, output string) error {
	switch strings.ToLower(strings.TrimSpace(output)) {
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
		return fmt.Errorf("unsupported output %q", output)
	}
}

func printApplyResult(stdout io.Writer, result applyexec.Result, output string) error {
	switch strings.ToLower(strings.TrimSpace(output)) {
	case "json":
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(result)
	case "text":
		for _, step := range result.Steps {
			fmt.Fprintf(stdout, "%s: %s", step.Status, step.Key)
			if step.ID != "" {
				fmt.Fprintf(stdout, " id=%s", step.ID)
			}
			if step.Message != "" {
				fmt.Fprintf(stdout, " - %s", step.Message)
			}
			fmt.Fprintln(stdout)
		}
		return nil
	default:
		return fmt.Errorf("unsupported output %q", output)
	}
}
