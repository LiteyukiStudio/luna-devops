package transferjob

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"sigs.k8s.io/yaml"
)

func TestWorkerImageDeliversVolumeTransferBinary(t *testing.T) {
	root := repositoryRoot(t)
	dockerfile := readContractFile(t, filepath.Join(root, "Dockerfile"))
	fragments := []string{
		"FROM source AS build-volume-transfer",
		"ARG TARGETOS\nARG TARGETARCH",
		"GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath -ldflags=\"-s -w\" -o /out/luna-volume-transfer ./cmd/volume-transfer",
		"FROM runtime AS runtime-worker",
		"COPY --from=build-volume-transfer --chmod=0755 /out/luna-volume-transfer /usr/local/bin/luna-volume-transfer",
		"RUN test -x /usr/local/bin/luna-volume-transfer",
	}
	for _, fragment := range fragments {
		if !strings.Contains(dockerfile, fragment) {
			t.Fatalf("Dockerfile does not satisfy Worker volume-transfer contract: missing %q", fragment)
		}
	}
	if strings.Index(dockerfile, "FROM runtime AS runtime-worker") < strings.Index(dockerfile, "ENTRYPOINT [\"/app/app\"]") {
		t.Fatal("runtime-worker must extend the completed non-root runtime image")
	}
}

func TestWorkerBuildConfigurationsSelectRuntimeWorkerTarget(t *testing.T) {
	root := repositoryRoot(t)
	var workflow struct {
		Jobs struct {
			Container struct {
				Strategy struct {
					Matrix struct {
						Include []struct {
							Title     string `json:"title"`
							Target    string `json:"target"`
							BuildArgs string `json:"build_args"`
						} `json:"include"`
					} `json:"matrix"`
				} `json:"strategy"`
			} `json:"container"`
		} `json:"jobs"`
	}
	if err := yaml.Unmarshal([]byte(readContractFile(t, filepath.Join(root, ".github", "workflows", "build-publish.yml"))), &workflow); err != nil {
		t.Fatalf("parse build workflow: %v", err)
	}
	foundWorker := false
	for _, item := range workflow.Jobs.Container.Strategy.Matrix.Include {
		if item.Title == "Worker" {
			foundWorker = true
			if item.Target != "runtime-worker" || item.BuildArgs != "TARGET=worker" {
				t.Fatalf("Worker publish target = %q args = %q", item.Target, item.BuildArgs)
			}
		}
	}
	if !foundWorker {
		t.Fatal("Worker image is missing from the publish matrix")
	}

	var compose struct {
		Services map[string]struct {
			Build struct {
				Target string            `json:"target"`
				Args   map[string]string `json:"args"`
			} `json:"build"`
		} `json:"services"`
	}
	if err := yaml.Unmarshal([]byte(readContractFile(t, filepath.Join(root, "docker-compose-build.yaml"))), &compose); err != nil {
		t.Fatalf("parse build compose file: %v", err)
	}
	worker, ok := compose.Services["worker"]
	if !ok || worker.Build.Target != "runtime-worker" || worker.Build.Args["TARGET"] != "worker" {
		t.Fatalf("local Worker image build contract = %#v", worker.Build)
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve image contract test path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
}

func readContractFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", filepath.Base(path), err)
	}
	return string(content)
}
