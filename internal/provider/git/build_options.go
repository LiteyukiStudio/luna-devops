package gitprovider

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

const gitBuildOptionsMaxDirectories = 80
const gitBuildOptionsMaxDepth = 3

var buildTemplateDetectionFiles = map[string]bool{
	"Cargo.lock": true, "Cargo.toml": true, "Gemfile": true, "Gemfile.lock": true,
	"build.gradle": true, "build.gradle.kts": true, "bun.lock": true, "bun.lockb": true,
	"go.mod": true, "gradlew": true, "index.html": true, "package.json": true,
	"next.config.js": true, "next.config.mjs": true, "next.config.ts": true,
	"pnpm-lock.yaml": true, "pom.xml": true, "pyproject.toml": true, "uv.lock": true,
	"vite.config.js": true, "vite.config.ts": true, "yarn.lock": true,
}

func (c Client) DiscoverBuildOptions(ctx context.Context, owner, repo, ref string) (BuildOptions, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		ref = "main"
	}
	options, err := c.discoverBuildOptionsByTree(ctx, owner, repo, ref)
	if err == nil && !options.Truncated {
		c.populateDockerfileExposedPorts(ctx, owner, repo, ref, &options)
		return options, nil
	}
	fallbackOptions, fallbackErr := c.discoverBuildOptionsByContents(ctx, owner, repo, ref)
	if fallbackErr != nil {
		if err != nil {
			return BuildOptions{}, err
		}
		return BuildOptions{}, fallbackErr
	}
	if err == nil && options.Truncated {
		fallbackOptions.Truncated = true
	}
	c.populateDockerfileExposedPorts(ctx, owner, repo, ref, &fallbackOptions)
	return fallbackOptions, nil
}

func (c Client) populateDockerfileExposedPorts(ctx context.Context, owner, repo, ref string, options *BuildOptions) {
	if options == nil || len(options.Dockerfiles) == 0 {
		return
	}
	exposedPorts := make(map[string][]int)
	for _, dockerfile := range options.Dockerfiles {
		content, err := c.ReadFile(ctx, owner, repo, dockerfile, ref)
		if err != nil {
			continue
		}
		ports := parseDockerfileExposedPorts(content.Content)
		if len(ports) > 0 {
			exposedPorts[dockerfile] = ports
		}
	}
	if len(exposedPorts) > 0 {
		options.ExposedPorts = exposedPorts
	}
}

func (c Client) discoverBuildOptionsByTree(ctx context.Context, owner, repo, ref string) (BuildOptions, error) {
	branch, err := c.GetBranch(ctx, owner, repo, ref)
	if err != nil {
		return BuildOptions{}, err
	}
	params := map[string]string{}
	switch c.provider.Type {
	case "github":
		params["recursive"] = "1"
	case "gitea":
		params["recursive"] = "true"
	default:
		return BuildOptions{}, fmt.Errorf("git provider type %q is not supported", c.provider.Type)
	}
	var tree gitTreeResponse
	if err := c.getJSON(ctx, c.apiURL(fmt.Sprintf("/repos/%s/%s/git/trees/%s", pathEscape(owner), pathEscape(repo), pathEscape(branch.SHA)), params), &tree); err != nil {
		return BuildOptions{}, err
	}
	dockerfiles := map[string]struct{}{}
	detectedFiles := map[string]struct{}{}
	directories := map[string]struct{}{".": {}}
	for _, item := range tree.Tree {
		path := strings.Trim(strings.TrimSpace(item.Path), "/")
		if path == "" {
			continue
		}
		switch normalizeContentType(item.Type) {
		case "dir":
			directories[path] = struct{}{}
		case "file":
			parts := strings.Split(path, "/")
			name := parts[len(parts)-1]
			if isDockerfileName(name) {
				dockerfiles[path] = struct{}{}
			}
			if isBuildTemplateDetectionFile(name) {
				detectedFiles[path] = struct{}{}
			}
		}
	}
	return BuildOptions{
		Directories:   sortedPathSet(directories),
		Dockerfiles:   sortedPathSet(dockerfiles),
		DetectedFiles: sortedPathSet(detectedFiles),
		Strategy:      "recursive-tree",
		Truncated:     tree.Truncated,
	}, nil
}

func (c Client) discoverBuildOptionsByContents(ctx context.Context, owner, repo, ref string) (BuildOptions, error) {
	dockerfiles := map[string]struct{}{}
	detectedFiles := map[string]struct{}{}
	directories := map[string]struct{}{".": {}}
	queue := []struct {
		path  string
		depth int
	}{{path: "", depth: 0}}

	for index := 0; index < len(queue) && index < gitBuildOptionsMaxDirectories; index++ {
		current := queue[index]
		items, err := c.ListContents(ctx, owner, repo, current.path, ref)
		if err != nil {
			return BuildOptions{}, err
		}
		for _, item := range items {
			switch item.Type {
			case "dir":
				directories[item.Path] = struct{}{}
				if current.depth < gitBuildOptionsMaxDepth {
					queue = append(queue, struct {
						path  string
						depth int
					}{path: item.Path, depth: current.depth + 1})
				}
			case "file":
				if isDockerfileName(item.Name) {
					dockerfiles[item.Path] = struct{}{}
				}
				if isBuildTemplateDetectionFile(item.Name) {
					detectedFiles[item.Path] = struct{}{}
				}
			}
		}
	}
	return BuildOptions{
		Directories:   sortedPathSet(directories),
		Dockerfiles:   sortedPathSet(dockerfiles),
		DetectedFiles: sortedPathSet(detectedFiles),
		Strategy:      "contents-bfs",
		Truncated:     len(queue) > gitBuildOptionsMaxDirectories,
	}, nil
}

func isDockerfileName(name string) bool {
	return name == "Dockerfile" || strings.HasPrefix(name, "Dockerfile.") || strings.HasSuffix(name, ".Dockerfile")
}

func isBuildTemplateDetectionFile(name string) bool {
	return buildTemplateDetectionFiles[name] || strings.HasSuffix(strings.ToLower(name), ".csproj")
}

func parseDockerfileExposedPorts(content string) []int {
	seen := map[int]bool{}
	ports := []int{}
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 || !strings.EqualFold(fields[0], "EXPOSE") {
			continue
		}
		for _, field := range fields[1:] {
			value := strings.TrimSpace(strings.SplitN(field, "/", 2)[0])
			port, err := strconv.Atoi(value)
			if err != nil || port < 1 || port > 65535 || seen[port] {
				continue
			}
			seen[port] = true
			ports = append(ports, port)
		}
	}
	return ports
}

func sortedPathSet(paths map[string]struct{}) []string {
	output := make([]string, 0, len(paths))
	for path := range paths {
		output = append(output, path)
	}
	sortBuildPaths(output)
	return output
}

func sortBuildPaths(paths []string) {
	sort.Slice(paths, func(i, j int) bool {
		if paths[i] == "." {
			return true
		}
		if paths[j] == "." {
			return false
		}
		return paths[i] < paths[j]
	})
}
