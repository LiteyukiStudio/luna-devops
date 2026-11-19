package aitool

import (
	"encoding/json"
	"slices"
	"testing"
	"time"
)

type sampleSecretItem struct {
	Key   string  `json:"key" desc:"密钥键名"`
	Value *string `json:"value,omitempty" sensitive:"true" desc:"密钥明文"`
}

type sampleInput struct {
	Project         string             `json:"project" desc:"项目 ID 或 identifier"`
	Stage           *string            `json:"stage,omitempty" enum:"dev|test|staging|prod" desc:"目标阶段"`
	PageSize        *int               `json:"pageSize,omitempty" min:"1" max:"50" default:"20"`
	SecretValues    map[string]string  `json:"secretValues,omitempty" sensitive:"true" desc:"敏感键值字典"`
	SecretMutations []sampleSecretItem `json:"secretMutations,omitempty" max:"20"`
}

func TestSchemaForInputGeneratesStrictSchemaAndSensitivePaths(t *testing.T) {
	schema, sensitive := SchemaForInput[sampleInput]()
	if schema["type"] != "object" || schema["additionalProperties"] != false {
		t.Fatalf("unexpected top-level schema: %#v", schema)
	}
	required, _ := schema["required"].([]any)
	if len(required) != 1 || required[0] != "project" {
		t.Fatalf("unexpected required fields: %#v", required)
	}
	if !slices.Equal(sensitive, []string{"secretMutations.*.value", "secretValues.*"}) {
		t.Fatalf("unexpected sensitivePaths: %#v", sensitive)
	}
	props := schema["properties"].(map[string]any)
	secretValuesProp := props["secretValues"].(map[string]any)
	valSchema := secretValuesProp["additionalProperties"].(map[string]any)
	if valSchema["writeOnly"] != true || valSchema["x-luna-sensitive"] != true {
		t.Fatalf("expected map additionalProperties to carry writeOnly & x-luna-sensitive, got %#v", valSchema)
	}
}

func TestPreviewLockSignAndVerifyWithSecretRotation(t *testing.T) {
	now := time.Unix(1700000000, 0)
	oldSecret := []byte("old-secret-key-0123456789abcdef")
	newSecret := []byte("new-secret-key-0123456789abcdef")

	argsRaw := json.RawMessage(`{"target":"prod","project":"prj_1","note":"a<b&c"}`)
	argsHash, err := CanonicalJSONHash(argsRaw)
	if err != nil {
		t.Fatalf("CanonicalJSONHash: %v", err)
	}

	claims := PreviewLockClaims{
		RunID:                 "airun_1",
		ToolCallID:            "aitool_1",
		OperationID:           "delivery_create_release",
		ArgumentsHash:         argsHash,
		OwnerUserID:           "usr_1",
		ConversationProjectID: "prj_1",
		IssuedAt:              now.Unix(),
		ExpiresAt:             now.Add(15 * time.Minute).Unix(),
	}
	token, err := SignPreviewLock(oldSecret, claims, map[string]string{"resolvedImageRef": "harbor.local/app:v2"})
	if err != nil {
		t.Fatalf("SignPreviewLock: %v", err)
	}

	lockedRaw, err := VerifyPreviewLock([][]byte{newSecret, oldSecret}, token, claims, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("VerifyPreviewLock rotated secret: %v", err)
	}
	var locked map[string]string
	if err := json.Unmarshal(lockedRaw, &locked); err != nil || locked["resolvedImageRef"] != "harbor.local/app:v2" {
		t.Fatalf("unexpected locked payload: %s (%v)", string(lockedRaw), err)
	}

	// Tampered arguments hash must fail
	tampered := claims
	tampered.ArgumentsHash = "sha256:0000"
	if _, err := VerifyPreviewLock([][]byte{newSecret, oldSecret}, token, tampered, now.Add(time.Minute)); err != ErrPreviewLockMismatch {
		t.Fatalf("expected ErrPreviewLockMismatch, got %v", err)
	}
}

func TestCanonicalJSONHashMatchesUnsortedKeys(t *testing.T) {
	h1, err := CanonicalJSONHash(json.RawMessage(`{"b":1,"a":"x<y","c":[true,null]}`))
	if err != nil {
		t.Fatalf("h1 err: %v", err)
	}
	h2, err := CanonicalJSONHash(json.RawMessage(`{"c":[true,null],"a":"x<y","b":1.0}`))
	if err != nil {
		t.Fatalf("h2 err: %v", err)
	}
	if h1 != h2 {
		t.Fatalf("expected canonical hashes to match: %s vs %s", h1, h2)
	}
}

func TestRegistryEnforcesL2PreviewAndCrossReferences(t *testing.T) {
	reg := NewRegistry()
	Register(reg, Spec[struct {
		Project string `json:"project"`
	}, map[string]any]{
		Name:           "delivery_get_target",
		Domain:         DomainDelivery,
		Summary:        "查看部署目标详情",
		ApprovalTier:   ApprovalTierL0,
		RequiredScopes: []string{"applications:read"},
		Execute: func(_ *ToolContext, in struct {
			Project string `json:"project"`
		}) (map[string]any, error) {
			return map[string]any{"projectId": in.Project}, nil
		},
	})
	Register(reg, Spec[sampleInput, map[string]any]{
		Name:           "delivery_update_target_config",
		Domain:         DomainDelivery,
		Summary:        "更新部署目标配置",
		ApprovalTier:   ApprovalTierL2,
		RequiredScopes: []string{"applications:write"},
		Verification: []OperationVerification{
			{OperationID: "delivery_get_target", Family: "deployment_target", Expect: "present"},
		},
		Preview: func(_ *ToolContext, _ sampleInput) (*PreviewResult, error) {
			return &PreviewResult{Preview: ApprovalPreview{Summary: "preview ok"}}, nil
		},
		Execute: func(_ *ToolContext, in sampleInput) (map[string]any, error) {
			return map[string]any{"projectId": in.Project}, nil
		},
	})
	if err := reg.ValidateCrossReferences(); err != nil {
		t.Fatalf("ValidateCrossReferences: %v", err)
	}
	tokens := SplitOperationIdentifier("delivery_update_target_config")
	if !slices.Equal(tokens, []string{"delivery", "update", "target", "config"}) {
		t.Fatalf("unexpected tokens: %#v", tokens)
	}
}
