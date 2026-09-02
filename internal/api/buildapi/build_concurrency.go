package buildapi

const (
	defaultClusterBuildConcurrency = 4
	defaultProjectBuildConcurrency = 2
)

func normalizeBuildConcurrency(value int, defaultValue int) int {
	if value > 0 {
		return value
	}
	return defaultValue
}
