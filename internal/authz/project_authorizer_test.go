package authz

import (
	"context"
	"errors"
	"testing"
)

type projectRoleReaderStub struct {
	role  string
	err   error
	calls int
}

func (r *projectRoleReaderStub) ProjectRole(context.Context, string, string) (string, error) {
	r.calls++
	return r.role, r.err
}

func TestProjectPolicyRoleMatrix(t *testing.T) {
	tests := []struct {
		action  Action
		allowed []string
		denied  []string
	}{
		{ActionDeploymentRead, []string{ProjectRoleOwner, ProjectRoleAdmin, ProjectRoleDeveloper, ProjectRoleViewer}, nil},
		{ActionDeploymentUpdate, []string{ProjectRoleOwner, ProjectRoleAdmin, ProjectRoleDeveloper}, []string{ProjectRoleViewer}},
		{ActionDeploymentDelete, []string{ProjectRoleOwner, ProjectRoleAdmin}, []string{ProjectRoleDeveloper, ProjectRoleViewer}},
		{ActionGatewayDelete, []string{ProjectRoleOwner, ProjectRoleAdmin}, []string{ProjectRoleDeveloper, ProjectRoleViewer}},
	}

	for _, test := range tests {
		t.Run(string(test.action), func(t *testing.T) {
			if _, ok := ProjectPolicyForAction(test.action); !ok {
				t.Fatalf("policy for %s is missing", test.action)
			}
			for _, role := range test.allowed {
				if !ProjectRoleAllows(role, test.action) {
					t.Fatalf("expected %s to allow %s", role, test.action)
				}
			}
			for _, role := range test.denied {
				if ProjectRoleAllows(role, test.action) {
					t.Fatalf("expected %s to deny %s", role, test.action)
				}
			}
		})
	}
}

func TestProjectAuthorizerFailsClosed(t *testing.T) {
	tests := []struct {
		name    string
		reader  ProjectMembershipReader
		subject ProjectSubject
		project string
		action  Action
		wantErr error
	}{
		{
			name: "undefined action", reader: &projectRoleReaderStub{role: ProjectRoleOwner},
			subject: ProjectSubject{UserID: "usr_1", PlatformRole: PlatformRoleUser}, project: "prj_1",
			action: Action("unknown:action"), wantErr: ErrProjectPolicyUndefined,
		},
		{
			name: "empty user", reader: &projectRoleReaderStub{role: ProjectRoleOwner},
			subject: ProjectSubject{PlatformRole: PlatformRoleUser}, project: "prj_1",
			action: ActionProjectRead, wantErr: ErrProjectAccessDenied,
		},
		{
			name: "missing membership reader", reader: nil,
			subject: ProjectSubject{UserID: "usr_1", PlatformRole: PlatformRoleUser}, project: "prj_1",
			action: ActionProjectRead, wantErr: ErrProjectAuthorizationUnavailable,
		},
		{
			name: "membership lookup failure", reader: &projectRoleReaderStub{err: errors.New("database unavailable")},
			subject: ProjectSubject{UserID: "usr_1", PlatformRole: PlatformRoleUser}, project: "prj_1",
			action: ActionProjectRead, wantErr: ErrProjectAuthorizationUnavailable,
		},
		{
			name: "membership not found", reader: &projectRoleReaderStub{err: ErrProjectMembershipNotFound},
			subject: ProjectSubject{UserID: "usr_1", PlatformRole: PlatformRoleUser}, project: "prj_1",
			action: ActionProjectRead, wantErr: ErrProjectAccessDenied,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewProjectAuthorizer(test.reader).AuthorizeProject(context.Background(), test.subject, test.project, test.action)
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("AuthorizeProject() error = %v, want %v", err, test.wantErr)
			}
		})
	}
}

func TestProjectAuthorizerPlatformAdminBypassesMembership(t *testing.T) {
	reader := &projectRoleReaderStub{err: errors.New("must not be called")}
	authorizer := NewProjectAuthorizer(reader)

	access, err := authorizer.AuthorizeProject(context.Background(), ProjectSubject{
		UserID: "usr_admin", PlatformRole: PlatformRoleAdmin,
	}, "prj_1", ActionDeploymentDelete)
	if err != nil || !access.PlatformAdmin || reader.calls != 0 {
		t.Fatalf("platform admin access = %#v, err=%v, membership calls=%d", access, err, reader.calls)
	}
}

func TestProjectPolicyForActionReturnsDefensiveCopy(t *testing.T) {
	policy, ok := ProjectPolicyForAction(ActionDeploymentDelete)
	if !ok {
		t.Fatal("deployment delete policy is missing")
	}
	policy.AllowedRoles[0] = ProjectRoleViewer
	if ProjectRoleAllows(ProjectRoleViewer, ActionDeploymentDelete) {
		t.Fatal("caller mutation changed the authorization policy")
	}
}
