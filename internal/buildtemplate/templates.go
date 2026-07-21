package buildtemplate

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"text/template"
)

const DefinitionModeRepository = "repository_dockerfile"
const DefinitionModeTemplate = "template"

type Parameter struct {
	Key          string   `json:"key"`
	Type         string   `json:"type"`
	Required     bool     `json:"required"`
	DefaultValue string   `json:"defaultValue"`
	Options      []string `json:"options,omitempty"`
}

type Definition struct {
	ID                 string      `json:"id"`
	Version            string      `json:"version"`
	Runtime            string      `json:"runtime"`
	Category           string      `json:"category"`
	DefaultServicePort int         `json:"defaultServicePort"`
	Parameters         []Parameter `json:"parameters"`
	dockerfile         string
	detectionFiles     []string
}

type Preview struct {
	TemplateID string            `json:"templateId"`
	Version    string            `json:"version"`
	Values     map[string]string `json:"values"`
	Dockerfile string            `json:"dockerfile"`
	Checksum   string            `json:"checksum"`
}

var safeValuePattern = regexp.MustCompile(`^[^\r\n\x00]+$`)
var safePathPattern = regexp.MustCompile(`^[A-Za-z0-9._/@+\-]+(?:/[A-Za-z0-9._/@+\-]+)*$`)
var safeIdentifierPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

var definitions = []Definition{
	{
		ID: "node-service", Version: "1.0.0", Runtime: "node", Category: "service", DefaultServicePort: 3000,
		Parameters: []Parameter{
			{Key: "nodeVersion", Type: "select", Required: true, DefaultValue: "22", Options: []string{"20", "22", "24"}},
			{Key: "installCommand", Type: "command", Required: true, DefaultValue: "npm install"},
			{Key: "buildCommand", Type: "command", Required: true, DefaultValue: "npm run build"},
			{Key: "startCommand", Type: "command", Required: true, DefaultValue: "npm start"},
			{Key: "port", Type: "port", Required: true, DefaultValue: "3000"},
		},
		detectionFiles: []string{"package.json"},
		dockerfile: `FROM node:{{.nodeVersion}}-alpine AS build
WORKDIR /app
RUN corepack enable
COPY . .
RUN {{.installCommand}}
RUN {{.buildCommand}}

FROM node:{{.nodeVersion}}-alpine AS runtime
WORKDIR /app
ENV NODE_ENV=production
RUN corepack enable
COPY --from=build /app /app
EXPOSE {{.port}}
CMD ["sh", "-c", {{json .startCommand}}]
`,
	},
	{
		ID: "node-static", Version: "1.0.0", Runtime: "node", Category: "static", DefaultServicePort: 8080,
		Parameters: []Parameter{
			{Key: "nodeVersion", Type: "select", Required: true, DefaultValue: "22", Options: []string{"20", "22", "24"}},
			{Key: "installCommand", Type: "command", Required: true, DefaultValue: "npm install"},
			{Key: "buildCommand", Type: "command", Required: true, DefaultValue: "npm run build"},
			{Key: "outputDirectory", Type: "path", Required: true, DefaultValue: "dist"},
		},
		detectionFiles: []string{"package.json", "vite.config.js", "vite.config.ts"},
		dockerfile: `FROM node:{{.nodeVersion}}-alpine AS build
WORKDIR /app
RUN corepack enable
COPY . .
RUN {{.installCommand}}
RUN {{.buildCommand}}

FROM nginxinc/nginx-unprivileged:1.27-alpine
COPY --from=build /app/{{.outputDirectory}} /usr/share/nginx/html
EXPOSE 8080
`,
	},
	{
		ID: "nextjs-service", Version: "1.0.0", Runtime: "nextjs", Category: "service", DefaultServicePort: 3000,
		Parameters: []Parameter{
			{Key: "nodeVersion", Type: "select", Required: true, DefaultValue: "24", Options: []string{"20", "22", "24"}},
			{Key: "port", Type: "port", Required: true, DefaultValue: "3000"},
		},
		detectionFiles: []string{"package.json", "next.config.js", "next.config.mjs", "next.config.ts"},
		dockerfile: `FROM node:{{.nodeVersion}}-slim AS dependencies
WORKDIR /app
COPY package.json yarn.lock* package-lock.json* pnpm-lock.yaml* .npmrc* ./
RUN --mount=type=cache,target=/root/.npm \
    --mount=type=cache,target=/usr/local/share/.cache/yarn \
    --mount=type=cache,target=/root/.local/share/pnpm/store \
    if [ -f package-lock.json ]; then npm ci --no-audit --no-fund; \
    elif [ -f yarn.lock ]; then corepack enable yarn && yarn install --frozen-lockfile; \
    elif [ -f pnpm-lock.yaml ]; then corepack enable pnpm && pnpm install --frozen-lockfile; \
    else echo "A package-lock.json, yarn.lock, or pnpm-lock.yaml file is required." >&2; exit 1; fi

FROM node:{{.nodeVersion}}-slim AS builder
WORKDIR /app
COPY --from=dependencies /app/node_modules ./node_modules
COPY . .
ENV NODE_ENV=production NEXT_TELEMETRY_DISABLED=1
RUN mkdir -p public
RUN if [ -f package-lock.json ]; then npm run build; \
    elif [ -f yarn.lock ]; then corepack enable yarn && yarn build; \
    elif [ -f pnpm-lock.yaml ]; then corepack enable pnpm && pnpm build; \
    else echo "A package-lock.json, yarn.lock, or pnpm-lock.yaml file is required." >&2; exit 1; fi
RUN test -f .next/standalone/server.js || \
    (echo "Next.js standalone output is missing. Set output: 'standalone' in next.config.js, next.config.mjs, or next.config.ts." >&2; exit 1)

FROM node:{{.nodeVersion}}-slim AS runner
WORKDIR /app
ENV NODE_ENV=production NEXT_TELEMETRY_DISABLED=1 HOSTNAME=0.0.0.0 PORT={{.port}}
RUN groupadd --system --gid 1001 nodejs && useradd --system --uid 1001 --gid nodejs nextjs
COPY --from=builder --chown=nextjs:nodejs /app/public ./public
RUN mkdir .next && chown nextjs:nodejs .next
COPY --from=builder --chown=nextjs:nodejs /app/.next/standalone ./
COPY --from=builder --chown=nextjs:nodejs /app/.next/static ./.next/static
USER nextjs
EXPOSE {{.port}}
CMD ["node", "server.js"]
`,
	},
	{
		ID: "bun-service", Version: "1.0.0", Runtime: "bun", Category: "service", DefaultServicePort: 3000,
		Parameters: []Parameter{
			{Key: "installCommand", Type: "command", Required: true, DefaultValue: "bun install --frozen-lockfile"},
			{Key: "buildCommand", Type: "command", Required: true, DefaultValue: "bun run build"},
			{Key: "startCommand", Type: "command", Required: true, DefaultValue: "bun run start"},
			{Key: "port", Type: "port", Required: true, DefaultValue: "3000"},
		},
		detectionFiles: []string{"package.json", "bun.lock", "bun.lockb"},
		dockerfile: `FROM oven/bun:1-alpine AS build
WORKDIR /app
COPY . .
RUN {{.installCommand}}
RUN {{.buildCommand}}

FROM oven/bun:1-alpine AS runtime
WORKDIR /app
ENV NODE_ENV=production
COPY --from=build /app /app
EXPOSE {{.port}}
CMD ["sh", "-c", {{json .startCommand}}]
`,
	},
	{
		ID: "python-uv", Version: "1.0.0", Runtime: "python", Category: "service", DefaultServicePort: 8000,
		Parameters: []Parameter{
			{Key: "pythonVersion", Type: "select", Required: true, DefaultValue: "3.13", Options: []string{"3.11", "3.12", "3.13"}},
			{Key: "installCommand", Type: "command", Required: true, DefaultValue: "uv sync --no-dev"},
			{Key: "startCommand", Type: "command", Required: true, DefaultValue: "uv run python -m app"},
			{Key: "port", Type: "port", Required: true, DefaultValue: "8000"},
		},
		detectionFiles: []string{"pyproject.toml", "uv.lock"},
		dockerfile: `FROM ghcr.io/astral-sh/uv:python{{.pythonVersion}}-bookworm-slim
WORKDIR /app
ENV UV_COMPILE_BYTECODE=1 UV_LINK_MODE=copy
COPY . .
RUN {{.installCommand}}
EXPOSE {{.port}}
CMD ["sh", "-c", {{json .startCommand}}]
`,
	},
	{
		ID: "ruby-service", Version: "1.0.0", Runtime: "ruby", Category: "service", DefaultServicePort: 3000,
		Parameters: []Parameter{
			{Key: "installCommand", Type: "command", Required: true, DefaultValue: "bundle install"},
			{Key: "startCommand", Type: "command", Required: true, DefaultValue: "bundle exec rackup --host 0.0.0.0 --port 3000"},
			{Key: "port", Type: "port", Required: true, DefaultValue: "3000"},
		},
		detectionFiles: []string{"Gemfile", "Gemfile.lock"},
		dockerfile: `FROM ruby:3.4-alpine
WORKDIR /app
RUN apk add --no-cache build-base
COPY . .
RUN {{.installCommand}}
EXPOSE {{.port}}
CMD ["sh", "-c", {{json .startCommand}}]
`,
	},
	{
		ID: "go-service", Version: "1.0.0", Runtime: "go", Category: "service", DefaultServicePort: 8080,
		Parameters: []Parameter{
			{Key: "goVersion", Type: "select", Required: true, DefaultValue: "1.25", Options: []string{"1.24", "1.25", "1.26"}},
			{Key: "buildPackage", Type: "path", Required: true, DefaultValue: "."},
			{Key: "port", Type: "port", Required: true, DefaultValue: "8080"},
		},
		detectionFiles: []string{"go.mod"},
		dockerfile: `FROM golang:{{.goVersion}}-alpine AS build
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/app {{.buildPackage}}

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/app /app
EXPOSE {{.port}}
USER nonroot:nonroot
ENTRYPOINT ["/app"]
`,
	},
	{
		ID: "rust-service", Version: "1.0.0", Runtime: "rust", Category: "service", DefaultServicePort: 8080,
		Parameters: []Parameter{
			{Key: "rustVersion", Type: "select", Required: true, DefaultValue: "1.88", Options: []string{"1.86", "1.87", "1.88"}},
			{Key: "buildCommand", Type: "command", Required: true, DefaultValue: "cargo build --release"},
			{Key: "binaryName", Type: "identifier", Required: true, DefaultValue: "app"},
			{Key: "port", Type: "port", Required: true, DefaultValue: "8080"},
		},
		detectionFiles: []string{"Cargo.toml", "Cargo.lock"},
		dockerfile: `FROM rust:{{.rustVersion}}-slim-bookworm AS build
WORKDIR /src
COPY . .
RUN {{.buildCommand}}

FROM debian:bookworm-slim
RUN useradd --system --uid 10001 app
COPY --from=build /src/target/release/{{.binaryName}} /usr/local/bin/app
EXPOSE {{.port}}
USER app
ENTRYPOINT ["/usr/local/bin/app"]
`,
	},
	{
		ID: "java-maven", Version: "1.0.0", Runtime: "java", Category: "service", DefaultServicePort: 8080,
		Parameters: []Parameter{
			{Key: "buildCommand", Type: "command", Required: true, DefaultValue: "mvn -B -DskipTests package"},
			{Key: "port", Type: "port", Required: true, DefaultValue: "8080"},
		},
		detectionFiles: []string{"pom.xml"},
		dockerfile: `FROM maven:3.9-eclipse-temurin-21-alpine AS build
WORKDIR /src
COPY . .
RUN {{.buildCommand}}
RUN artifact="$(find target -maxdepth 1 -type f -name '*.jar' ! -name 'original-*' | head -n 1)" \
    && test -n "$artifact" \
    && cp "$artifact" /tmp/app.jar

FROM eclipse-temurin:21-jre-alpine
WORKDIR /app
COPY --from=build /tmp/app.jar /app/app.jar
EXPOSE {{.port}}
ENTRYPOINT ["java", "-jar", "/app/app.jar"]
`,
	},
	{
		ID: "java-gradle", Version: "1.0.0", Runtime: "java", Category: "service", DefaultServicePort: 8080,
		Parameters: []Parameter{
			{Key: "buildCommand", Type: "command", Required: true, DefaultValue: "gradle --no-daemon build -x test"},
			{Key: "port", Type: "port", Required: true, DefaultValue: "8080"},
		},
		detectionFiles: []string{"build.gradle", "build.gradle.kts", "gradlew"},
		dockerfile: `FROM gradle:8-jdk21-alpine AS build
WORKDIR /home/gradle/project
COPY --chown=gradle:gradle . .
RUN {{.buildCommand}}
RUN artifact="$(find build/libs -maxdepth 1 -type f -name '*.jar' ! -name '*-plain.jar' | head -n 1)" \
    && test -n "$artifact" \
    && cp "$artifact" /tmp/app.jar

FROM eclipse-temurin:21-jre-alpine
WORKDIR /app
COPY --from=build /tmp/app.jar /app/app.jar
EXPOSE {{.port}}
ENTRYPOINT ["java", "-jar", "/app/app.jar"]
`,
	},
	{
		ID: "dotnet-service", Version: "1.0.0", Runtime: "dotnet", Category: "service", DefaultServicePort: 8080,
		Parameters: []Parameter{
			{Key: "projectPath", Type: "path", Required: true, DefaultValue: "."},
			{Key: "assemblyFile", Type: "identifier", Required: true, DefaultValue: "app.dll"},
			{Key: "port", Type: "port", Required: true, DefaultValue: "8080"},
		},
		detectionFiles: []string{".csproj"},
		dockerfile: `FROM mcr.microsoft.com/dotnet/sdk:8.0-alpine3.21 AS build
WORKDIR /src
COPY . .
RUN dotnet publish {{.projectPath}} -c Release -o /out --no-self-contained

FROM mcr.microsoft.com/dotnet/aspnet:8.0-alpine3.21
WORKDIR /app
ENV ASPNETCORE_HTTP_PORTS={{.port}}
COPY --from=build /out /app
EXPOSE {{.port}}
ENTRYPOINT ["dotnet", {{json .assemblyFile}}]
`,
	},
	{
		ID: "static-site", Version: "1.0.0", Runtime: "static", Category: "static", DefaultServicePort: 8080,
		Parameters: []Parameter{
			{Key: "sourceDirectory", Type: "path", Required: true, DefaultValue: "."},
		},
		detectionFiles: []string{"index.html"},
		dockerfile: `FROM nginxinc/nginx-unprivileged:1.27-alpine
COPY {{.sourceDirectory}} /usr/share/nginx/html
EXPOSE 8080
`,
	},
}

func List() []Definition {
	items := make([]Definition, len(definitions))
	copy(items, definitions)
	return items
}

func Find(id, version string) (Definition, bool) {
	for _, definition := range definitions {
		if definition.ID == strings.TrimSpace(id) && (strings.TrimSpace(version) == "" || definition.Version == strings.TrimSpace(version)) {
			return definition, true
		}
	}
	return Definition{}, false
}

func Recommend(files []string) []string {
	normalized := make(map[string]bool, len(files))
	for _, file := range files {
		normalized[filepath.ToSlash(strings.TrimPrefix(strings.TrimSpace(file), "./"))] = true
	}
	type scored struct {
		id    string
		score int
	}
	scores := make([]scored, 0, len(definitions))
	for _, definition := range definitions {
		score := 0
		for _, wanted := range definition.detectionFiles {
			for file := range normalized {
				if file == wanted || strings.HasSuffix(file, "/"+wanted) || (strings.HasPrefix(wanted, ".") && strings.HasSuffix(strings.ToLower(file), strings.ToLower(wanted))) {
					score++
					break
				}
			}
		}
		if score > 0 {
			scores = append(scores, scored{id: definition.ID, score: score})
		}
	}
	sort.SliceStable(scores, func(i, j int) bool { return scores[i].score > scores[j].score })
	output := make([]string, 0, len(scores))
	for _, item := range scores {
		output = append(output, item.id)
	}
	return output
}

func NormalizeValues(definition Definition, raw string) (map[string]string, error) {
	provided := map[string]string{}
	if strings.TrimSpace(raw) != "" {
		if err := json.Unmarshal([]byte(raw), &provided); err != nil {
			return nil, fmt.Errorf("invalid build template values: %w", err)
		}
	}
	return normalizeMap(definition, provided)
}

func Render(id, version string, values map[string]string) (Preview, error) {
	definition, ok := Find(id, version)
	if !ok {
		return Preview{}, fmt.Errorf("build template not found")
	}
	normalized, err := normalizeMap(definition, values)
	if err != nil {
		return Preview{}, err
	}
	tmpl, err := template.New(definition.ID).Funcs(template.FuncMap{
		"json": func(value string) string {
			encoded, _ := json.Marshal(value)
			return string(encoded)
		},
	}).Option("missingkey=error").Parse(definition.dockerfile)
	if err != nil {
		return Preview{}, fmt.Errorf("parse build template: %w", err)
	}
	var output bytes.Buffer
	if err := tmpl.Execute(&output, normalized); err != nil {
		return Preview{}, fmt.Errorf("render build template: %w", err)
	}
	dockerfile := output.String()
	digest := sha256.Sum256([]byte(dockerfile))
	return Preview{TemplateID: definition.ID, Version: definition.Version, Values: normalized, Dockerfile: dockerfile, Checksum: hex.EncodeToString(digest[:])}, nil
}

func EncodeValues(values map[string]string) string {
	content, _ := json.Marshal(values)
	return string(content)
}

func normalizeMap(definition Definition, provided map[string]string) (map[string]string, error) {
	allowed := make(map[string]Parameter, len(definition.Parameters))
	for _, parameter := range definition.Parameters {
		allowed[parameter.Key] = parameter
	}
	for key := range provided {
		if _, ok := allowed[key]; !ok {
			return nil, fmt.Errorf("unknown build template parameter %q", key)
		}
	}
	output := make(map[string]string, len(definition.Parameters))
	for _, parameter := range definition.Parameters {
		value := strings.TrimSpace(provided[parameter.Key])
		if value == "" {
			value = parameter.DefaultValue
		}
		if parameter.Required && value == "" {
			return nil, fmt.Errorf("build template parameter %q is required", parameter.Key)
		}
		if err := validateValue(parameter, value); err != nil {
			return nil, err
		}
		output[parameter.Key] = value
	}
	return output, nil
}

func validateValue(parameter Parameter, value string) error {
	if value == "" {
		return nil
	}
	if !safeValuePattern.MatchString(value) {
		return fmt.Errorf("build template parameter %q contains unsupported characters", parameter.Key)
	}
	if len(parameter.Options) > 0 {
		for _, option := range parameter.Options {
			if value == option {
				return nil
			}
		}
		return fmt.Errorf("build template parameter %q has an unsupported value", parameter.Key)
	}
	switch parameter.Type {
	case "path":
		if value != "." && (!safePathPattern.MatchString(value) || filepath.IsAbs(value) || strings.Contains(value, "..")) {
			return fmt.Errorf("build template parameter %q must be a relative path", parameter.Key)
		}
	case "identifier":
		if !safeIdentifierPattern.MatchString(value) {
			return fmt.Errorf("build template parameter %q must be an identifier", parameter.Key)
		}
	case "port":
		port, err := strconv.Atoi(value)
		if err != nil || port < 1 || port > 65535 {
			return fmt.Errorf("build template parameter %q must be a valid port", parameter.Key)
		}
	}
	return nil
}
