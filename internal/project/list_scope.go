package project

import (
	"errors"
	"strings"
)

type ListScope string

const (
	ListScopeRelated ListScope = "related"
	ListScopeAll     ListScope = "all"
)

var (
	ErrListScopeInvalid   = errors.New("project list scope is invalid")
	ErrListScopeForbidden = errors.New("project list scope requires platform administrator")
)

// ResolveListScope keeps project discovery related to the caller by default.
// Listing every project is an explicit platform-administrator-only operation.
func ResolveListScope(value string, platformAdmin bool) (ListScope, error) {
	scope := ListScope(strings.ToLower(strings.TrimSpace(value)))
	if scope == "" {
		scope = ListScopeRelated
	}
	switch scope {
	case ListScopeRelated:
		return scope, nil
	case ListScopeAll:
		if !platformAdmin {
			return "", ErrListScopeForbidden
		}
		return scope, nil
	default:
		return "", ErrListScopeInvalid
	}
}
