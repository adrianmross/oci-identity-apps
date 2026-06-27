package cli

import (
	"bytes"
	"encoding/json"
	"testing"
)

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
	if payload["schemaVersion"] != "oci-identity-apps.plan.v1" {
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
