package aiagent

import (
	"testing"
	"time"
)

func TestRunGrantAndDelegationAudiencesCannotBeInterchanged(t *testing.T) {
	now := time.Unix(1000, 0)
	grant, err := SignRunActorGrant(RunActorGrant{
		Audience: "luna-ai-run-grant", Purpose: "agent_delegation_exchange",
		RunID: "airun_1", UserID: "usr_1", SessionID: "sess_1", IssuedAt: 1000, ExpiresAt: 2000,
	}, "run-grant-key")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyRunActorGrant(grant, "run-grant-key", now); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyDelegationToken(grant, "run-grant-key", now); err == nil {
		t.Fatal("Run Actor Grant must not be accepted as a business delegation token")
	}

	delegation, err := SignDelegationToken(DelegationClaims{
		Audience: "luna-api-ai-tools", Purpose: "execute_registered_tool",
		RunID: "airun_1", ToolCallID: "aitool_1", OperationID: "getDashboard",
		UserID: "usr_1", SessionID: "sess_1", IssuedAt: 1000, ExpiresAt: 1060,
	}, "delegation-key")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyDelegationToken(delegation, "delegation-key", now); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyRunActorGrant(delegation, "delegation-key", now); err == nil {
		t.Fatal("delegation token must not be accepted as a Run Actor Grant")
	}
}
