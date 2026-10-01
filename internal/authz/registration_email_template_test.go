package authz

import "testing"

func TestRegistrationEmailTemplateExactRBAC(t *testing.T) {
	svc := setupAuthzServiceTest(t)
	if err := svc.BootstrapBuiltinRoles(); err != nil {
		t.Fatal(err)
	}
	path := "/api/v1/admin/settings/registration-email-template"
	for _, role := range []string{"system_admin", "operations", "support", "readonly_auditor"} {
		if err := svc.SetAdminRoles(92, []string{role}); err != nil {
			t.Fatal(err)
		}
		for _, request := range []struct {
			path, method string
			allowed      bool
		}{
			{path, "GET", true}, {path, "PUT", true}, {path + "/defaults", "GET", true},
			{path, "POST", false}, {path, "DELETE", false}, {path + "/reset", "POST", false}, {path + "/defaults", "PUT", false},
		} {
			allow, err := svc.EnforceAdmin(92, request.path, request.method)
			if err != nil || allow != (role == "system_admin" && request.allowed) {
				t.Errorf("%s %s %s: allow=%v err=%v", role, request.method, request.path, allow, err)
			}
		}
	}
}
