package deploymentapi

type releaseRuntimeExecInput struct {
	Command   string `json:"command"`
	Pod       string `json:"pod"`
	Container string `json:"container"`
}
