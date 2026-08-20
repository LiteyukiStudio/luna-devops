package aitool

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

var (
	agentToolActions     = stringSet("discover", "read", "create", "update", "delete", "execute", "verify")
	agentToolSideEffects = stringSet("none", "external-read", "external-write", "platform-write", "destructive")
	agentToolRisks       = stringSet("low", "medium", "high", "critical")
	agentToolApprovals   = stringSet("never", "always")
)

func parseAgentToolContract(operationID string, raw map[string]any) (AgentToolContract, error) {
	encoded, err := json.Marshal(raw)
	if err != nil {
		return AgentToolContract{}, fmt.Errorf("operation %s has invalid x-luna-agent metadata: %w", operationID, err)
	}
	for _, field := range []string{"allowed", "resourceTypes", "action", "sideEffect", "idempotent", "replaySafe", "risk", "approval", "intents", "useWhen", "successEvidence", "verification"} {
		if _, exists := raw[field]; !exists {
			return AgentToolContract{}, fmt.Errorf("operation %s has invalid x-luna-agent.%s: field is required", operationID, field)
		}
	}
	var contract AgentToolContract
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&contract); err != nil {
		return AgentToolContract{}, fmt.Errorf("operation %s has invalid x-luna-agent metadata: %w", operationID, err)
	}
	if err := validateAgentToolContract(operationID, contract); err != nil {
		return AgentToolContract{}, err
	}
	return contract, nil
}

func validateAgentToolContract(operationID string, contract AgentToolContract) error {
	fail := func(field, detail string) error {
		return fmt.Errorf("operation %s has invalid x-luna-agent.%s: %s", operationID, field, detail)
	}
	if !contract.Allowed {
		return fail("allowed", "must be explicitly true")
	}
	for field, values := range map[string][]string{
		"resourceTypes":   contract.ResourceTypes,
		"intents":         contract.Intents,
		"useWhen":         contract.UseWhen,
		"successEvidence": contract.SuccessEvidence,
	} {
		if !nonEmptyStrings(values) {
			return fail(field, "must contain at least one non-empty value")
		}
	}
	if _, ok := agentToolActions[contract.Action]; !ok {
		return fail("action", "unsupported value")
	}
	if _, ok := agentToolSideEffects[contract.SideEffect]; !ok {
		return fail("sideEffect", "unsupported value")
	}
	if _, ok := agentToolRisks[contract.Risk]; !ok {
		return fail("risk", "unsupported value")
	}
	if _, ok := agentToolApprovals[contract.Approval]; !ok {
		return fail("approval", "unsupported value")
	}
	if (contract.Risk == "high" || contract.Risk == "critical") && contract.Approval != "always" {
		return fail("approval", "high and critical risk tools require approval=always")
	}
	if contract.SideEffect == "external-write" || contract.SideEffect == "platform-write" || contract.SideEffect == "destructive" {
		if !nonEmptyStrings(contract.AvoidWhen) {
			return fail("avoidWhen", "write tools must state a negative boundary")
		}
		if !nonEmptyStrings(contract.Prerequisites) {
			return fail("prerequisites", "write tools must state prerequisites")
		}
	}
	if err := validateAgentToolVerification(contract); err != nil {
		return fail("verification", err.Error())
	}
	return nil
}

func validateAgentToolVerification(contract AgentToolContract) error {
	verification := contract.Verification
	switch verification.Mode {
	case "response":
		if len(verification.SuccessCodes) == 0 {
			return fmt.Errorf("response verification requires successCodes")
		}
		for _, code := range verification.SuccessCodes {
			if code < 100 || code > 599 {
				return fmt.Errorf("success code %d is outside the HTTP range", code)
			}
		}
		if verification.OperationID != "" || verification.IDSource != "" || len(verification.ArgumentBindings) > 0 || verification.Completion != nil {
			return fmt.Errorf("response verification cannot declare readback fields")
		}
	case "readback", "async-readback":
		if contract.SideEffect == "none" {
			return fmt.Errorf("sideEffect=none cannot use readback verification")
		}
		if strings.TrimSpace(verification.OperationID) == "" || !validJSONPointer(verification.IDSource) || len(verification.ArgumentBindings) == 0 || verification.Completion == nil {
			return fmt.Errorf("readback verification requires operationId, idSource, argumentBindings, and completion")
		}
		for argument, pointer := range verification.ArgumentBindings {
			if strings.TrimSpace(argument) == "" || !validJSONPointer(pointer) {
				return fmt.Errorf("argumentBindings must map non-empty argument names to JSON pointers")
			}
		}
		switch verification.Completion.Mode {
		case "readback-success":
			if verification.Completion.Path != "" || len(verification.Completion.SuccessStates) > 0 || len(verification.Completion.PendingStates) > 0 || len(verification.Completion.FailureStates) > 0 {
				return fmt.Errorf("readback-success completion cannot declare state fields")
			}
		case "state":
			if !validJSONPointer(verification.Completion.Path) || !nonEmptyStrings(verification.Completion.SuccessStates) {
				return fmt.Errorf("state completion requires path and successStates")
			}
		default:
			return fmt.Errorf("unsupported completion mode")
		}
	default:
		return fmt.Errorf("unsupported mode")
	}
	return nil
}

func validateAgentContractReferences(operations []OpenAPIOperation) error {
	known := make(map[string]OpenAPIOperation, len(operations))
	for _, operation := range operations {
		known[operation.OperationID] = operation
	}
	for _, operation := range operations {
		if err := validateAgentOperationContract(operation); err != nil {
			return err
		}
		references := append([]string{}, operation.Contract.Predecessors...)
		references = append(references, operation.Contract.Followups...)
		if operation.Contract.Verification.Mode != "response" {
			references = append(references, operation.Contract.Verification.OperationID)
		}
		for _, reference := range references {
			if _, ok := known[reference]; !ok {
				return fmt.Errorf("operation %s references non-Agent operation %q", operation.OperationID, reference)
			}
		}
		if operation.Contract.Verification.Mode != "response" {
			if err := validateReadbackPair(operation, known[operation.Contract.Verification.OperationID]); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateAgentOperationContract(operation OpenAPIOperation) error {
	contract := operation.Contract
	if operation.Idempotent != contract.Idempotent || operation.Approval != contract.Approval || operation.StepUpPurpose != contract.MFAPurpose {
		return fmt.Errorf("operation %s transport policy disagrees with its Agent contract", operation.OperationID)
	}
	if contract.ReplaySafe && !contract.Idempotent {
		return fmt.Errorf("operation %s declares replaySafe without idempotent", operation.OperationID)
	}
	if strings.EqualFold(operation.Method, http.MethodGet) && (contract.SideEffect == "external-write" || contract.SideEffect == "platform-write" || contract.SideEffect == "destructive") {
		return fmt.Errorf("operation %s declares a write side effect for GET", operation.OperationID)
	}
	if contract.Verification.Mode == "response" {
		for _, code := range contract.Verification.SuccessCodes {
			if _, ok := operation.ResponseSchemas[code]; !ok {
				return fmt.Errorf("operation %s verifies undeclared HTTP success code %d", operation.OperationID, code)
			}
		}
	}
	return nil
}

func validateReadbackPair(writer, verifier OpenAPIOperation) error {
	verification := writer.Contract.Verification
	if verifier.OperationID == "" {
		return fmt.Errorf("operation %s readback %q is unavailable", writer.OperationID, verification.OperationID)
	}
	if verifier.Contract.Verification.Mode != "response" || !verifier.Contract.Idempotent || !verifier.Contract.ReplaySafe {
		return fmt.Errorf("operation %s readback %s is not a replay-safe response verifier", writer.OperationID, verifier.OperationID)
	}
	if !containsString(writer.Contract.Followups, verifier.OperationID) || !containsString(verifier.Contract.Predecessors, writer.OperationID) {
		return fmt.Errorf("operation %s and readback %s do not declare a bidirectional workflow relation", writer.OperationID, verifier.OperationID)
	}
	verifierProperties := mapValue(verifier.InputSchema["properties"])
	for argument := range verification.ArgumentBindings {
		if _, ok := verifierProperties[argument]; !ok {
			return fmt.Errorf("operation %s binds unknown readback argument %s", writer.OperationID, argument)
		}
	}
	for _, required := range stringArray(verifier.InputSchema["required"]) {
		if _, ok := verification.ArgumentBindings[required]; !ok {
			return fmt.Errorf("operation %s does not bind required readback argument %s", writer.OperationID, required)
		}
	}
	writeSchemas := successResponseSchemas(writer)
	if len(writeSchemas) == 0 {
		return fmt.Errorf("operation %s has no typed 2xx response for readback binding", writer.OperationID)
	}
	for code, schema := range writeSchemas {
		if !schemaHasJSONPointer(schema, verification.IDSource) {
			return fmt.Errorf("operation %s response %d does not define idSource %s", writer.OperationID, code, verification.IDSource)
		}
		for argument, pointer := range verification.ArgumentBindings {
			if !schemaHasJSONPointer(schema, pointer) {
				return fmt.Errorf("operation %s response %d does not define binding %s=%s", writer.OperationID, code, argument, pointer)
			}
		}
	}
	completion := verification.Completion
	if completion == nil || completion.Mode != "state" {
		return nil
	}
	states := append(append(append([]string{}, completion.PendingStates...), completion.SuccessStates...), completion.FailureStates...)
	if hasDuplicateString(states) {
		return fmt.Errorf("operation %s readback completion states overlap", writer.OperationID)
	}
	for _, code := range verifier.Contract.Verification.SuccessCodes {
		schema := verifier.ResponseSchemas[code]
		stateSchema, ok := schemaAtJSONPointer(schema, completion.Path)
		if !ok {
			return fmt.Errorf("operation %s readback %s response %d does not define completion path %s", writer.OperationID, verifier.OperationID, code, completion.Path)
		}
		if allowed := stringArray(stateSchema["enum"]); len(allowed) > 0 {
			for _, state := range states {
				if !containsString(allowed, state) {
					return fmt.Errorf("operation %s completion state %q is absent from %s response enum", writer.OperationID, state, verifier.OperationID)
				}
			}
		}
	}
	return nil
}

func successResponseSchemas(operation OpenAPIOperation) map[int]map[string]any {
	result := make(map[int]map[string]any)
	for code, schema := range operation.ResponseSchemas {
		if code >= http.StatusOK && code < http.StatusMultipleChoices && len(schema) > 0 {
			result[code] = schema
		}
	}
	return result
}

func schemaHasJSONPointer(schema map[string]any, pointer string) bool {
	_, ok := schemaAtJSONPointer(schema, pointer)
	return ok
}

func schemaAtJSONPointer(schema map[string]any, pointer string) (map[string]any, bool) {
	if !validJSONPointer(pointer) {
		return nil, false
	}
	current := schema
	for _, raw := range strings.Split(strings.TrimPrefix(pointer, "/"), "/") {
		segment := strings.ReplaceAll(strings.ReplaceAll(raw, "~1", "/"), "~0", "~")
		property, ok := schemaProperty(current, segment, 0)
		if !ok {
			return nil, false
		}
		current = property
	}
	return current, true
}

func schemaProperty(schema map[string]any, name string, depth int) (map[string]any, bool) {
	if depth > 20 {
		return nil, false
	}
	if property, ok := mapValue(schema["properties"])[name]; ok {
		return mapValue(property), true
	}
	for _, candidate := range arrayValue(schema["allOf"]) {
		if property, ok := schemaProperty(mapValue(candidate), name, depth+1); ok {
			return property, true
		}
	}
	return nil, false
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func hasDuplicateString(values []string) bool {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			return true
		}
		seen[value] = struct{}{}
	}
	return false
}

func validJSONPointer(value string) bool {
	return strings.HasPrefix(strings.TrimSpace(value), "/")
}

func nonEmptyStrings(values []string) bool {
	if len(values) == 0 {
		return false
	}
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			return false
		}
	}
	return true
}

func stringSet(values ...string) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		result[value] = struct{}{}
	}
	return result
}
