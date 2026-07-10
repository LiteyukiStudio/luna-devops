package model

import (
	"time"

	"gorm.io/gorm"
)

type GatewayRoute struct {
	ID                     string           `gorm:"primaryKey" json:"id"`
	ProjectID              string           `gorm:"index;not null" json:"projectId"`
	ApplicationID          string           `gorm:"index;not null" json:"applicationId"`
	EnvironmentID          string           `gorm:"index" json:"environmentId"`
	DeploymentTargetID     string           `gorm:"index;not null;default:''" json:"deploymentTargetId"`
	Host                   string           `gorm:"index;not null" json:"host"`
	DomainSuffix           string           `gorm:"not null;default:''" json:"domainSuffix"`
	Path                   string           `gorm:"not null;default:/" json:"path"`
	ServicePort            int              `gorm:"not null;default:80" json:"servicePort"`
	TLSMode                string           `gorm:"not null;default:http-only" json:"tlsMode"`
	ParentGatewayName      string           `gorm:"not null;default:''" json:"parentGatewayName"`
	ParentGatewayNamespace string           `gorm:"not null;default:''" json:"parentGatewayNamespace"`
	SectionName            string           `gorm:"not null;default:''" json:"sectionName"`
	PathMatchType          string           `gorm:"not null;default:PathPrefix" json:"pathMatchType"`
	RequestHeaders         string           `gorm:"type:text;not null;default:''" json:"requestHeaders"`
	ResponseHeaders        string           `gorm:"type:text;not null;default:''" json:"responseHeaders"`
	URLRewrite             string           `gorm:"type:text;not null;default:''" json:"urlRewrite"`
	RequestRedirect        string           `gorm:"type:text;not null;default:''" json:"requestRedirect"`
	BackendWeight          int              `gorm:"not null;default:1" json:"backendWeight"`
	HostnameAliases        string           `gorm:"type:text;not null;default:''" json:"hostnameAliases"`
	CertificateStatus      string           `gorm:"not null;default:disabled" json:"certificateStatus"`
	CertificateMessage     string           `gorm:"type:text;not null;default:''" json:"certificateMessage"`
	CertificateNotAfter    *time.Time       `json:"certificateNotAfter"`
	CertificateIssuerKind  string           `gorm:"not null;default:''" json:"certificateIssuerKind"`
	CertificateIssuerName  string           `gorm:"not null;default:''" json:"certificateIssuerName"`
	CNAMEName              string           `json:"cnameName"`
	CNAMETarget            string           `json:"cnameTarget"`
	DNSStatus              string           `gorm:"not null;default:pending" json:"dnsStatus"`
	Status                 string           `gorm:"not null;default:pending" json:"status"`
	Enabled                bool             `gorm:"not null;default:true" json:"enabled"`
	DeleteStatus           string           `gorm:"index;not null;default:active" json:"deleteStatus"`
	DeleteMessage          string           `gorm:"type:text;not null;default:''" json:"deleteMessage"`
	DeleteStartedAt        *time.Time       `json:"deleteStartedAt"`
	DeleteFinishedAt       *time.Time       `json:"deleteFinishedAt"`
	IsDefault              bool             `gorm:"not null;default:false" json:"isDefault"`
	AccessURL              string           `gorm:"-" json:"accessUrl"`
	RouteSummary           string           `gorm:"-" json:"routeSummary"`
	Conditions             []RouteCondition `gorm:"-" json:"conditions"`
	CreatedBy              string           `gorm:"index" json:"createdBy"`
	CreatedAt              time.Time        `json:"createdAt"`
	UpdatedAt              time.Time        `json:"updatedAt"`
	DeletedAt              gorm.DeletedAt   `gorm:"index" json:"-"`
}

type RouteCondition struct {
	Type               string `json:"type"`
	Status             string `json:"status"`
	Reason             string `json:"reason"`
	Message            string `json:"message"`
	ObservedGeneration int64  `json:"observedGeneration"`
}
