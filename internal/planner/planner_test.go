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
	if len(plan.Apps) != 5 {
		t.Fatalf("expected 5 apps, got %d", len(plan.Apps))
	}
	if plan.Target.PrincipalMode != PrincipalNone {
		t.Fatalf("generic service should default to no principal user, got %s", plan.Target.PrincipalMode)
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
	if user.OCICreatePayload.AllURLSchemes == nil || !*user.OCICreatePayload.AllURLSchemes {
		t.Fatal("expected user app payload to allow the loopback http redirect scheme")
	}
	jwtService := plan.Apps[2]
	if jwtService.Key != AppJWTService || jwtService.Name != "example-service-service-jwt" {
		t.Fatalf("unexpected service JWT app: %+v", jwtService)
	}
	if jwtService.OCICreatePayload.AllowedGrants[0] != ClientCredentialsGrant {
		t.Fatalf("unexpected service JWT grant: %v", jwtService.OCICreatePayload.AllowedGrants)
	}
	jwtUser := plan.Apps[3]
	if jwtUser.Key != AppJWTUser || jwtUser.OCICreatePayload.AllowedGrants[0] != JWTBearerGrant {
		t.Fatalf("unexpected user JWT app: %+v", jwtUser)
	}
	workload := plan.Apps[4]
	if workload.Key != AppWorkload || workload.OCICreatePayload.AllowedGrants[0] != TokenExchangeGrant {
		t.Fatalf("unexpected workload app: %+v", workload)
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

func TestBuildOBPPlanIncludesResourceAppRolesAndJWTCertificate(t *testing.T) {
	plan, err := Build(Options{
		Service:       ServiceOBP,
		Issuer:        "https://idcs-example.identity.oraclecloud.com",
		Platform:      "https://example-oabcs.blockchain.ocp.oraclecloud.com:7443/restproxy",
		ResourceAppID: "resource-app-id",
		BaseAppName:   "example-obp_APPID",
		Include:       []AppKind{AppJWTService},
		AppRoleGrants: []AppRoleGrant{
			{DisplayName: "ADMIN", ID: "admin-role-id"},
			{DisplayName: "REST_CLIENT", ID: "rest-role-id"},
		},
	})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	if plan.Target.ResourceAppID != "resource-app-id" {
		t.Fatalf("unexpected resource app id: %s", plan.Target.ResourceAppID)
	}
	if len(plan.Apps) != 1 {
		t.Fatalf("expected one app, got %d", len(plan.Apps))
	}
	app := plan.Apps[0]
	if app.OCICreatePayload.AllowedScopes[0].IDOfDefiningApp != "resource-app-id" {
		t.Fatalf("missing idOfDefiningApp: %#v", app.OCICreatePayload.AllowedScopes[0])
	}
	if len(app.OCICreatePayload.Certificates) != 1 {
		t.Fatalf("expected certificate reference: %#v", app.OCICreatePayload.Certificates)
	}
	if len(app.OCIPreCreate) != 1 {
		t.Fatalf("expected certificate pre-create action, got %d", len(app.OCIPreCreate))
	}
	certPayload, ok := app.OCIPreCreate[0].Payload.(OAuthClientCertificateInput)
	if !ok {
		t.Fatalf("unexpected cert payload type: %#v", app.OCIPreCreate[0].Payload)
	}
	if certPayload.CertificateAlias != "example-obp-service-jwt-cert" {
		t.Fatalf("unexpected cert alias: %s", certPayload.CertificateAlias)
	}
	if app.Principal == nil {
		t.Fatal("expected OBP jwt-service to plan a same-name principal user")
	}
	if app.Principal.UserName != "example-obp-service-jwt" {
		t.Fatalf("unexpected principal user name: %s", app.Principal.UserName)
	}
	if len(app.OCIPostCreate) != 5 {
		t.Fatalf("expected app-role grants, principal user, and user grants, got %d", len(app.OCIPostCreate))
	}
	grantPayload, ok := app.OCIPostCreate[0].Payload.(GrantInput)
	if !ok {
		t.Fatalf("unexpected grant payload type: %#v", app.OCIPostCreate[0].Payload)
	}
	if grantPayload.App.Value != "resource-app-id" {
		t.Fatalf("unexpected grant app: %#v", grantPayload.App)
	}
	if grantPayload.Entitlement.AttributeValue != "admin-role-id" {
		t.Fatalf("unexpected grant entitlement: %#v", grantPayload.Entitlement)
	}
	if grantPayload.Grantee.Value != "<created-app-id>" {
		t.Fatalf("expected created app placeholder, got %#v", grantPayload.Grantee)
	}
	userPayload, ok := app.OCIPostCreate[2].Payload.(UserCreateInput)
	if !ok {
		t.Fatalf("unexpected principal user payload type: %#v", app.OCIPostCreate[2].Payload)
	}
	if userPayload.UserName != app.Name {
		t.Fatalf("expected same-name user, got %#v", userPayload)
	}
	userGrantPayload, ok := app.OCIPostCreate[3].Payload.(GrantInput)
	if !ok {
		t.Fatalf("unexpected user grant payload type: %#v", app.OCIPostCreate[3].Payload)
	}
	if userGrantPayload.GrantMechanism != AdministratorToUserGrant || userGrantPayload.Grantee.Type != "User" {
		t.Fatalf("expected user grant, got %#v", userGrantPayload)
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
	if len(includes) != 4 ||
		includes[0] != AppService ||
		includes[1] != AppJWTService ||
		includes[2] != AppJWTUser ||
		includes[3] != AppWorkload {
		t.Fatalf("unexpected includes: %#v", includes)
	}
	if _, err := ParseIncludes("cloudgate"); err == nil {
		t.Fatal("expected unsupported include error")
	}
}

func TestPrincipalModeExplicitForGenericService(t *testing.T) {
	plan, err := Build(Options{
		Service:       ServiceGeneric,
		Issuer:        "https://idcs.example.com",
		Scope:         "https://service.example.com/.default",
		ResourceAppID: "resource-app-id",
		AppPrefix:     "example",
		Include:       []AppKind{AppService},
		PrincipalMode: PrincipalSameNameUser,
		AppRoleGrants: []AppRoleGrant{{DisplayName: "SERVICE_ADMIN", ID: "service-admin-role-id"}},
	})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	app := plan.Apps[0]
	if app.Principal == nil || app.Principal.Mode != PrincipalSameNameUser {
		t.Fatalf("expected same-name principal: %#v", app.Principal)
	}
	if len(app.OCIPostCreate) != 3 {
		t.Fatalf("expected app grant, user create, and user grant, got %d", len(app.OCIPostCreate))
	}
}

func TestParseAppRoleGrants(t *testing.T) {
	grants, err := ParseAppRoleGrants("ADMIN=admin-role-id, REST_CLIENT=rest-role-id,admin-role-id")
	if err != nil {
		t.Fatalf("ParseAppRoleGrants returned error: %v", err)
	}
	if len(grants) != 2 {
		t.Fatalf("unexpected grants: %#v", grants)
	}
	if grants[0].DisplayName != "ADMIN" || grants[0].ID != "admin-role-id" {
		t.Fatalf("unexpected first grant: %#v", grants[0])
	}
}

func TestRolePresetCanBeOverriddenByCustomGrants(t *testing.T) {
	plan, err := Build(Options{
		Issuer:        "https://idcs.example.com",
		Scope:         "https://service.example.com/.default",
		ResourceAppID: "resource-app-id",
		AppPrefix:     "example",
		Include:       []AppKind{AppService},
		RolePresets:   []RolePreset{RolePresetOBPAdmin},
		AppRoleGrants: []AppRoleGrant{
			{DisplayName: "ADMIN", ID: "real-admin-role-id"},
			{DisplayName: "REST_CLIENT", ID: "real-rest-role-id"},
		},
	})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	if len(plan.Apps) != 1 {
		t.Fatalf("unexpected apps: %#v", plan.Apps)
	}
	actions := plan.Apps[0].OCIPostCreate
	if len(actions) != 2 {
		t.Fatalf("expected two grants, got %d", len(actions))
	}
	first := actions[0].Payload.(GrantInput)
	second := actions[1].Payload.(GrantInput)
	if first.Entitlement.AttributeValue != "real-admin-role-id" {
		t.Fatalf("unexpected admin role id: %#v", first.Entitlement)
	}
	if second.Entitlement.AttributeValue != "real-rest-role-id" {
		t.Fatalf("unexpected rest role id: %#v", second.Entitlement)
	}
}

func TestBuildRequiresScopeForGeneric(t *testing.T) {
	if _, err := Build(Options{Issuer: "https://idcs.example.com"}); err == nil {
		t.Fatal("expected missing scope error")
	}
}
