package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVersion(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run([]string{"version"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run failed with %d: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "dev") {
		t.Fatalf("unexpected version output: %q", stdout.String())
	}
}

func TestPlanJSON(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run([]string{
		"plan",
		"--service", "obp",
		"--issuer", "https://idcs-example.identity.oraclecloud.com",
		"--platform", "https://example-oabcs.blockchain.ocp.oraclecloud.com:7443/restproxy",
		"--base-app-name", "example-obp_APPID",
		"--include", "user,service",
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run failed with %d: %s", code, stderr.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if payload["schemaVersion"] != "oci-idm.plan.v1" {
		t.Fatalf("unexpected schema version: %#v", payload["schemaVersion"])
	}
	apps := payload["apps"].([]any)
	if len(apps) != 2 {
		t.Fatalf("expected 2 apps, got %d", len(apps))
	}
}

func TestPlanRejectsUnknownInclude(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run([]string{
		"plan",
		"--issuer", "https://idcs-example.identity.oraclecloud.com",
		"--scope", "https://service.example.com/.default",
		"--include", "bad",
	}, &stdout, &stderr)
	if code == 0 {
		t.Fatal("expected failure")
	}
	if stderr.Len() == 0 {
		t.Fatal("expected stderr output")
	}
}

func TestDiscoverText(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run([]string{
		"discover",
		"--issuer", "https://idcs-example.identity.oraclecloud.com",
		"--query", "example",
		"--app-id", "resource-app-id",
		"--profile", "DEFAULT",
		"--format", "text",
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run failed with %d: %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"search-apps", "get-resource-app", "search-grants-for-resource-app", "--profile 'DEFAULT'"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in output:\n%s", want, out)
		}
	}
}

func TestMaterializeAndValidate(t *testing.T) {
	dir := t.TempDir()
	planPath := filepath.Join(dir, "plan.json")
	var planOut bytes.Buffer
	var stderr bytes.Buffer
	code := Run([]string{
		"plan",
		"--service", "obp",
		"--issuer", "https://idcs-example.identity.oraclecloud.com",
		"--platform", "https://example-oabcs.blockchain.ocp.oraclecloud.com:7443/restproxy",
		"--resource-app-id", "resource-app-id",
		"--base-app-name", "example-obp_APPID",
		"--include", "jwt-service",
		"--role-preset", "obp-admin",
	}, &planOut, &stderr)
	if code != 0 {
		t.Fatalf("plan failed with %d: %s", code, stderr.String())
	}
	if err := os.WriteFile(planPath, planOut.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	var materializeOut bytes.Buffer
	outDir := filepath.Join(dir, "artifacts")
	code = Run([]string{
		"materialize",
		"--plan", planPath,
		"--out", outDir,
	}, &materializeOut, &stderr)
	if code != 0 {
		t.Fatalf("materialize failed with %d: %s", code, stderr.String())
	}
	for _, name := range []string{
		"plan.json",
		"example-obp-service-jwt.json",
		"example-obp-service-jwt-oauth-client-certificate.json",
		"example-obp-service-jwt-grant-admin.json",
		"apply.sh",
		"validate.sh",
		"cleanup.sh",
		"oci-context.handoff.json",
		"oci-context-token-services.yml",
		"oci-context-token-commands.sh",
	} {
		if _, err := os.Stat(filepath.Join(outDir, name)); err != nil {
			t.Fatalf("expected %s: %v", name, err)
		}
	}

	var validateOut bytes.Buffer
	code = Run([]string{
		"validate",
		"--plan", planPath,
		"--format", "text",
	}, &validateOut, &stderr)
	if code != 0 {
		t.Fatalf("validate failed with %d: %s", code, stderr.String())
	}
	if !strings.Contains(validateOut.String(), "placeholder <ADMIN-app-role-id>") {
		t.Fatalf("expected placeholder warning in validate output:\n%s", validateOut.String())
	}

	var applyOut bytes.Buffer
	applyDir := filepath.Join(dir, "apply-artifacts")
	code = Run([]string{
		"apply",
		"--plan", planPath,
		"--out", applyDir,
	}, &applyOut, &stderr)
	if code != 0 {
		t.Fatalf("apply failed with %d: %s", code, stderr.String())
	}
	if !strings.Contains(applyOut.String(), "dry-run apply artifacts") {
		t.Fatalf("unexpected apply output: %s", applyOut.String())
	}

	code = Run([]string{
		"apply",
		"--plan", planPath,
		"--out", applyDir,
		"--execute",
	}, &applyOut, &stderr)
	if code == 0 {
		t.Fatal("expected execute mode to fail closed")
	}
}

func TestHandoffOCIContextYAML(t *testing.T) {
	dir := t.TempDir()
	planPath := filepath.Join(dir, "plan.json")
	var planOut bytes.Buffer
	var stderr bytes.Buffer
	code := Run([]string{
		"plan",
		"--service", "obp",
		"--issuer", "https://idcs-example.identity.oraclecloud.com",
		"--platform", "https://example-oabcs.blockchain.ocp.oraclecloud.com:7443/restproxy",
		"--resource-app-id", "resource-app-id",
		"--base-app-name", "example-obp_APPID",
		"--include", "user,jwt-service",
		"--app-role-grants", "ADMIN=admin-role-id",
	}, &planOut, &stderr)
	if code != 0 {
		t.Fatalf("plan failed with %d: %s", code, stderr.String())
	}
	if err := os.WriteFile(planPath, planOut.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	var handoffOut bytes.Buffer
	code = Run([]string{
		"handoff",
		"--plan", planPath,
		"--target", "oci-context",
		"--format", "yaml",
	}, &handoffOut, &stderr)
	if code != 0 {
		t.Fatalf("handoff failed with %d: %s", code, stderr.String())
	}
	out := handoffOut.String()
	for _, want := range []string{
		"token_services:",
		"name: 'obp'",
		"flow: 'authorization-code'",
		"name: 'obp-jwt-service'",
		"flow: 'jwt-client-credentials'",
		"jwt_audience: 'https://identity.oraclecloud.com/'",
		"private_key_file_env:",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in handoff output:\n%s", want, out)
		}
	}

	var commandsOut bytes.Buffer
	code = Run([]string{
		"handoff",
		"--plan", planPath,
		"--target", "oci-context",
		"--format", "commands",
	}, &commandsOut, &stderr)
	if code != 0 {
		t.Fatalf("handoff commands failed with %d: %s", code, stderr.String())
	}
	commands := commandsOut.String()
	if strings.Contains(commands, "--flow 'authorization-code' --issuer") &&
		strings.Contains(strings.Split(commands, "--flow 'authorization-code'")[1], "--no-login") &&
		strings.Index(commands, "--no-login") < strings.Index(commands, "--flow 'jwt-client-credentials'") {
		t.Fatalf("authorization-code command should not include --no-login:\n%s", commands)
	}
	if !strings.Contains(commands, "--flow 'jwt-client-credentials'") || !strings.Contains(commands, "--no-login") {
		t.Fatalf("jwt-client-credentials command should include --no-login:\n%s", commands)
	}
}
