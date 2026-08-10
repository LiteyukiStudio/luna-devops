package notification

import (
	"context"
	"encoding/json"
	"time"
)

const (
	AdapterKindWebhook = "webhook"
	AdapterKindSMTP    = "smtp"

	SeverityInfo    = "info"
	SeverityWarning = "warning"
	SeverityError   = "error"
)

type Event struct {
	ID               string                `json:"id"`
	Type             string                `json:"type"`
	Severity         string                `json:"severity"`
	Locale           string                `json:"locale"`
	Project          EntityRef             `json:"project"`
	Application      EntityRef             `json:"application"`
	DeploymentTarget EntityRef             `json:"deploymentTarget"`
	Build            BuildContext          `json:"build"`
	Release          ReleaseContext        `json:"release"`
	Hook             HookContext           `json:"hook"`
	Gateway          GatewayContext        `json:"gateway"`
	Certificate      CertificateContext    `json:"certificate"`
	ServiceBinding   ServiceBindingContext `json:"serviceBinding"`
	Actor            ActorContext          `json:"actor"`
	Links            map[string]string     `json:"links"`
	CorrelationID    string                `json:"correlationId"`
	TraceID          string                `json:"traceId"`
	DedupKey         string                `json:"-"`
	OccurredAt       time.Time             `json:"occurredAt"`
	Message          string                `json:"message"`
}

type ServiceBindingContext struct {
	ID                       string `json:"id"`
	Status                   string `json:"status"`
	SourceDeploymentTargetID string `json:"sourceDeploymentTargetId"`
	TargetApplicationID      string `json:"targetApplicationId"`
	TargetDeploymentTargetID string `json:"targetDeploymentTargetId"`
}

type EntityRef struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Identifier string `json:"identifier"`
}

type BuildContext struct {
	ID      string `json:"id"`
	Status  string `json:"status"`
	Message string `json:"message"`
	Image   string `json:"image"`
	GitRef  string `json:"gitRef"`
	GitSHA  string `json:"gitSha"`
}

type ReleaseContext struct {
	ID       string `json:"id"`
	Status   string `json:"status"`
	Revision int    `json:"revision"`
	ImageRef string `json:"imageRef"`
	Message  string `json:"message"`
}

type HookContext struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Phase   string `json:"phase"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

type GatewayContext struct {
	ID      string `json:"id"`
	Domain  string `json:"domain"`
	Path    string `json:"path"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

type CertificateContext struct {
	RouteID    string     `json:"routeId"`
	Host       string     `json:"host"`
	Status     string     `json:"status"`
	Message    string     `json:"message"`
	NotAfter   *time.Time `json:"notAfter"`
	IssuerKind string     `json:"issuerKind"`
	IssuerName string     `json:"issuerName"`
}

type ActorContext struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

type Template struct {
	Subject string
	Body    string
	JSON    string
}

type RenderedMessage struct {
	Subject string
	Body    string
	JSON    []byte
	Method  string
	URL     string
	Headers map[string]string
}

type SendResult struct {
	StatusCode      int
	ResponseSnippet string
}

type Adapter interface {
	Kind() string
	Validate(ctx context.Context, config json.RawMessage, secretResolver SecretResolver) error
	Render(ctx context.Context, event Event, template Template, config json.RawMessage, secrets json.RawMessage, secretResolver SecretResolver, locale string) (RenderedMessage, error)
	Send(ctx context.Context, config json.RawMessage, secrets json.RawMessage, message RenderedMessage, secretResolver SecretResolver) (SendResult, error)
	Test(ctx context.Context, config json.RawMessage, secrets json.RawMessage, secretResolver SecretResolver) error
}

type SecretResolver interface {
	ResolveContext(ctx context.Context, ref string) string
}

type StaticSecretResolver map[string]string

func (r StaticSecretResolver) ResolveContext(_ context.Context, ref string) string {
	return r[ref]
}
