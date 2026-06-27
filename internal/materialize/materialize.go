package materialize

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/adrianmross/oci-identity-apps/internal/planner"
)

type Result struct {
	OutDir string   `json:"outDir"`
	Files  []string `json:"files"`
}

func FromPlanFile(planPath string, outDir string) (Result, error) {
	data, err := os.ReadFile(planPath)
	if err != nil {
		return Result{}, err
	}
	var plan planner.Plan
	if err := json.Unmarshal(data, &plan); err != nil {
		return Result{}, err
	}
	if strings.TrimSpace(outDir) == "" {
		outDir = strings.TrimSuffix(filepath.Base(planPath), filepath.Ext(planPath)) + "-artifacts"
	}
	return FromPlan(plan, outDir)
}

func FromPlan(plan planner.Plan, outDir string) (Result, error) {
	if strings.TrimSpace(outDir) == "" {
		return Result{}, fmt.Errorf("output directory is required")
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return Result{}, err
	}
	result := Result{OutDir: outDir}
	writeJSON := func(name string, value any) error {
		data, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			return err
		}
		data = append(data, '\n')
		path := filepath.Join(outDir, name)
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return err
		}
		result.Files = append(result.Files, path)
		return nil
	}

	if err := writeJSON("plan.json", plan); err != nil {
		return Result{}, err
	}
	for _, app := range plan.Apps {
		for _, action := range app.OCIPreCreate {
			if action.PayloadFile != "" && action.Payload != nil {
				if err := writeJSON(action.PayloadFile, action.Payload); err != nil {
					return Result{}, err
				}
			}
		}
		if err := writeJSON(app.OCICreatePayloadFile, app.OCICreatePayload); err != nil {
			return Result{}, err
		}
		for _, action := range app.OCIPostCreate {
			if action.PayloadFile != "" && action.Payload != nil {
				if err := writeJSON(action.PayloadFile, action.Payload); err != nil {
					return Result{}, err
				}
			}
		}
	}
	scripts := map[string]string{
		"apply.sh":    applyScript(plan),
		"validate.sh": validateScript(plan),
		"cleanup.sh":  cleanupScript(plan),
	}
	for name, content := range scripts {
		path := filepath.Join(outDir, name)
		if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
			return Result{}, err
		}
		result.Files = append(result.Files, path)
	}
	return result, nil
}

func applyScript(plan planner.Plan) string {
	var b strings.Builder
	b.WriteString("#!/usr/bin/env bash\nset -euo pipefail\n\n")
	b.WriteString("cd \"$(dirname \"$0\")\"\n\n")
	b.WriteString("# Review every payload before running this script.\n")
	b.WriteString("# Replace <created-app-id> placeholders in grant payloads after app creation.\n\n")
	for _, command := range plan.Apply.PreCreateCommands {
		b.WriteString(command + "\n")
	}
	for _, command := range plan.Apply.Commands {
		b.WriteString(command + "\n")
	}
	for _, command := range plan.Apply.PostCreateCommands {
		b.WriteString(command + "\n")
	}
	return b.String()
}

func validateScript(plan planner.Plan) string {
	var b strings.Builder
	b.WriteString("#!/usr/bin/env bash\nset -euo pipefail\n\n")
	b.WriteString("cd \"$(dirname \"$0\")\"\n\n")
	b.WriteString("# Fill in generated client ids, secrets, private key paths, and target-service probe commands.\n\n")
	for _, app := range plan.Apps {
		switch app.Key {
		case planner.AppJWTService:
			certAlias := app.Name + "-cert"
			if len(app.OCICreatePayload.Certificates) > 0 {
				certAlias = app.OCICreatePayload.Certificates[0].CertAlias
			}
			b.WriteString("oci-context auth token \\\n")
			b.WriteString("  --service " + shellQuote(string(plan.Target.Service)) + " \\\n")
			b.WriteString("  --flow jwt-client-credentials \\\n")
			b.WriteString("  --token-endpoint " + shellQuote(plan.Target.Issuer+"/oauth2/v1/token") + " \\\n")
			b.WriteString("  --client-id " + shellQuote(app.Name) + " \\\n")
			b.WriteString("  --scope " + shellQuote(plan.Target.Scope) + " \\\n")
			b.WriteString("  --private-key-file " + shellQuote("<private-key-file>") + " \\\n")
			b.WriteString("  --key-id " + shellQuote(certAlias) + " \\\n")
			b.WriteString("  --jwt-audience https://identity.oraclecloud.com/ \\\n")
			b.WriteString("  --no-login \\\n")
			b.WriteString("  --format raw >/dev/null\n\n")
		case planner.AppService:
			b.WriteString("oci-context auth token \\\n")
			b.WriteString("  --service " + shellQuote(string(plan.Target.Service)) + " \\\n")
			b.WriteString("  --flow client-credentials \\\n")
			b.WriteString("  --issuer " + shellQuote(plan.Target.Issuer) + " \\\n")
			b.WriteString("  --client-id " + shellQuote(app.Name) + " \\\n")
			b.WriteString("  --client-secret " + shellQuote("<client-secret>") + " \\\n")
			b.WriteString("  --scope " + shellQuote(plan.Target.Scope) + " \\\n")
			b.WriteString("  --no-login \\\n")
			b.WriteString("  --format raw >/dev/null\n\n")
		}
	}
	return b.String()
}

func cleanupScript(plan planner.Plan) string {
	var b strings.Builder
	b.WriteString("#!/usr/bin/env bash\nset -euo pipefail\n\n")
	b.WriteString("cd \"$(dirname \"$0\")\"\n\n")
	b.WriteString("# Delete in reverse order. Replace placeholders with IDs returned by OCI Identity Domains.\n\n")
	for i := len(plan.Apps) - 1; i >= 0; i-- {
		app := plan.Apps[i]
		for range app.OCIPostCreate {
			b.WriteString("oci identity-domains grant delete --endpoint " + shellQuote(plan.Target.IDCSEndpoint) + " --grant-id <grant-id> --force\n")
		}
		b.WriteString("oci identity-domains app delete --endpoint " + shellQuote(plan.Target.IDCSEndpoint) + " --app-id <created-app-id-for-" + app.Name + "> --force\n")
		for range app.OCIPreCreate {
			b.WriteString("oci identity-domains o-auth-client-certificate delete --endpoint " + shellQuote(plan.Target.IDCSEndpoint) + " --o-auth-client-certificate-id <certificate-id> --force\n")
		}
	}
	return b.String()
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
