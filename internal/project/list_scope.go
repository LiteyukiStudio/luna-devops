package project

import (
	"errors"
	"strings"
)

type ListVisibility string

const (
	ListVisibilityRelated ListVisibility = "related"
	ListVisibilityAll     ListVisibility = "all"
)

var (
	ErrListVisibilityInvalid   = errors.New("list visibility is invalid")
	ErrListVisibilityForbidden = errors.New("all list visibility requires platform administrator")
)

// ResolveListVisibility keeps cross-project discovery related to the caller by
// default. Listing every project-scoped resource is an explicit
// platform-administrator-only operation.
func ResolveListVisibility(value string, platformAdmin bool) (ListVisibility, error) {
	visibility := ListVisibility(strings.ToLower(strings.TrimSpace(value)))
	if visibility == "" {
		visibility = ListVisibilityRelated
	}
	switch visibility {
	case ListVisibilityRelated:
		return visibility, nil
	case ListVisibilityAll:
		if !platformAdmin {
			return "", ErrListVisibilityForbidden
		}
		return visibility, nil
	default:
		return "", ErrListVisibilityInvalid
	}
}
