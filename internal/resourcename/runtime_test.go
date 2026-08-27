package resourcename

import "testing"

func TestRuntimeResourceNames(t *testing.T) {
	tests := map[string]struct {
		got  string
		want string
	}{
		"project namespace":  {got: ProjectNamespace("prj_c119e462fb7c5eed20ec4ca4"), want: "ns-c119e462fb"},
		"deployment target":  {got: DeploymentTarget("dplt_b530527f18113463aa3bf8a7"), want: "dplt-b530527f18"},
		"invalid characters": {got: FromID("Hook", "run_ABC_123"), want: "hook-abc-123"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if test.got != test.want {
				t.Fatalf("resource name = %q, want %q", test.got, test.want)
			}
		})
	}
}
