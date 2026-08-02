package resourceidentifier

import "testing"

func TestValidate(t *testing.T) {
	t.Parallel()

	for _, value := range []string{"ab", "account-api", "prod2"} {
		if err := Validate(value, 2, 24); err != nil {
			t.Fatalf("Validate(%q) returned %v", value, err)
		}
	}
	for _, value := range []string{"a", "-api", "api-", "API", "api_test", "api.example"} {
		if err := Validate(value, 2, 24); err == nil {
			t.Fatalf("Validate(%q) succeeded", value)
		}
	}
}

func TestReadableNamesStayWithinDNSLabelLimit(t *testing.T) {
	t.Parallel()

	project := "abcdefghijklmnopqrstuv"
	application := "abcdefghijklmnopqrstuv"
	stage := "abcdefghijkl"
	if got := ProjectNamespace(project); len(got) > 63 {
		t.Fatalf("project namespace is %d characters: %q", len(got), got)
	}
	if got := DeploymentTargetName(application, stage); len(got) > 63 {
		t.Fatalf("deployment target name is %d characters: %q", len(got), got)
	}
}

func TestReadableKubernetesNames(t *testing.T) {
	t.Parallel()

	if got := ProjectNamespace("payments"); got != "luna-payments" {
		t.Fatalf("unexpected project namespace %q", got)
	}
	if got := DeploymentTargetName("api", "prod"); got != "luna-api-prod" {
		t.Fatalf("unexpected deployment target name %q", got)
	}
}
