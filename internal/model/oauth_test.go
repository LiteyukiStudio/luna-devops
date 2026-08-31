package model

import (
	"sync"
	"testing"

	"gorm.io/gorm/schema"
)

func TestOAuthModelsUseMigrationTableNames(t *testing.T) {
	tests := []struct {
		name string
		got  string
		want string
	}{
		{name: "application", got: (OAuthApplication{}).TableName(), want: "oauth_applications"},
		{name: "grant", got: (OAuthGrant{}).TableName(), want: "oauth_grants"},
		{name: "authorization code", got: (OAuthAuthorizationCode{}).TableName(), want: "oauth_authorization_codes"},
		{name: "refresh token", got: (OAuthRefreshToken{}).TableName(), want: "oauth_refresh_tokens"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.got != test.want {
				t.Fatalf("table name = %q, want %q", test.got, test.want)
			}
		})
	}
}

func TestOAuthAccessTokenFieldsUseMigrationColumnNames(t *testing.T) {
	parsed, err := schema.Parse(&AccessToken{}, &sync.Map{}, schema.NamingStrategy{})
	if err != nil {
		t.Fatalf("parse access token schema: %v", err)
	}
	for fieldName, want := range map[string]string{
		"OAuthApplicationID": "oauth_application_id",
		"OAuthFamilyID":      "oauth_family_id",
		"OAuthGrantID":       "oauth_grant_id",
	} {
		field := parsed.LookUpField(fieldName)
		if field == nil {
			t.Fatalf("field %q not found", fieldName)
		}
		if field.DBName != want {
			t.Fatalf("field %q database name = %q, want %q", fieldName, field.DBName, want)
		}
	}
}

func TestOAuthRefreshTokenFamilyUsesMigrationColumnName(t *testing.T) {
	parsed, err := schema.Parse(&OAuthRefreshToken{}, &sync.Map{}, schema.NamingStrategy{})
	if err != nil {
		t.Fatalf("parse refresh token schema: %v", err)
	}
	field := parsed.LookUpField("FamilyID")
	if field == nil {
		t.Fatal("field FamilyID not found")
	}
	if field.DBName != "family_id" {
		t.Fatalf("FamilyID database name = %q, want family_id", field.DBName)
	}
}
