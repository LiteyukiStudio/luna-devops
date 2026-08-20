package aiagent

import (
	"testing"
	"time"
)

func TestRunGrantAndDelegationAudiencesCannotBeInterchanged(t *testing.T) {
	now := time.Unix(1000, 0)
	grant, err := SignRunActorGrant(RunActorGrant{
		Audience: "luna-ai-run-grant", Purpose: "agent_delegation_exchange",
		RunID: "airun_1", ConversationID: "aicnv_1", UserID: "usr_1", SessionID: "sess_1", IssuedAt: 1000, ExpiresAt: 2000,
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
		RunID: "airun_1", ConversationID: "aicnv_1", ToolCallID: "aitool_1", OperationID: "getDashboard",
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

func TestConversationAuthorizationGrantIsBoundAndShortLived(t *testing.T) {
	now := time.Unix(1000, 0)
	token, err := SignConversationAuthorizationGrant(ConversationAuthorizationGrant{
		Audience: "luna-ai-conversation-authorization", Purpose: "approve_conversation_tools",
		GrantID: "aicag_1", ConversationID: "aicnv_1", UserID: "usr_1", SessionID: "sess_1",
		StepUpAssertionID: "mfa_1", IssuedAt: now.Unix(), ExpiresAt: now.Add(time.Hour).Unix(),
	}, "conversation-key")
	if err != nil {
		t.Fatal(err)
	}
	claims, err := VerifyConversationAuthorizationGrant(token, "conversation-key", now)
	if err != nil || claims.ConversationID != "aicnv_1" || claims.SessionID != "sess_1" {
		t.Fatalf("unexpected conversation grant: %#v, %v", claims, err)
	}
	if _, err := VerifyConversationAuthorizationGrant(token, "conversation-key", now.Add(time.Hour)); err == nil {
		t.Fatal("expired conversation authorization must fail closed")
	}
	if _, err := VerifyConversationAuthorizationGrant(token, "another-key", now); err == nil {
		t.Fatal("conversation authorization signed by another key must be rejected")
	}
}
