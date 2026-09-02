package runtimeaccess

// Enabled applies the project-level ceiling and an optional deployment-level
// further restriction used by interactive runtime subresources.
func Enabled(projectEnabled bool, deploymentOverride *bool) bool {
	return projectEnabled && (deploymentOverride == nil || *deploymentOverride)
}

// NormalizeOverride only persists a further disable. A true value means
// inherit the project setting and is represented as nil.
func NormalizeOverride(value *bool) *bool {
	if value == nil || *value {
		return nil
	}
	return value
}
