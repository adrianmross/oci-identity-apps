package planner

import "testing"

func TestBuildGenericPlan(t *testing.T) {
	plan, err := Build(Options{
		Service:      ServiceGeneric,
		Issuer:       "https://idcs-example.identity.oraclecloud.com/oauth2/v1",
		Scope:        "https://service.example.com/.default",
		AppPrefix:    "Example Service",
		IDCSEndpoint: "https://idcs-example.identity.oraclecloud.com/admin/v1",
		Include:      []AppKind{AppUser, AppService, AppJWT},
	})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	if plan.SchemaVersion != SchemaVersion {
		t.Fatalf("unexpected schema version: %s", plan.SchemaVersion)
	}
	if plan.Target.IDCSEndpoint != "https://idcs-example.identity.oraclecloud.com" {
		t.Fatalf("unexpected endpoint: %s", plan.Target.IDCSEndpoint)
	}
	if plan.Target.IDCSAdminURL != "https://idcs-example.identity.oraclecloud.com/admin/v1/" {
		t.Fatalf("unexpected admin URL: %s", plan.Target.IDCSAdminURL)
	}
	if got := plan.Target.TemplateIDs[AppUser]; got != DefaultPublicAppTemplateID {
		t.Fatalf("unexpected user template: %s", got)
	}
	if got := plan.Target.TemplateIDs[AppService]; got != DefaultWebAppTemplateID {
		t.Fatalf("unexpected service template: %s", got)
	}
	if len(plan.Apps) != 3 {
		t.Fatalf("expected 3 apps, got %d", len(plan.Apps))
	}
	user := plan.Apps[0]
	if user.Name != "example-service-cli-user" {
		t.Fatalf("unexpected user app name: %s", user.Name)
	}
	if user.OCICreatePayload.ClientType != ClientPublic {
		t.Fatalf("unexpected user client type: %s", user.OCICreatePayload.ClientType)
	}
	if user.OCICreatePayload.BasedOnTemplate.Value != DefaultPublicAppTemplateID {
		t.Fatalf("unexpected user payload template: %s", user.OCICreatePayload.BasedOnTemplate.Value)
	}
	if user.OCICreatePayload.AllowedGrants[0] != AuthorizationCodeGrant {
		t.Fatalf("unexpected user grant: %v", user.OCICreatePayload.AllowedGrants)
	}
}

func TestBuildOBPPlanDefaultsScopeToPlatform(t *testing.T) {
	plan, err := Build(Options{
		Service:     ServiceOBP,
		Issuer:      "https://idcs-example.identity.oraclecloud.com",
		Platform:    "https://example-oabcs.blockchain.ocp.oraclecloud.com:7443/restproxy",
		BaseAppName: "example-obp_APPID",
		Include:     []AppKind{AppUser},
	})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	if plan.Target.Scope != "https://example-oabcs.blockchain.ocp.oraclecloud.com:7443/restproxy" {
		t.Fatalf("unexpected scope: %s", plan.Target.Scope)
	}
	if plan.Apps[0].Name != "example-obp-cli-user" {
		t.Fatalf("unexpected app name: %s", plan.Apps[0].Name)
	}
	if len(plan.BaseCloudServiceApp.ExpectedBehavior) == 0 {
		t.Fatal("expected OBP CloudGate behavior notes")
	}
}

func TestBuildConfidentialUserAndTemplateOverrides(t *testing.T) {
	plan, err := Build(Options{
		Issuer:             "https://idcs.example.com",
		Scope:              "scope",
		AppPrefix:          "custom",
		UserClientType:     ClientConfidential,
		Include:            []AppKind{AppUser},
		UserTemplateID:     "CustomConfidentialTemplate",
		RefreshTokenExpiry: 604800,
	})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	user := plan.Apps[0]
	if user.ClientType != ClientConfidential {
		t.Fatalf("unexpected user client type: %s", user.ClientType)
	}
	if user.OCICreatePayload.BasedOnTemplate.Value != "CustomConfidentialTemplate" {
		t.Fatalf("unexpected template: %s", user.OCICreatePayload.BasedOnTemplate.Value)
	}
	if user.OCICreatePayload.RefreshTokenExpiry == nil || *user.OCICreatePayload.RefreshTokenExpiry != 604800 {
		t.Fatalf("unexpected refresh token expiry: %#v", user.OCICreatePayload.RefreshTokenExpiry)
	}
}

func TestParseIncludes(t *testing.T) {
	includes, err := ParseIncludes("service,jwt,service")
	if err != nil {
		t.Fatalf("ParseIncludes returned error: %v", err)
	}
	if len(includes) != 2 || includes[0] != AppService || includes[1] != AppJWT {
		t.Fatalf("unexpected includes: %#v", includes)
	}
	if _, err := ParseIncludes("cloudgate"); err == nil {
		t.Fatal("expected unsupported include error")
	}
}

func TestBuildRequiresScopeForGeneric(t *testing.T) {
	if _, err := Build(Options{Issuer: "https://idcs.example.com"}); err == nil {
		t.Fatal("expected missing scope error")
	}
}
