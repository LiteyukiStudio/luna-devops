package deploymentapi

type releaseRuntimeExecInput struct {
	Command   string `json:"command"`
	Container string `json:"container"`
}
