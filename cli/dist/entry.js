var __defProp = Object.defineProperty;
var __export = (target, all) => {
  for (var name in all)
    __defProp(target, name, { get: all[name], enumerable: true });
};

// src/entry.ts
import process13 from "process";
import { pathToFileURL } from "url";

// ../packages/api-contract/src/index.ts
var src_exports = {};
__export(src_exports, {
  CONTRACT_SCHEMA_IDENTITY: () => CONTRACT_SCHEMA_IDENTITY,
  CONTRACT_SCHEMA_NAME: () => CONTRACT_SCHEMA_NAME,
  CONTRACT_SCHEMA_VERSION: () => CONTRACT_SCHEMA_VERSION,
  HTTP_METHODS: () => HTTP_METHODS,
  OPENAPI_OPERATION_SNAPSHOTS: () => OPENAPI_OPERATION_SNAPSHOTS,
  OPENAPI_SNAPSHOT_METADATA: () => OPENAPI_SNAPSHOT_METADATA,
  OPERATION_CATALOG: () => OPERATION_CATALOG,
  OPERATION_CATALOG_METADATA: () => OPERATION_CATALOG_METADATA,
  buildOperationCatalog: () => buildOperationCatalog,
  createFallbackOperationId: () => createFallbackOperationId,
  createOperationKey: () => createOperationKey,
  digestJson: () => digestJson,
  filterOperationCatalog: () => filterOperationCatalog,
  findOperationByCommand: () => findOperationByCommand,
  findOperationById: () => findOperationById,
  findOperationByRoute: () => findOperationByRoute,
  isCompatibleContractSchema: () => isCompatibleContractSchema,
  isSha256Digest: () => isSha256Digest,
  normalizeOpenApiPath: () => normalizeOpenApiPath,
  pageOperationCatalog: () => pageOperationCatalog,
  sha256Hex: () => sha256Hex,
  stableStringify: () => stableStringify,
  toKebabCase: () => toKebabCase
});

// ../packages/api-contract/src/generated/operations.ts
var OPENAPI_SNAPSHOT_METADATA = {
  "source": "openapi/openapi.yaml",
  "openapiVersion": "3.1.0",
  "apiVersion": "0.1.0",
  "sourceDigest": "sha256:6fb5ef6c0ed37ce09f0d7c63b53188c5c956cc1b21a3543df3c1653f0c578bc7",
  "operationCount": 110
};
var OPENAPI_OPERATION_SNAPSHOTS = [
  {
    "method": "get",
    "path": "/healthz",
    "tags": [
      "Health"
    ],
    "deprecated": false,
    "security": [],
    "parameters": [],
    "responses": [
      {
        "status": "200",
        "contentTypes": [],
        "schemaRefs": [],
        "description": "Service is healthy."
      }
    ],
    "summary": "Health check"
  },
  {
    "method": "get",
    "path": "/api/v1/meta",
    "tags": [
      "Health"
    ],
    "deprecated": false,
    "security": [],
    "parameters": [],
    "responses": [
      {
        "status": "200",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [],
        "description": "Public API compatibility and feature metadata."
      }
    ],
    "summary": "Get API version and CLI capability metadata",
    "operationId": "getApiMeta"
  },
  {
    "method": "post",
    "path": "/api/v1/public/configs",
    "tags": [
      "Configs"
    ],
    "deprecated": false,
    "security": [],
    "parameters": [],
    "responses": [
      {
        "status": "200",
        "contentTypes": [],
        "schemaRefs": [],
        "description": "Public config dictionary."
      }
    ],
    "summary": "Get public app configs by keys",
    "requestBody": {
      "required": true,
      "contentTypes": [
        "application/json"
      ],
      "schemaRefs": [
        "#/components/schemas/ConfigKeysInput"
      ]
    },
    "inputSchema": {
      "type": "object",
      "properties": {
        "body": {
          "ref": "#/components/schemas/ConfigKeysInput",
          "type": "object",
          "required": [
            "keys"
          ],
          "properties": {
            "keys": {
              "type": "array",
              "items": {
                "type": "string"
              }
            }
          }
        }
      },
      "required": [
        "body"
      ],
      "additionalProperties": false
    }
  },
  {
    "method": "get",
    "path": "/api/v1/build/templates",
    "tags": [
      "Builds"
    ],
    "deprecated": false,
    "security": [],
    "parameters": [],
    "responses": [
      {
        "status": "200",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/BuildTemplate"
        ],
        "description": "Built-in template catalog."
      }
    ],
    "summary": "List immutable platform build templates"
  },
  {
    "method": "post",
    "path": "/api/v1/build/templates/{templateId}/preview",
    "tags": [
      "Builds"
    ],
    "deprecated": false,
    "security": [],
    "parameters": [
      {
        "name": "templateId",
        "in": "path",
        "required": true,
        "schema": {
          "type": "string"
        }
      }
    ],
    "responses": [
      {
        "status": "200",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/BuildTemplatePreview"
        ],
        "description": "Rendered, immutable build definition preview."
      }
    ],
    "summary": "Validate template parameters and preview the generated Dockerfile",
    "requestBody": {
      "required": true,
      "contentTypes": [
        "application/json"
      ],
      "schemaRefs": [
        "#/components/schemas/BuildTemplatePreviewInput"
      ]
    },
    "inputSchema": {
      "type": "object",
      "properties": {
        "templateId": {
          "type": "string"
        },
        "body": {
          "ref": "#/components/schemas/BuildTemplatePreviewInput",
          "type": "object",
          "required": [
            "values"
          ],
          "properties": {
            "values": {
              "type": "object",
              "additionalProperties": {
                "type": "string"
              }
            },
            "version": {
              "type": "string"
            }
          }
        }
      },
      "required": [
        "body",
        "templateId"
      ],
      "additionalProperties": false
    }
  },
  {
    "method": "get",
    "path": "/api/v1/build/environment-config",
    "tags": [
      "Builds"
    ],
    "deprecated": false,
    "security": [],
    "parameters": [
      {
        "name": "scope",
        "in": "query",
        "required": true,
        "ref": "#/components/parameters/BuildEnvironmentScope",
        "schema": {
          "type": "string",
          "enum": [
            "global",
            "application",
            "deployment"
          ]
        }
      },
      {
        "name": "projectId",
        "in": "query",
        "schema": {
          "type": "string"
        }
      },
      {
        "name": "applicationId",
        "in": "query",
        "schema": {
          "type": "string"
        }
      },
      {
        "name": "deploymentTargetId",
        "in": "query",
        "schema": {
          "type": "string"
        }
      }
    ],
    "responses": [
      {
        "status": "200",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/BuildEnvironmentConfig"
        ],
        "description": "Public values and boolean secret presence. Secret values and references are never returned."
      }
    ],
    "summary": "Get one global, application, or deployment build environment",
    "inputSchema": {
      "type": "object",
      "properties": {
        "scope": {
          "type": "string",
          "enum": [
            "global",
            "application",
            "deployment"
          ]
        },
        "projectId": {
          "type": "string"
        },
        "applicationId": {
          "type": "string"
        },
        "deploymentTargetId": {
          "type": "string"
        }
      },
      "required": [
        "scope"
      ],
      "additionalProperties": false
    }
  },
  {
    "method": "put",
    "path": "/api/v1/build/environment-config",
    "tags": [
      "Builds"
    ],
    "deprecated": false,
    "security": [],
    "parameters": [
      {
        "name": "scope",
        "in": "query",
        "required": true,
        "ref": "#/components/parameters/BuildEnvironmentScope",
        "schema": {
          "type": "string",
          "enum": [
            "global",
            "application",
            "deployment"
          ]
        }
      },
      {
        "name": "projectId",
        "in": "query",
        "schema": {
          "type": "string"
        }
      },
      {
        "name": "applicationId",
        "in": "query",
        "schema": {
          "type": "string"
        }
      },
      {
        "name": "deploymentTargetId",
        "in": "query",
        "schema": {
          "type": "string"
        }
      }
    ],
    "responses": [
      {
        "status": "200",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/BuildEnvironmentConfig"
        ],
        "description": "Updated build environment with secret presence only."
      }
    ],
    "summary": "Replace one global, application, or deployment build environment",
    "requestBody": {
      "required": true,
      "contentTypes": [
        "application/json"
      ],
      "schemaRefs": [
        "#/components/schemas/BuildEnvironmentConfigInput"
      ]
    },
    "inputSchema": {
      "type": "object",
      "properties": {
        "scope": {
          "type": "string",
          "enum": [
            "global",
            "application",
            "deployment"
          ]
        },
        "projectId": {
          "type": "string"
        },
        "applicationId": {
          "type": "string"
        },
        "deploymentTargetId": {
          "type": "string"
        },
        "body": {
          "ref": "#/components/schemas/BuildEnvironmentConfigInput",
          "type": "object",
          "required": [
            "secrets",
            "variables"
          ],
          "properties": {
            "secrets": {
              "type": "object",
              "description": "Existing keys may use an empty value to retain their encrypted value. Omitted keys are removed.",
              "writeOnly": true,
              "additionalProperties": {
                "type": "string"
              }
            },
            "variables": {
              "type": "object",
              "additionalProperties": {
                "type": "string"
              }
            }
          }
        }
      },
      "required": [
        "body",
        "scope"
      ],
      "additionalProperties": false
    }
  },
  {
    "method": "get",
    "path": "/api/v1/auth/bootstrap",
    "tags": [
      "Auth"
    ],
    "deprecated": false,
    "security": [],
    "parameters": [],
    "responses": [
      {
        "status": "200",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/BootstrapStatus"
        ],
        "description": "Bootstrap status. devLoginHint is returned only in development mode."
      }
    ],
    "summary": "Get bootstrap and runtime mode status"
  },
  {
    "method": "post",
    "path": "/api/v1/auth/bootstrap/admin",
    "tags": [
      "Auth"
    ],
    "deprecated": false,
    "security": [],
    "parameters": [],
    "responses": [
      {
        "status": "201",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/AuthSessionResponse"
        ],
        "description": "Created platform admin and session."
      },
      {
        "status": "400",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/ErrorResponse"
        ],
        "description": "Invalid email, password, language, or JSON (`bootstrap.invalid_input` or `request.invalid_json`)."
      },
      {
        "status": "403",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/ErrorResponse"
        ],
        "description": "The production bootstrap token is invalid (`bootstrap.token_invalid`)."
      },
      {
        "status": "409",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/ErrorResponse"
        ],
        "description": "Platform admin already exists."
      },
      {
        "status": "503",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/ErrorResponse"
        ],
        "description": "Production bootstrap is unavailable because `BOOTSTRAP_TOKEN` is not configured (`bootstrap.unavailable`)."
      }
    ],
    "summary": "Initialize the first platform admin",
    "requestBody": {
      "required": true,
      "contentTypes": [
        "application/json"
      ],
      "schemaRefs": [
        "#/components/schemas/InitializeAdminInput"
      ]
    },
    "inputSchema": {
      "type": "object",
      "properties": {
        "body": {
          "ref": "#/components/schemas/InitializeAdminInput",
          "type": "object",
          "required": [
            "email",
            "password"
          ],
          "properties": {
            "bootstrapToken": {
              "type": "string",
              "description": "Required when `mode` is `production`; must exactly match the API process `BOOTSTRAP_TOKEN`. Ignored in development.",
              "writeOnly": true
            },
            "email": {
              "type": "string",
              "format": "email"
            },
            "language": {
              "type": "string",
              "enum": [
                "zh-CN",
                "en-US"
              ]
            },
            "name": {
              "type": "string"
            },
            "password": {
              "type": "string",
              "minLength": 8
            },
            "rememberMe": {
              "type": "boolean",
              "description": "When true, also creates a rotating, per-user 30-day HttpOnly remember cookie. The regular session remains valid for 24 hours.",
              "default": false
            }
          }
        }
      },
      "required": [
        "body"
      ],
      "additionalProperties": false
    }
  },
  {
    "method": "post",
    "path": "/api/v1/auth/login",
    "tags": [
      "Auth"
    ],
    "deprecated": false,
    "security": [],
    "parameters": [],
    "responses": [
      {
        "status": "200",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/AuthSessionResponse"
        ],
        "description": "Login succeeded."
      },
      {
        "status": "401",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/ErrorResponse"
        ],
        "description": "Login failed."
      }
    ],
    "summary": "Login with a local account",
    "requestBody": {
      "required": true,
      "contentTypes": [
        "application/json"
      ],
      "schemaRefs": [
        "#/components/schemas/LoginInput"
      ]
    },
    "inputSchema": {
      "type": "object",
      "properties": {
        "body": {
          "ref": "#/components/schemas/LoginInput",
          "type": "object",
          "required": [
            "email",
            "password"
          ],
          "properties": {
            "email": {
              "type": "string",
              "format": "email"
            },
            "password": {
              "type": "string"
            },
            "rememberMe": {
              "type": "boolean",
              "description": "When true, also creates a rotating, per-user 30-day HttpOnly remember cookie. The regular session remains valid for 24 hours.",
              "default": false
            }
          }
        }
      },
      "required": [
        "body"
      ],
      "additionalProperties": false
    }
  },
  {
    "method": "post",
    "path": "/api/v1/auth/login/resume",
    "tags": [
      "Auth"
    ],
    "deprecated": false,
    "security": [],
    "parameters": [],
    "responses": [
      {
        "status": "200",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/AuthSessionResponse"
        ],
        "description": "Remembered login resumed."
      },
      {
        "status": "401",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/ErrorResponse"
        ],
        "description": "Remember token missing, expired, revoked, or the account is disabled."
      }
    ],
    "summary": "Resume login with a remembered account",
    "description": "Rotates the per-user remember token, creates a new 24-hour session, and refreshes the 30-day remember cookie. Browser cookies are required.",
    "requestBody": {
      "required": true,
      "contentTypes": [
        "application/json"
      ],
      "schemaRefs": [
        "#/components/schemas/ResumeLoginInput"
      ]
    },
    "inputSchema": {
      "type": "object",
      "properties": {
        "body": {
          "ref": "#/components/schemas/ResumeLoginInput",
          "type": "object",
          "required": [
            "userId"
          ],
          "properties": {
            "userId": {
              "type": "string",
              "description": "User selected from locally stored recent-account display metadata; authentication still requires that user's HttpOnly remember cookie."
            }
          }
        }
      },
      "required": [
        "body"
      ],
      "additionalProperties": false
    }
  },
  {
    "method": "post",
    "path": "/api/v1/auth/logout",
    "tags": [
      "Auth"
    ],
    "deprecated": false,
    "security": [],
    "parameters": [],
    "responses": [
      {
        "status": "204",
        "contentTypes": [],
        "schemaRefs": [],
        "description": "Logged out."
      }
    ],
    "summary": "Logout current session"
  },
  {
    "method": "get",
    "path": "/api/v1/auth/registration",
    "tags": [
      "Auth"
    ],
    "deprecated": false,
    "security": [],
    "parameters": [],
    "responses": [
      {
        "status": "200",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/AuthRegistrationStatus"
        ],
        "description": "Public registration capability flags."
      }
    ],
    "summary": "Get public registration capabilities"
  },
  {
    "method": "post",
    "path": "/api/v1/auth/registration/email/code",
    "tags": [
      "Auth"
    ],
    "deprecated": false,
    "security": [],
    "parameters": [],
    "responses": [
      {
        "status": "202",
        "contentTypes": [],
        "schemaRefs": [],
        "description": "Verification challenge created."
      }
    ],
    "summary": "Request an email registration verification code",
    "requestBody": {
      "required": true,
      "contentTypes": [
        "application/json"
      ],
      "schemaRefs": [
        "#/components/schemas/EmailRegistrationCodeInput"
      ]
    },
    "inputSchema": {
      "type": "object",
      "properties": {
        "body": {
          "ref": "#/components/schemas/EmailRegistrationCodeInput",
          "type": "object",
          "required": [
            "email"
          ],
          "properties": {
            "email": {
              "type": "string",
              "format": "email"
            },
            "language": {
              "type": "string",
              "enum": [
                "zh-CN",
                "en-US"
              ]
            }
          }
        }
      },
      "required": [
        "body"
      ],
      "additionalProperties": false
    }
  },
  {
    "method": "post",
    "path": "/api/v1/auth/registration/email",
    "tags": [
      "Auth"
    ],
    "deprecated": false,
    "security": [],
    "parameters": [],
    "responses": [
      {
        "status": "201",
        "contentTypes": [],
        "schemaRefs": [],
        "description": "Account created and signed in."
      }
    ],
    "summary": "Complete email registration",
    "requestBody": {
      "required": true,
      "contentTypes": [
        "application/json"
      ],
      "schemaRefs": [
        "#/components/schemas/EmailRegistrationInput"
      ]
    },
    "inputSchema": {
      "type": "object",
      "properties": {
        "body": {
          "ref": "#/components/schemas/EmailRegistrationInput",
          "type": "object",
          "required": [
            "challengeId",
            "code",
            "email",
            "name",
            "password"
          ],
          "properties": {
            "challengeId": {
              "type": "string"
            },
            "code": {
              "type": "string",
              "minLength": 6,
              "maxLength": 6
            },
            "email": {
              "type": "string",
              "format": "email"
            },
            "language": {
              "type": "string",
              "enum": [
                "zh-CN",
                "en-US"
              ]
            },
            "name": {
              "type": "string"
            },
            "password": {
              "type": "string",
              "writeOnly": true,
              "minLength": 8
            },
            "rememberMe": {
              "type": "boolean"
            }
          }
        }
      },
      "required": [
        "body"
      ],
      "additionalProperties": false
    }
  },
  {
    "method": "get",
    "path": "/api/v1/auth/registration/settings",
    "tags": [
      "Auth"
    ],
    "deprecated": false,
    "security": [],
    "parameters": [],
    "responses": [
      {
        "status": "200",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/AuthRegistrationSettings"
        ],
        "description": "Registration settings with the write-only SMTP password omitted."
      }
    ],
    "summary": "Get registration and SMTP settings"
  },
  {
    "method": "put",
    "path": "/api/v1/auth/registration/settings",
    "tags": [
      "Auth"
    ],
    "deprecated": false,
    "security": [],
    "parameters": [],
    "responses": [
      {
        "status": "200",
        "contentTypes": [],
        "schemaRefs": [],
        "description": "Registration settings updated."
      }
    ],
    "summary": "Update registration and SMTP settings",
    "requestBody": {
      "required": true,
      "contentTypes": [
        "application/json"
      ],
      "schemaRefs": [
        "#/components/schemas/AuthRegistrationSettingsInput"
      ]
    },
    "inputSchema": {
      "type": "object",
      "properties": {
        "body": {
          "ref": "#/components/schemas/AuthRegistrationSettingsInput",
          "type": "object",
          "required": [
            "allowEmailRegistration",
            "allowExternalIdentityPassword",
            "allowOidcRegistration",
            "smtpFromAddress",
            "smtpFromName",
            "smtpHost",
            "smtpPort",
            "smtpSecurity",
            "smtpUsername"
          ],
          "properties": {
            "allowEmailRegistration": {
              "type": "boolean"
            },
            "allowExternalIdentityPassword": {
              "type": "boolean"
            },
            "allowOidcRegistration": {
              "type": "boolean"
            },
            "smtpFromAddress": {
              "type": "string",
              "format": "email"
            },
            "smtpFromName": {
              "type": "string"
            },
            "smtpHost": {
              "type": "string"
            },
            "smtpPassword": {
              "type": "string",
              "description": "Leave empty to keep the existing Secret Store value.",
              "writeOnly": true
            },
            "smtpPort": {
              "type": "integer",
              "minimum": 1,
              "maximum": 65535
            },
            "smtpSecurity": {
              "type": "string",
              "enum": [
                "none",
                "starttls",
                "tls"
              ]
            },
            "smtpUsername": {
              "type": "string"
            }
          }
        }
      },
      "required": [
        "body"
      ],
      "additionalProperties": false
    }
  },
  {
    "method": "get",
    "path": "/api/v1/auth/mfa/status",
    "tags": [
      "Auth"
    ],
    "deprecated": false,
    "security": [
      {
        "SessionCookie": []
      }
    ],
    "parameters": [],
    "responses": [
      {
        "status": "200",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/MFAStatus"
        ],
        "description": "Current enrollment, policy, and recovery-code status."
      },
      {
        "status": "401",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/ErrorResponse"
        ],
        "description": "Browser session is missing or invalid (`mfa.session_required` or an authentication error)."
      },
      {
        "status": "403",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/ErrorResponse"
        ],
        "description": "Personal access tokens cannot access MFA session endpoints (`mfa.session_required`)."
      },
      {
        "status": "500",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/ErrorResponse"
        ],
        "description": "MFA status could not be loaded."
      }
    ],
    "summary": "Get current user's MFA status",
    "description": "Requires an interactive browser session. Personal access tokens cannot manage or verify MFA."
  },
  {
    "method": "post",
    "path": "/api/v1/auth/mfa/totp/enroll",
    "tags": [
      "Auth"
    ],
    "deprecated": false,
    "security": [
      {
        "SessionCookie": []
      }
    ],
    "parameters": [],
    "responses": [
      {
        "status": "201",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/MFAEnrollment"
        ],
        "description": "Pending TOTP enrollment created."
      },
      {
        "status": "401",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/ErrorResponse"
        ],
        "description": "Browser session is missing or invalid, or primary reauthentication is required (`mfa.reauth_required`)."
      },
      {
        "status": "403",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/ErrorResponse"
        ],
        "description": "Personal access tokens cannot enroll MFA (`mfa.session_required`)."
      },
      {
        "status": "409",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/ErrorResponse"
        ],
        "description": "MFA is already enabled (`mfa.already_enabled`)."
      },
      {
        "status": "429",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/ErrorResponse"
        ],
        "description": "Enrollment attempts exceeded the user or IP rate limit (`mfa.rate_limited`)."
      },
      {
        "status": "500",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/ErrorResponse"
        ],
        "description": "The TOTP secret could not be stored (`mfa.secret_store_failed`) or enrollment persistence failed."
      },
      {
        "status": "503",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/ErrorResponse"
        ],
        "description": "MFA rate limiting is unavailable in production (`mfa.rate_limit_unavailable`)."
      }
    ],
    "summary": "Start TOTP enrollment",
    "description": "Replaces any pending enrollment, stores the TOTP secret in the encrypted secret store, and returns the secret only for the current enrollment flow. Local accounts must re-enter their current password. OIDC accounts require non-impersonated primary authentication within the last five minutes; remember-token recovery does not refresh that timestamp.",
    "requestBody": {
      "required": true,
      "contentTypes": [
        "application/json"
      ],
      "schemaRefs": [
        "#/components/schemas/MFAEnrollmentInput"
      ]
    },
    "inputSchema": {
      "type": "object",
      "properties": {
        "body": {
          "ref": "#/components/schemas/MFAEnrollmentInput",
          "type": "object",
          "properties": {
            "currentPassword": {
              "type": "string",
              "format": "password",
              "description": "Required for local accounts and ignored for OIDC accounts.",
              "writeOnly": true
            }
          }
        }
      },
      "required": [
        "body"
      ],
      "additionalProperties": false
    }
  },
  {
    "method": "post",
    "path": "/api/v1/auth/mfa/totp/confirm",
    "tags": [
      "Auth"
    ],
    "deprecated": false,
    "security": [
      {
        "SessionCookie": []
      }
    ],
    "parameters": [],
    "responses": [
      {
        "status": "200",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/MFAConfirmResult"
        ],
        "description": "MFA enabled and recovery codes generated."
      },
      {
        "status": "400",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/ErrorResponse"
        ],
        "description": "Invalid request body (`request.invalid_json`)."
      },
      {
        "status": "401",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/ErrorResponse"
        ],
        "description": "Browser session is invalid or the TOTP code is invalid (`mfa.invalid_code`)."
      },
      {
        "status": "403",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/ErrorResponse"
        ],
        "description": "Personal access tokens cannot confirm MFA (`mfa.session_required`)."
      },
      {
        "status": "409",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/ErrorResponse"
        ],
        "description": "Enrollment is missing, changed, or already enabled (`mfa.enrollment_required`, `mfa.enrollment_changed`, or `mfa.already_enabled`)."
      },
      {
        "status": "429",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/ErrorResponse"
        ],
        "description": "Confirmation attempts exceeded the user or IP rate limit (`mfa.rate_limited`)."
      },
      {
        "status": "500",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/ErrorResponse"
        ],
        "description": "Recovery codes or enrollment state could not be persisted."
      },
      {
        "status": "503",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/ErrorResponse"
        ],
        "description": "MFA rate limiting is unavailable in production (`mfa.rate_limit_unavailable`)."
      }
    ],
    "summary": "Confirm pending TOTP enrollment",
    "description": "Accepts the current or adjacent 30-second TOTP window. On success, enables MFA and returns ten one-time recovery codes that are shown only once.",
    "requestBody": {
      "required": true,
      "contentTypes": [
        "application/json"
      ],
      "schemaRefs": [
        "#/components/schemas/MFAConfirmInput"
      ]
    },
    "inputSchema": {
      "type": "object",
      "properties": {
        "body": {
          "ref": "#/components/schemas/MFAConfirmInput",
          "type": "object",
          "required": [
            "code"
          ],
          "properties": {
            "code": {
              "type": "string",
              "pattern": "^[0-9]{6}$"
            }
          }
        }
      },
      "required": [
        "body"
      ],
      "additionalProperties": false
    }
  },
  {
    "method": "post",
    "path": "/api/v1/auth/mfa/verify",
    "tags": [
      "Auth"
    ],
    "deprecated": false,
    "security": [
      {
        "SessionCookie": []
      }
    ],
    "parameters": [],
    "responses": [
      {
        "status": "200",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/MFAVerifyResult"
        ],
        "description": "Step-up assertion created."
      },
      {
        "status": "400",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/ErrorResponse"
        ],
        "description": "Unsupported purpose or both/neither credentials were supplied (`mfa.invalid_purpose` or `mfa.credential_required`)."
      },
      {
        "status": "401",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/ErrorResponse"
        ],
        "description": "Browser session or MFA credential is invalid (`mfa.invalid_code`)."
      },
      {
        "status": "403",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/ErrorResponse"
        ],
        "description": "Personal access tokens cannot create MFA assertions (`mfa.session_required`)."
      },
      {
        "status": "409",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/ErrorResponse"
        ],
        "description": "MFA is not enabled for the current user (`mfa.not_enabled`)."
      },
      {
        "status": "429",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/ErrorResponse"
        ],
        "description": "Verification attempts exceeded the user or IP rate limit (`mfa.rate_limited`)."
      },
      {
        "status": "500",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/ErrorResponse"
        ],
        "description": "The Step-up assertion could not be persisted."
      },
      {
        "status": "503",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/ErrorResponse"
        ],
        "description": "MFA rate limiting is unavailable in production (`mfa.rate_limit_unavailable`)."
      }
    ],
    "summary": "Verify MFA for a sensitive-operation purpose",
    "description": "Accepts exactly one TOTP code or one recovery code. A successful recovery code is consumed atomically. The resulting assertion is bound to the current user, browser session, and purpose.",
    "requestBody": {
      "required": true,
      "contentTypes": [
        "application/json"
      ],
      "schemaRefs": [
        "#/components/schemas/MFAVerifyInput"
      ]
    },
    "inputSchema": {
      "type": "object",
      "properties": {
        "body": {
          "ref": "#/components/schemas/MFAVerifyInput",
          "type": "object",
          "required": [
            "purpose"
          ],
          "properties": {
            "code": {
              "type": "string",
              "pattern": "^[0-9]{6}$"
            },
            "purpose": {
              "ref": "#/components/schemas/MFAPurpose",
              "type": "string",
              "enum": [
                "runtime_exec",
                "runtime_terminal",
                "data_export",
                "secret_update",
                "registry_credential_update",
                "kubeconfig_update",
                "auth_provider_update",
                "user_admin_update",
                "mfa_manage",
                "security_settings_update",
                "data_retention_cleanup",
                "password_update",
                "access_token_manage"
              ]
            },
            "recoveryCode": {
              "type": "string",
              "description": "One-time recovery code. Hyphens and case are normalized before verification."
            }
          },
          "oneOf": [
            {
              "required": [
                "code"
              ],
              "not": {
                "required": [
                  "recoveryCode"
                ]
              }
            },
            {
              "required": [
                "recoveryCode"
              ],
              "not": {
                "required": [
                  "code"
                ]
              }
            }
          ]
        }
      },
      "required": [
        "body"
      ],
      "additionalProperties": false
    }
  },
  {
    "method": "post",
    "path": "/api/v1/auth/mfa/recovery-codes",
    "tags": [
      "Auth"
    ],
    "deprecated": false,
    "security": [
      {
        "SessionCookie": []
      }
    ],
    "parameters": [],
    "responses": [
      {
        "status": "200",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/MFARecoveryCodes"
        ],
        "description": "Recovery codes replaced."
      },
      {
        "status": "401",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/ErrorResponse"
        ],
        "description": "Browser session is missing or invalid."
      },
      {
        "status": "403",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/ErrorResponse"
        ],
        "description": "MFA management assertion is missing or expired (`mfa_required`), or a personal access token was used (`mfa.session_required`)."
      },
      {
        "status": "409",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/ErrorResponse"
        ],
        "description": "MFA is not enabled (`mfa.not_enabled`)."
      },
      {
        "status": "500",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/ErrorResponse"
        ],
        "description": "Recovery codes could not be generated or persisted."
      }
    ],
    "summary": "Regenerate MFA recovery codes",
    "description": "Requires a valid `mfa_manage` assertion. Replaces and invalidates all previous recovery codes; the new plaintext codes are returned only once."
  },
  {
    "method": "delete",
    "path": "/api/v1/auth/mfa",
    "tags": [
      "Auth"
    ],
    "deprecated": false,
    "security": [
      {
        "SessionCookie": []
      }
    ],
    "parameters": [],
    "responses": [
      {
        "status": "204",
        "contentTypes": [],
        "schemaRefs": [],
        "description": "MFA disabled and assertions revoked."
      },
      {
        "status": "401",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/ErrorResponse"
        ],
        "description": "Browser session is missing or invalid."
      },
      {
        "status": "403",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/ErrorResponse"
        ],
        "description": "MFA management assertion is missing or expired (`mfa_required`), or a personal access token was used (`mfa.session_required`)."
      },
      {
        "status": "409",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/ErrorResponse"
        ],
        "description": "The global policy requires another MFA-enabled platform administrator (`mfa.last_admin_required`)."
      },
      {
        "status": "500",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/ErrorResponse"
        ],
        "description": "MFA state or encrypted secret data could not be deleted."
      }
    ],
    "summary": "Disable current user's MFA",
    "description": "Requires a valid `mfa_manage` assertion. Deletes the TOTP secret, recovery codes, and all current step-up assertions. While the global policy is enabled, the last MFA-enabled platform administrator cannot disable MFA."
  },
  {
    "method": "get",
    "path": "/api/v1/auth/providers",
    "tags": [
      "Auth"
    ],
    "deprecated": false,
    "security": [],
    "parameters": [],
    "responses": [
      {
        "status": "200",
        "contentTypes": [],
        "schemaRefs": [],
        "description": "Auth provider list."
      }
    ],
    "summary": "List auth providers"
  },
  {
    "method": "post",
    "path": "/api/v1/auth/providers",
    "tags": [
      "Auth"
    ],
    "deprecated": false,
    "security": [],
    "parameters": [],
    "responses": [
      {
        "status": "201",
        "contentTypes": [],
        "schemaRefs": [],
        "description": "Created auth provider."
      }
    ],
    "summary": "Create auth provider",
    "requestBody": {
      "required": true,
      "contentTypes": [
        "application/json"
      ],
      "schemaRefs": [
        "#/components/schemas/AuthProviderInput"
      ]
    },
    "inputSchema": {
      "type": "object",
      "properties": {
        "body": {
          "ref": "#/components/schemas/AuthProviderInput",
          "type": "object",
          "required": [
            "clientId",
            "issuerUrl",
            "name"
          ],
          "properties": {
            "clientId": {
              "type": "string"
            },
            "clientSecret": {
              "type": "string"
            },
            "emailClaim": {
              "type": "string"
            },
            "enabled": {
              "type": "boolean"
            },
            "groupClaim": {
              "type": "string"
            },
            "isDefault": {
              "type": "boolean"
            },
            "issuerUrl": {
              "type": "string"
            },
            "name": {
              "type": "string"
            },
            "scopes": {
              "type": "string"
            },
            "type": {
              "type": "string",
              "enum": [
                "oidc"
              ]
            },
            "usernameClaim": {
              "type": "string"
            }
          }
        }
      },
      "required": [
        "body"
      ],
      "additionalProperties": false
    }
  },
  {
    "method": "put",
    "path": "/api/v1/auth/providers/{providerId}",
    "tags": [
      "Auth"
    ],
    "deprecated": false,
    "security": [],
    "parameters": [
      {
        "name": "providerId",
        "in": "path",
        "required": true,
        "ref": "#/components/parameters/ProviderId",
        "schema": {
          "type": "string"
        }
      }
    ],
    "responses": [
      {
        "status": "200",
        "contentTypes": [],
        "schemaRefs": [],
        "description": "Updated auth provider."
      }
    ],
    "summary": "Update auth provider",
    "requestBody": {
      "required": true,
      "contentTypes": [
        "application/json"
      ],
      "schemaRefs": [
        "#/components/schemas/AuthProviderInput"
      ]
    },
    "inputSchema": {
      "type": "object",
      "properties": {
        "providerId": {
          "type": "string"
        },
        "body": {
          "ref": "#/components/schemas/AuthProviderInput",
          "type": "object",
          "required": [
            "clientId",
            "issuerUrl",
            "name"
          ],
          "properties": {
            "clientId": {
              "type": "string"
            },
            "clientSecret": {
              "type": "string"
            },
            "emailClaim": {
              "type": "string"
            },
            "enabled": {
              "type": "boolean"
            },
            "groupClaim": {
              "type": "string"
            },
            "isDefault": {
              "type": "boolean"
            },
            "issuerUrl": {
              "type": "string"
            },
            "name": {
              "type": "string"
            },
            "scopes": {
              "type": "string"
            },
            "type": {
              "type": "string",
              "enum": [
                "oidc"
              ]
            },
            "usernameClaim": {
              "type": "string"
            }
          }
        }
      },
      "required": [
        "body",
        "providerId"
      ],
      "additionalProperties": false
    }
  },
  {
    "method": "get",
    "path": "/api/v1/auth/admission-policy",
    "tags": [
      "Auth"
    ],
    "deprecated": false,
    "security": [],
    "parameters": [],
    "responses": [
      {
        "status": "200",
        "contentTypes": [],
        "schemaRefs": [],
        "description": "Auth admission policy."
      }
    ],
    "summary": "Get auth admission policy"
  },
  {
    "method": "put",
    "path": "/api/v1/auth/admission-policy",
    "tags": [
      "Auth"
    ],
    "deprecated": false,
    "security": [],
    "parameters": [],
    "responses": [
      {
        "status": "200",
        "contentTypes": [],
        "schemaRefs": [],
        "description": "Updated auth admission policy."
      }
    ],
    "summary": "Update auth admission policy",
    "requestBody": {
      "required": true,
      "contentTypes": [
        "application/json"
      ],
      "schemaRefs": [
        "#/components/schemas/AuthAdmissionPolicyInput"
      ]
    },
    "inputSchema": {
      "type": "object",
      "properties": {
        "body": {
          "ref": "#/components/schemas/AuthAdmissionPolicyInput",
          "type": "object",
          "properties": {
            "allowedEmailDomains": {
              "type": "array",
              "items": {
                "type": "string"
              }
            },
            "allowedOidcGroups": {
              "type": "array",
              "items": {
                "type": "string"
              }
            },
            "allowLocalLogin": {
              "type": "boolean"
            },
            "allowOidcLogin": {
              "type": "boolean"
            },
            "defaultRole": {
              "type": "string",
              "enum": [
                "platform_admin",
                "user"
              ]
            },
            "invitedEmails": {
              "type": "array",
              "items": {
                "type": "string"
              }
            }
          }
        }
      },
      "required": [
        "body"
      ],
      "additionalProperties": false
    }
  },
  {
    "method": "get",
    "path": "/api/v1/auth/oidc/{providerId}/start",
    "tags": [
      "Auth"
    ],
    "deprecated": false,
    "security": [],
    "parameters": [
      {
        "name": "providerId",
        "in": "path",
        "required": true,
        "ref": "#/components/parameters/ProviderId",
        "schema": {
          "type": "string"
        }
      },
      {
        "name": "mode",
        "in": "query",
        "schema": {
          "type": "string",
          "enum": [
            "login",
            "bind"
          ],
          "default": "login"
        }
      },
      {
        "name": "redirect",
        "in": "query",
        "schema": {
          "type": "string",
          "default": "/projects"
        }
      }
    ],
    "responses": [
      {
        "status": "302",
        "contentTypes": [],
        "schemaRefs": [],
        "description": "Redirect to OIDC provider."
      }
    ],
    "summary": "Start OIDC login or binding flow",
    "inputSchema": {
      "type": "object",
      "properties": {
        "providerId": {
          "type": "string"
        },
        "mode": {
          "type": "string",
          "enum": [
            "login",
            "bind"
          ],
          "default": "login"
        },
        "redirect": {
          "type": "string",
          "default": "/projects"
        }
      },
      "required": [
        "providerId"
      ],
      "additionalProperties": false
    }
  },
  {
    "method": "get",
    "path": "/api/v1/auth/oidc/callback",
    "tags": [
      "Auth"
    ],
    "deprecated": false,
    "security": [],
    "parameters": [
      {
        "name": "state",
        "in": "query",
        "required": true,
        "ref": "#/components/parameters/OAuthState",
        "schema": {
          "type": "string"
        }
      },
      {
        "name": "code",
        "in": "query",
        "required": true,
        "ref": "#/components/parameters/OAuthCode",
        "schema": {
          "type": "string"
        }
      }
    ],
    "responses": [
      {
        "status": "302",
        "contentTypes": [],
        "schemaRefs": [],
        "description": "Redirect after OIDC callback."
      }
    ],
    "summary": "Complete OIDC callback",
    "inputSchema": {
      "type": "object",
      "properties": {
        "state": {
          "type": "string"
        },
        "code": {
          "type": "string"
        }
      },
      "required": [
        "code",
        "state"
      ],
      "additionalProperties": false
    }
  },
  {
    "method": "get",
    "path": "/api/v1/users/me",
    "tags": [
      "Users"
    ],
    "deprecated": false,
    "security": [],
    "parameters": [],
    "responses": [
      {
        "status": "200",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/CurrentUser"
        ],
        "description": "Current user."
      }
    ],
    "summary": "Get current user"
  },
  {
    "method": "put",
    "path": "/api/v1/users/me",
    "tags": [
      "Users"
    ],
    "deprecated": false,
    "security": [],
    "parameters": [],
    "responses": [
      {
        "status": "200",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/CurrentUser"
        ],
        "description": "Updated current user."
      }
    ],
    "summary": "Update current user preferences",
    "requestBody": {
      "required": true,
      "contentTypes": [
        "application/json"
      ],
      "schemaRefs": [
        "#/components/schemas/UpdateCurrentUserInput"
      ]
    },
    "inputSchema": {
      "type": "object",
      "properties": {
        "body": {
          "ref": "#/components/schemas/UpdateCurrentUserInput",
          "type": "object",
          "properties": {
            "avatarUrl": {
              "type": "string"
            },
            "brandColorPreset": {
              "type": "string",
              "description": "Empty follows the platform color theme; otherwise stores a curated multi-color theme or official Radix single-color preset ID.",
              "enum": [
                "",
                "gold",
                "bronze",
                "brown",
                "yellow",
                "amber",
                "orange",
                "tomato",
                "red",
                "ruby",
                "crimson",
                "pink",
                "plum",
                "purple",
                "violet",
                "iris",
                "indigo",
                "blue",
                "cyan",
                "teal",
                "jade",
                "green",
                "grass",
                "lime",
                "mint",
                "sky"
              ]
            },
            "interfaceStyle": {
              "type": "string",
              "description": "Empty follows the platform default; otherwise overrides the interface style.",
              "enum": [
                "",
                "minimal",
                "themed"
              ]
            },
            "language": {
              "type": "string",
              "enum": [
                "zh-CN",
                "en-US"
              ]
            },
            "name": {
              "type": "string"
            }
          }
        }
      },
      "required": [
        "body"
      ],
      "additionalProperties": false
    }
  },
  {
    "method": "put",
    "path": "/api/v1/users/me/password",
    "tags": [
      "Users"
    ],
    "deprecated": false,
    "security": [],
    "parameters": [],
    "responses": [
      {
        "status": "204",
        "contentTypes": [],
        "schemaRefs": [],
        "description": "Password updated and all sessions revoked."
      }
    ],
    "summary": "Set or change the current user's local password",
    "requestBody": {
      "required": true,
      "contentTypes": [
        "application/json"
      ],
      "schemaRefs": [
        "#/components/schemas/UpdateMyPasswordInput"
      ]
    },
    "inputSchema": {
      "type": "object",
      "properties": {
        "body": {
          "ref": "#/components/schemas/UpdateMyPasswordInput",
          "type": "object",
          "required": [
            "newPassword"
          ],
          "properties": {
            "currentPassword": {
              "type": "string",
              "writeOnly": true
            },
            "newPassword": {
              "type": "string",
              "writeOnly": true,
              "minLength": 8
            }
          }
        }
      },
      "required": [
        "body"
      ],
      "additionalProperties": false
    }
  },
  {
    "method": "get",
    "path": "/api/v1/users/me/external-identities",
    "tags": [
      "Users"
    ],
    "deprecated": false,
    "security": [],
    "parameters": [],
    "responses": [
      {
        "status": "200",
        "contentTypes": [],
        "schemaRefs": [],
        "description": "External identity list."
      }
    ],
    "summary": "List current user's external identities"
  },
  {
    "method": "delete",
    "path": "/api/v1/users/me/external-identities/{identityId}",
    "tags": [
      "Users"
    ],
    "deprecated": false,
    "security": [],
    "parameters": [
      {
        "name": "identityId",
        "in": "path",
        "required": true,
        "ref": "#/components/parameters/IdentityId",
        "schema": {
          "type": "string"
        }
      }
    ],
    "responses": [
      {
        "status": "204",
        "contentTypes": [],
        "schemaRefs": [],
        "description": "External identity unbound."
      }
    ],
    "summary": "Unbind current user's external identity",
    "inputSchema": {
      "type": "object",
      "properties": {
        "identityId": {
          "type": "string"
        }
      },
      "required": [
        "identityId"
      ],
      "additionalProperties": false
    }
  },
  {
    "method": "get",
    "path": "/api/v1/users",
    "tags": [
      "Users"
    ],
    "deprecated": false,
    "security": [],
    "parameters": [
      {
        "name": "page",
        "in": "query",
        "ref": "#/components/parameters/Page",
        "schema": {
          "type": "integer",
          "default": 1,
          "minimum": 1
        }
      },
      {
        "name": "pageSize",
        "in": "query",
        "ref": "#/components/parameters/PageSize",
        "schema": {
          "type": "integer",
          "default": 20,
          "minimum": 1,
          "maximum": 100
        }
      },
      {
        "name": "sortBy",
        "in": "query",
        "schema": {
          "type": "string",
          "enum": [
            "createdAt",
            "email",
            "name",
            "role",
            "passwordSet",
            "status"
          ],
          "default": "createdAt"
        }
      },
      {
        "name": "sortOrder",
        "in": "query",
        "ref": "#/components/parameters/SortOrder",
        "schema": {
          "type": "string",
          "enum": [
            "asc",
            "desc"
          ],
          "default": "desc"
        }
      }
    ],
    "responses": [
      {
        "status": "200",
        "contentTypes": [],
        "schemaRefs": [],
        "description": "Paginated user list."
      }
    ],
    "summary": "List users",
    "inputSchema": {
      "type": "object",
      "properties": {
        "page": {
          "type": "integer",
          "default": 1,
          "minimum": 1
        },
        "pageSize": {
          "type": "integer",
          "default": 20,
          "minimum": 1,
          "maximum": 100
        },
        "sortBy": {
          "type": "string",
          "enum": [
            "createdAt",
            "email",
            "name",
            "role",
            "passwordSet",
            "status"
          ],
          "default": "createdAt"
        },
        "sortOrder": {
          "type": "string",
          "enum": [
            "asc",
            "desc"
          ],
          "default": "desc"
        }
      },
      "additionalProperties": false
    }
  },
  {
    "method": "post",
    "path": "/api/v1/users",
    "tags": [
      "Users"
    ],
    "deprecated": false,
    "security": [],
    "parameters": [],
    "responses": [
      {
        "status": "201",
        "contentTypes": [],
        "schemaRefs": [],
        "description": "Created user."
      }
    ],
    "summary": "Create local user",
    "requestBody": {
      "required": true,
      "contentTypes": [
        "application/json"
      ],
      "schemaRefs": [
        "#/components/schemas/UserInput"
      ]
    },
    "inputSchema": {
      "type": "object",
      "properties": {
        "body": {
          "ref": "#/components/schemas/UserInput",
          "type": "object",
          "required": [
            "email",
            "name"
          ],
          "properties": {
            "disabled": {
              "type": "boolean"
            },
            "email": {
              "type": "string"
            },
            "language": {
              "type": "string",
              "enum": [
                "zh-CN",
                "en-US"
              ]
            },
            "name": {
              "type": "string"
            },
            "password": {
              "type": "string"
            },
            "role": {
              "type": "string",
              "enum": [
                "platform_admin",
                "user"
              ]
            }
          }
        }
      },
      "required": [
        "body"
      ],
      "additionalProperties": false
    }
  },
  {
    "method": "put",
    "path": "/api/v1/users/{userId}",
    "tags": [
      "Users"
    ],
    "deprecated": false,
    "security": [],
    "parameters": [
      {
        "name": "userId",
        "in": "path",
        "required": true,
        "ref": "#/components/parameters/UserId",
        "schema": {
          "type": "string"
        }
      }
    ],
    "responses": [
      {
        "status": "200",
        "contentTypes": [],
        "schemaRefs": [],
        "description": "Updated user."
      }
    ],
    "summary": "Update user",
    "requestBody": {
      "required": true,
      "contentTypes": [
        "application/json"
      ],
      "schemaRefs": [
        "#/components/schemas/UserInput"
      ]
    },
    "inputSchema": {
      "type": "object",
      "properties": {
        "userId": {
          "type": "string"
        },
        "body": {
          "ref": "#/components/schemas/UserInput",
          "type": "object",
          "required": [
            "email",
            "name"
          ],
          "properties": {
            "disabled": {
              "type": "boolean"
            },
            "email": {
              "type": "string"
            },
            "language": {
              "type": "string",
              "enum": [
                "zh-CN",
                "en-US"
              ]
            },
            "name": {
              "type": "string"
            },
            "password": {
              "type": "string"
            },
            "role": {
              "type": "string",
              "enum": [
                "platform_admin",
                "user"
              ]
            }
          }
        }
      },
      "required": [
        "body",
        "userId"
      ],
      "additionalProperties": false
    }
  },
  {
    "method": "delete",
    "path": "/api/v1/users/{userId}/mfa",
    "tags": [
      "Users"
    ],
    "deprecated": false,
    "security": [
      {
        "SessionCookie": []
      }
    ],
    "parameters": [
      {
        "name": "userId",
        "in": "path",
        "required": true,
        "ref": "#/components/parameters/UserId",
        "schema": {
          "type": "string"
        }
      }
    ],
    "responses": [
      {
        "status": "204",
        "contentTypes": [],
        "schemaRefs": [],
        "description": "Target MFA state reset."
      },
      {
        "status": "401",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/ErrorResponse"
        ],
        "description": "Interactive browser session is missing or invalid."
      },
      {
        "status": "403",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/ErrorResponse"
        ],
        "description": "Platform-administrator role or `user_admin_update` Step-up verification is required."
      },
      {
        "status": "404",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/ErrorResponse"
        ],
        "description": "Target user or MFA enrollment was not found (`mfa.reset_target_not_found`)."
      },
      {
        "status": "409",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/ErrorResponse"
        ],
        "description": "Self-reset is forbidden (`mfa.admin_reset_self_forbidden`) or the target is the last MFA-enabled platform administrator (`mfa.last_admin_required`)."
      }
    ],
    "summary": "Reset another user's MFA enrollment",
    "description": "Requires an interactive platform-administrator session and an active `user_admin_update` Step-up assertion. Deletes the target user's authenticator secret, recovery codes, and active Step-up assertions. Administrators cannot reset their own MFA through this endpoint and cannot remove the last enabled administrator MFA while the global policy is active.",
    "inputSchema": {
      "type": "object",
      "properties": {
        "userId": {
          "type": "string"
        }
      },
      "required": [
        "userId"
      ],
      "additionalProperties": false
    }
  },
  {
    "method": "get",
    "path": "/api/v1/configs/definitions",
    "tags": [
      "Configs"
    ],
    "deprecated": false,
    "security": [],
    "parameters": [],
    "responses": [
      {
        "status": "200",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/ConfigDefinition"
        ],
        "description": "Config definitions."
      }
    ],
    "summary": "List configurable app config definitions"
  },
  {
    "method": "put",
    "path": "/api/v1/configs",
    "tags": [
      "Configs"
    ],
    "deprecated": false,
    "security": [],
    "parameters": [],
    "responses": [
      {
        "status": "200",
        "contentTypes": [],
        "schemaRefs": [],
        "description": "Updated config dictionary."
      }
    ],
    "summary": "Update app configs",
    "requestBody": {
      "required": true,
      "contentTypes": [
        "application/json"
      ],
      "schemaRefs": [
        "#/components/schemas/UpdateConfigsInput"
      ]
    },
    "inputSchema": {
      "type": "object",
      "properties": {
        "body": {
          "ref": "#/components/schemas/UpdateConfigsInput",
          "type": "object",
          "required": [
            "values"
          ],
          "properties": {
            "values": {
              "type": "object",
              "additionalProperties": {
                "oneOf": [
                  {
                    "type": "string"
                  },
                  {
                    "type": "number"
                  },
                  {
                    "type": "boolean"
                  },
                  {
                    "type": "object"
                  },
                  {
                    "type": "array"
                  }
                ]
              }
            }
          }
        }
      },
      "required": [
        "body"
      ],
      "additionalProperties": false
    }
  },
  {
    "method": "get",
    "path": "/api/v1/data-retention/catalog",
    "tags": [
      "DataRetention"
    ],
    "deprecated": false,
    "security": [
      {
        "SessionCookie": []
      }
    ],
    "parameters": [],
    "responses": [
      {
        "status": "200",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/DataRetentionCatalogResponse"
        ],
        "description": "Retention dataset catalog."
      },
      {
        "status": "403",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/ErrorResponse"
        ],
        "description": "Platform-administrator role is required."
      }
    ],
    "summary": "List supported data-retention datasets",
    "description": "Returns the fixed cleanup catalog. Audit logs, billing data, and build, release, or Hook metadata are intentionally excluded.",
    "operationId": "listDataRetentionCatalog",
    "xLunaCli": {
      "command": "retention.catalog",
      "classification": "business-command",
      "risk": "low",
      "requiredScopes": [
        "retention:read"
      ]
    }
  },
  {
    "method": "post",
    "path": "/api/v1/data-retention/preview",
    "tags": [
      "DataRetention"
    ],
    "deprecated": false,
    "security": [
      {
        "SessionCookie": []
      }
    ],
    "parameters": [],
    "responses": [
      {
        "status": "200",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/DataRetentionResultResponse"
        ],
        "description": "Matching counts by dataset. `deleted` is zero for a preview."
      },
      {
        "status": "400",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/ErrorResponse"
        ],
        "description": "Invalid range or unknown dataset (`retention.invalid_range` or `retention.invalid_dataset`)."
      },
      {
        "status": "403",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/ErrorResponse"
        ],
        "description": "Platform-administrator role is required."
      }
    ],
    "summary": "Preview data matching a retention range",
    "description": "Counts rows without changing data. The selected range is left-closed and right-open (`startAt <= timestamp < endAt`). Active runtime records and protected datasets are never matched.",
    "operationId": "previewDataRetention",
    "requestBody": {
      "required": true,
      "contentTypes": [
        "application/json"
      ],
      "schemaRefs": [
        "#/components/schemas/DataRetentionRequest"
      ]
    },
    "inputSchema": {
      "type": "object",
      "properties": {
        "body": {
          "ref": "#/components/schemas/DataRetentionRequest",
          "type": "object",
          "required": [
            "datasets",
            "endAt",
            "startAt"
          ],
          "properties": {
            "datasets": {
              "type": "array",
              "items": {
                "type": "string",
                "enum": [
                  "platform_events",
                  "notification_deliveries",
                  "worker_task_events",
                  "build_logs",
                  "release_logs",
                  "hook_run_logs",
                  "expired_auth_data"
                ]
              },
              "minItems": 1,
              "uniqueItems": true
            },
            "endAt": {
              "type": "string",
              "format": "date-time"
            },
            "startAt": {
              "type": "string",
              "format": "date-time"
            }
          }
        }
      },
      "required": [
        "body"
      ],
      "additionalProperties": false
    },
    "xLunaCli": {
      "command": "retention.preview",
      "classification": "business-command",
      "risk": "low",
      "requiredScopes": [
        "retention:read"
      ]
    }
  },
  {
    "method": "post",
    "path": "/api/v1/data-retention/cleanup",
    "tags": [
      "DataRetention"
    ],
    "deprecated": false,
    "security": [
      {
        "SessionCookie": []
      }
    ],
    "parameters": [],
    "responses": [
      {
        "status": "200",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/DataRetentionResultResponse"
        ],
        "description": "Matched and deleted counts by dataset."
      },
      {
        "status": "400",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/ErrorResponse"
        ],
        "description": "Invalid range or unknown dataset (`retention.invalid_range` or `retention.invalid_dataset`)."
      },
      {
        "status": "403",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/ErrorResponse"
        ],
        "description": "Platform-administrator role or Step-up MFA assertion is required."
      },
      {
        "status": "500",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/ErrorResponse"
        ],
        "description": "Cleanup failed (`retention.cleanup_failed`)."
      }
    ],
    "summary": "Permanently remove data matching a retention range",
    "description": "Runs the same fixed whitelist and protection rules as preview, then writes only the aggregate result to the audit log. The operation does not accept table names or SQL expressions. When Step-up MFA is enabled, a valid `data_retention_cleanup` assertion is also required.",
    "operationId": "cleanupDataRetention",
    "requestBody": {
      "required": true,
      "contentTypes": [
        "application/json"
      ],
      "schemaRefs": [
        "#/components/schemas/DataRetentionRequest"
      ]
    },
    "inputSchema": {
      "type": "object",
      "properties": {
        "body": {
          "ref": "#/components/schemas/DataRetentionRequest",
          "type": "object",
          "required": [
            "datasets",
            "endAt",
            "startAt"
          ],
          "properties": {
            "datasets": {
              "type": "array",
              "items": {
                "type": "string",
                "enum": [
                  "platform_events",
                  "notification_deliveries",
                  "worker_task_events",
                  "build_logs",
                  "release_logs",
                  "hook_run_logs",
                  "expired_auth_data"
                ]
              },
              "minItems": 1,
              "uniqueItems": true
            },
            "endAt": {
              "type": "string",
              "format": "date-time"
            },
            "startAt": {
              "type": "string",
              "format": "date-time"
            }
          }
        }
      },
      "required": [
        "body"
      ],
      "additionalProperties": false
    },
    "xLunaCli": {
      "command": "retention.cleanup",
      "classification": "business-command",
      "risk": "critical",
      "requiredScopes": [
        "retention:manage"
      ]
    }
  },
  {
    "method": "post",
    "path": "/api/v1/runtime/clusters/{clusterId}/pods/terminal/authorize",
    "tags": [
      "Runtime"
    ],
    "deprecated": false,
    "security": [
      {
        "SessionCookie": []
      }
    ],
    "parameters": [
      {
        "name": "clusterId",
        "in": "path",
        "required": true,
        "ref": "#/components/parameters/ClusterId",
        "schema": {
          "type": "string"
        }
      },
      {
        "name": "namespace",
        "in": "query",
        "required": true,
        "schema": {
          "type": "string"
        }
      },
      {
        "name": "name",
        "in": "query",
        "required": true,
        "schema": {
          "type": "string"
        }
      }
    ],
    "responses": [
      {
        "status": "204",
        "contentTypes": [],
        "schemaRefs": [],
        "description": "Terminal preflight authorized. The WebSocket endpoint must still perform its own authorization checks."
      },
      {
        "status": "400",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/ErrorResponse"
        ],
        "description": "Pod namespace or name is empty."
      },
      {
        "status": "401",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/ErrorResponse"
        ],
        "description": "Interactive browser session is missing or invalid."
      },
      {
        "status": "403",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/ErrorResponse"
        ],
        "description": "The current user is not a platform administrator, a personal access token was used (`mfa.session_required`), or Step-up verification is required (`mfa_required` with purpose `runtime_terminal`)."
      },
      {
        "status": "404",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/ErrorResponse"
        ],
        "description": "Runtime cluster was not found."
      }
    ],
    "summary": "Authorize a runtime-cluster Pod terminal connection",
    "description": "Normal HTTP preflight used before opening the Pod terminal WebSocket. It verifies the interactive session, platform-administrator role, target cluster, and `runtime_terminal` Step-up assertion. A missing assertion returns `mfa_required`, allowing the frontend to show the MFA dialog and retry. A 204 authorizes only the preflight; the WebSocket repeats all checks before upgrading and revalidates session, role, assertion, Pod identity, and platform ownership every three seconds while connected. Revocation or expiry closes the shell.",
    "inputSchema": {
      "type": "object",
      "properties": {
        "clusterId": {
          "type": "string"
        },
        "namespace": {
          "type": "string"
        },
        "name": {
          "type": "string"
        }
      },
      "required": [
        "clusterId",
        "name",
        "namespace"
      ],
      "additionalProperties": false
    }
  },
  {
    "method": "get",
    "path": "/api/v1/runtime/clusters",
    "tags": [
      "Runtime"
    ],
    "deprecated": false,
    "security": [],
    "parameters": [
      {
        "name": "projectId",
        "in": "query",
        "schema": {
          "type": "string"
        }
      },
      {
        "name": "page",
        "in": "query",
        "ref": "#/components/parameters/Page",
        "schema": {
          "type": "integer",
          "default": 1,
          "minimum": 1
        }
      },
      {
        "name": "pageSize",
        "in": "query",
        "ref": "#/components/parameters/PageSize",
        "schema": {
          "type": "integer",
          "default": 20,
          "minimum": 1,
          "maximum": 100
        }
      },
      {
        "name": "sortBy",
        "in": "query",
        "schema": {
          "type": "string",
          "enum": [
            "name",
            "type",
            "scope",
            "status",
            "createdAt"
          ],
          "default": "createdAt"
        }
      },
      {
        "name": "sortOrder",
        "in": "query",
        "ref": "#/components/parameters/SortOrder",
        "schema": {
          "type": "string",
          "enum": [
            "asc",
            "desc"
          ],
          "default": "desc"
        }
      }
    ],
    "responses": [
      {
        "status": "200",
        "contentTypes": [],
        "schemaRefs": [],
        "description": "Runtime cluster list or paginated runtime cluster list."
      }
    ],
    "summary": "List runtime clusters",
    "description": "Returns the legacy array response when pagination parameters are omitted, or a paginated response when `page`/`pageSize` is supplied.",
    "inputSchema": {
      "type": "object",
      "properties": {
        "projectId": {
          "type": "string"
        },
        "page": {
          "type": "integer",
          "default": 1,
          "minimum": 1
        },
        "pageSize": {
          "type": "integer",
          "default": 20,
          "minimum": 1,
          "maximum": 100
        },
        "sortBy": {
          "type": "string",
          "enum": [
            "name",
            "type",
            "scope",
            "status",
            "createdAt"
          ],
          "default": "createdAt"
        },
        "sortOrder": {
          "type": "string",
          "enum": [
            "asc",
            "desc"
          ],
          "default": "desc"
        }
      },
      "additionalProperties": false
    }
  },
  {
    "method": "get",
    "path": "/api/v1/git/providers",
    "tags": [
      "Git"
    ],
    "deprecated": false,
    "security": [],
    "parameters": [
      {
        "name": "projectId",
        "in": "query",
        "schema": {
          "type": "string"
        }
      }
    ],
    "responses": [
      {
        "status": "200",
        "contentTypes": [],
        "schemaRefs": [],
        "description": "Git provider list."
      }
    ],
    "summary": "List Git providers",
    "inputSchema": {
      "type": "object",
      "properties": {
        "projectId": {
          "type": "string"
        }
      },
      "additionalProperties": false
    }
  },
  {
    "method": "post",
    "path": "/api/v1/git/providers",
    "tags": [
      "Git"
    ],
    "deprecated": false,
    "security": [],
    "parameters": [],
    "responses": [
      {
        "status": "201",
        "contentTypes": [],
        "schemaRefs": [],
        "description": "Created Git provider."
      }
    ],
    "summary": "Create Git provider",
    "requestBody": {
      "required": true,
      "contentTypes": [
        "application/json"
      ],
      "schemaRefs": [
        "#/components/schemas/GitProviderInput"
      ]
    },
    "inputSchema": {
      "type": "object",
      "properties": {
        "body": {
          "ref": "#/components/schemas/GitProviderInput",
          "type": "object",
          "required": [
            "name"
          ],
          "properties": {
            "authType": {
              "type": "string",
              "enum": [
                "oauth",
                "github-app",
                "pat"
              ]
            },
            "baseUrl": {
              "type": "string"
            },
            "clientId": {
              "type": "string"
            },
            "clientSecret": {
              "type": "string",
              "writeOnly": true
            },
            "enabled": {
              "type": "boolean"
            },
            "name": {
              "type": "string"
            },
            "ownerRef": {
              "type": "string"
            },
            "scope": {
              "type": "string",
              "enum": [
                "global",
                "project",
                "user"
              ]
            },
            "type": {
              "type": "string",
              "enum": [
                "github",
                "gitea",
                "gitlab"
              ]
            }
          }
        }
      },
      "required": [
        "body"
      ],
      "additionalProperties": false
    }
  },
  {
    "method": "put",
    "path": "/api/v1/git/providers/{providerId}",
    "tags": [
      "Git"
    ],
    "deprecated": false,
    "security": [],
    "parameters": [
      {
        "name": "providerId",
        "in": "path",
        "required": true,
        "ref": "#/components/parameters/ProviderId",
        "schema": {
          "type": "string"
        }
      }
    ],
    "responses": [
      {
        "status": "200",
        "contentTypes": [],
        "schemaRefs": [],
        "description": "Updated Git provider."
      }
    ],
    "summary": "Update Git provider",
    "requestBody": {
      "required": true,
      "contentTypes": [
        "application/json"
      ],
      "schemaRefs": [
        "#/components/schemas/GitProviderInput"
      ]
    },
    "inputSchema": {
      "type": "object",
      "properties": {
        "providerId": {
          "type": "string"
        },
        "body": {
          "ref": "#/components/schemas/GitProviderInput",
          "type": "object",
          "required": [
            "name"
          ],
          "properties": {
            "authType": {
              "type": "string",
              "enum": [
                "oauth",
                "github-app",
                "pat"
              ]
            },
            "baseUrl": {
              "type": "string"
            },
            "clientId": {
              "type": "string"
            },
            "clientSecret": {
              "type": "string",
              "writeOnly": true
            },
            "enabled": {
              "type": "boolean"
            },
            "name": {
              "type": "string"
            },
            "ownerRef": {
              "type": "string"
            },
            "scope": {
              "type": "string",
              "enum": [
                "global",
                "project",
                "user"
              ]
            },
            "type": {
              "type": "string",
              "enum": [
                "github",
                "gitea",
                "gitlab"
              ]
            }
          }
        }
      },
      "required": [
        "body",
        "providerId"
      ],
      "additionalProperties": false
    }
  },
  {
    "method": "delete",
    "path": "/api/v1/git/providers/{providerId}",
    "tags": [
      "Git"
    ],
    "deprecated": false,
    "security": [],
    "parameters": [
      {
        "name": "providerId",
        "in": "path",
        "required": true,
        "ref": "#/components/parameters/ProviderId",
        "schema": {
          "type": "string"
        }
      }
    ],
    "responses": [
      {
        "status": "204",
        "contentTypes": [],
        "schemaRefs": [],
        "description": "Deleted Git provider."
      }
    ],
    "summary": "Delete Git provider",
    "inputSchema": {
      "type": "object",
      "properties": {
        "providerId": {
          "type": "string"
        }
      },
      "required": [
        "providerId"
      ],
      "additionalProperties": false
    }
  },
  {
    "method": "get",
    "path": "/api/v1/git/providers/{providerId}/oauth/start",
    "tags": [
      "Git"
    ],
    "deprecated": false,
    "security": [],
    "parameters": [
      {
        "name": "providerId",
        "in": "path",
        "required": true,
        "ref": "#/components/parameters/ProviderId",
        "schema": {
          "type": "string"
        }
      },
      {
        "name": "redirect",
        "in": "query",
        "schema": {
          "type": "string",
          "default": "/projects"
        }
      },
      {
        "name": "frontendOrigin",
        "in": "query",
        "schema": {
          "type": "string",
          "default": ""
        }
      }
    ],
    "responses": [
      {
        "status": "302",
        "contentTypes": [],
        "schemaRefs": [],
        "description": "Redirect to Git OAuth provider."
      }
    ],
    "summary": "Start GitHub or Gitea OAuth flow",
    "inputSchema": {
      "type": "object",
      "properties": {
        "providerId": {
          "type": "string"
        },
        "redirect": {
          "type": "string",
          "default": "/projects"
        },
        "frontendOrigin": {
          "type": "string",
          "default": ""
        }
      },
      "required": [
        "providerId"
      ],
      "additionalProperties": false
    }
  },
  {
    "method": "get",
    "path": "/api/v1/git/oauth/callback",
    "tags": [
      "Git"
    ],
    "deprecated": false,
    "security": [],
    "parameters": [
      {
        "name": "state",
        "in": "query",
        "required": true,
        "ref": "#/components/parameters/OAuthState",
        "schema": {
          "type": "string"
        }
      },
      {
        "name": "code",
        "in": "query",
        "required": true,
        "ref": "#/components/parameters/OAuthCode",
        "schema": {
          "type": "string"
        }
      }
    ],
    "responses": [
      {
        "status": "302",
        "contentTypes": [],
        "schemaRefs": [],
        "description": "Redirect after Git OAuth callback."
      }
    ],
    "summary": "Complete Git OAuth callback",
    "inputSchema": {
      "type": "object",
      "properties": {
        "state": {
          "type": "string"
        },
        "code": {
          "type": "string"
        }
      },
      "required": [
        "code",
        "state"
      ],
      "additionalProperties": false
    }
  },
  {
    "method": "post",
    "path": "/api/v1/git/webhooks/{bindingId}",
    "tags": [
      "Git"
    ],
    "deprecated": false,
    "security": [],
    "parameters": [
      {
        "name": "bindingId",
        "in": "path",
        "required": true,
        "ref": "#/components/parameters/BindingId",
        "schema": {
          "type": "string"
        }
      }
    ],
    "responses": [
      {
        "status": "200",
        "contentTypes": [],
        "schemaRefs": [],
        "description": "Webhook accepted."
      },
      {
        "status": "401",
        "contentTypes": [],
        "schemaRefs": [],
        "description": "Invalid webhook signature."
      }
    ],
    "summary": "Receive Git webhook event",
    "inputSchema": {
      "type": "object",
      "properties": {
        "bindingId": {
          "type": "string"
        }
      },
      "required": [
        "bindingId"
      ],
      "additionalProperties": false
    }
  },
  {
    "method": "get",
    "path": "/api/v1/git/accounts",
    "tags": [
      "Git"
    ],
    "deprecated": false,
    "security": [],
    "parameters": [
      {
        "name": "projectId",
        "in": "query",
        "schema": {
          "type": "string"
        }
      }
    ],
    "responses": [
      {
        "status": "200",
        "contentTypes": [],
        "schemaRefs": [],
        "description": "Git account list."
      }
    ],
    "summary": "List current user Git accounts",
    "inputSchema": {
      "type": "object",
      "properties": {
        "projectId": {
          "type": "string"
        }
      },
      "additionalProperties": false
    }
  },
  {
    "method": "post",
    "path": "/api/v1/git/accounts",
    "tags": [
      "Git"
    ],
    "deprecated": false,
    "security": [],
    "parameters": [],
    "responses": [
      {
        "status": "201",
        "contentTypes": [],
        "schemaRefs": [],
        "description": "Created Git account."
      }
    ],
    "summary": "Create current user Git account manually",
    "requestBody": {
      "required": true,
      "contentTypes": [
        "application/json"
      ],
      "schemaRefs": [
        "#/components/schemas/GitAccountInput"
      ]
    },
    "inputSchema": {
      "type": "object",
      "properties": {
        "body": {
          "ref": "#/components/schemas/GitAccountInput",
          "type": "object",
          "required": [
            "providerId",
            "username"
          ],
          "properties": {
            "accessToken": {
              "type": "string",
              "writeOnly": true
            },
            "avatarUrl": {
              "type": "string"
            },
            "externalUserId": {
              "type": "string"
            },
            "ownerRef": {
              "type": "string"
            },
            "providerId": {
              "type": "string"
            },
            "refreshToken": {
              "type": "string",
              "writeOnly": true
            },
            "scope": {
              "type": "string",
              "enum": [
                "global",
                "project",
                "user"
              ]
            },
            "scopes": {
              "type": "array",
              "items": {
                "type": "string"
              }
            },
            "status": {
              "type": "string",
              "enum": [
                "connected",
                "expired",
                "revoked"
              ]
            },
            "username": {
              "type": "string"
            }
          }
        }
      },
      "required": [
        "body"
      ],
      "additionalProperties": false
    }
  },
  {
    "method": "put",
    "path": "/api/v1/git/accounts/{accountId}",
    "tags": [
      "Git"
    ],
    "deprecated": false,
    "security": [],
    "parameters": [
      {
        "name": "accountId",
        "in": "path",
        "required": true,
        "ref": "#/components/parameters/AccountId",
        "schema": {
          "type": "string"
        }
      }
    ],
    "responses": [
      {
        "status": "200",
        "contentTypes": [],
        "schemaRefs": [],
        "description": "Updated Git account."
      }
    ],
    "summary": "Update current user Git account",
    "requestBody": {
      "required": true,
      "contentTypes": [
        "application/json"
      ],
      "schemaRefs": [
        "#/components/schemas/GitAccountInput"
      ]
    },
    "inputSchema": {
      "type": "object",
      "properties": {
        "accountId": {
          "type": "string"
        },
        "body": {
          "ref": "#/components/schemas/GitAccountInput",
          "type": "object",
          "required": [
            "providerId",
            "username"
          ],
          "properties": {
            "accessToken": {
              "type": "string",
              "writeOnly": true
            },
            "avatarUrl": {
              "type": "string"
            },
            "externalUserId": {
              "type": "string"
            },
            "ownerRef": {
              "type": "string"
            },
            "providerId": {
              "type": "string"
            },
            "refreshToken": {
              "type": "string",
              "writeOnly": true
            },
            "scope": {
              "type": "string",
              "enum": [
                "global",
                "project",
                "user"
              ]
            },
            "scopes": {
              "type": "array",
              "items": {
                "type": "string"
              }
            },
            "status": {
              "type": "string",
              "enum": [
                "connected",
                "expired",
                "revoked"
              ]
            },
            "username": {
              "type": "string"
            }
          }
        }
      },
      "required": [
        "accountId",
        "body"
      ],
      "additionalProperties": false
    }
  },
  {
    "method": "delete",
    "path": "/api/v1/git/accounts/{accountId}",
    "tags": [
      "Git"
    ],
    "deprecated": false,
    "security": [],
    "parameters": [
      {
        "name": "accountId",
        "in": "path",
        "required": true,
        "ref": "#/components/parameters/AccountId",
        "schema": {
          "type": "string"
        }
      }
    ],
    "responses": [
      {
        "status": "204",
        "contentTypes": [],
        "schemaRefs": [],
        "description": "Deleted Git account."
      }
    ],
    "summary": "Delete current user Git account",
    "inputSchema": {
      "type": "object",
      "properties": {
        "accountId": {
          "type": "string"
        }
      },
      "required": [
        "accountId"
      ],
      "additionalProperties": false
    }
  },
  {
    "method": "post",
    "path": "/api/v1/git/accounts/{accountId}/refresh",
    "tags": [
      "Git"
    ],
    "deprecated": false,
    "security": [],
    "parameters": [
      {
        "name": "accountId",
        "in": "path",
        "required": true,
        "ref": "#/components/parameters/AccountId",
        "schema": {
          "type": "string"
        }
      }
    ],
    "responses": [
      {
        "status": "200",
        "contentTypes": [],
        "schemaRefs": [],
        "description": "Refreshed Git account."
      }
    ],
    "summary": "Refresh current user Git account token",
    "inputSchema": {
      "type": "object",
      "properties": {
        "accountId": {
          "type": "string"
        }
      },
      "required": [
        "accountId"
      ],
      "additionalProperties": false
    }
  },
  {
    "method": "get",
    "path": "/api/v1/git/accounts/{accountId}/repositories",
    "tags": [
      "Git"
    ],
    "deprecated": false,
    "security": [],
    "parameters": [
      {
        "name": "accountId",
        "in": "path",
        "required": true,
        "ref": "#/components/parameters/AccountId",
        "schema": {
          "type": "string"
        }
      },
      {
        "name": "page",
        "in": "query",
        "ref": "#/components/parameters/Page",
        "schema": {
          "type": "integer",
          "default": 1,
          "minimum": 1
        }
      },
      {
        "name": "pageSize",
        "in": "query",
        "ref": "#/components/parameters/PageSize",
        "schema": {
          "type": "integer",
          "default": 20,
          "minimum": 1,
          "maximum": 100
        }
      },
      {
        "name": "search",
        "in": "query",
        "schema": {
          "type": "string"
        }
      }
    ],
    "responses": [
      {
        "status": "200",
        "contentTypes": [],
        "schemaRefs": [],
        "description": "Repository list."
      }
    ],
    "summary": "List repositories visible to a Git account",
    "inputSchema": {
      "type": "object",
      "properties": {
        "accountId": {
          "type": "string"
        },
        "page": {
          "type": "integer",
          "default": 1,
          "minimum": 1
        },
        "pageSize": {
          "type": "integer",
          "default": 20,
          "minimum": 1,
          "maximum": 100
        },
        "search": {
          "type": "string"
        }
      },
      "required": [
        "accountId"
      ],
      "additionalProperties": false
    }
  },
  {
    "method": "get",
    "path": "/api/v1/git/accounts/{accountId}/repositories/{owner}/{repo}/branches",
    "tags": [
      "Git"
    ],
    "deprecated": false,
    "security": [],
    "parameters": [
      {
        "name": "accountId",
        "in": "path",
        "required": true,
        "ref": "#/components/parameters/AccountId",
        "schema": {
          "type": "string"
        }
      },
      {
        "name": "owner",
        "in": "path",
        "required": true,
        "ref": "#/components/parameters/Owner",
        "schema": {
          "type": "string"
        }
      },
      {
        "name": "repo",
        "in": "path",
        "required": true,
        "ref": "#/components/parameters/Repo",
        "schema": {
          "type": "string"
        }
      }
    ],
    "responses": [
      {
        "status": "200",
        "contentTypes": [],
        "schemaRefs": [],
        "description": "Branch list."
      }
    ],
    "summary": "List repository branches",
    "inputSchema": {
      "type": "object",
      "properties": {
        "accountId": {
          "type": "string"
        },
        "owner": {
          "type": "string"
        },
        "repo": {
          "type": "string"
        }
      },
      "required": [
        "accountId",
        "owner",
        "repo"
      ],
      "additionalProperties": false
    }
  },
  {
    "method": "get",
    "path": "/api/v1/git/accounts/{accountId}/repositories/{owner}/{repo}/file",
    "tags": [
      "Git"
    ],
    "deprecated": false,
    "security": [],
    "parameters": [
      {
        "name": "accountId",
        "in": "path",
        "required": true,
        "ref": "#/components/parameters/AccountId",
        "schema": {
          "type": "string"
        }
      },
      {
        "name": "owner",
        "in": "path",
        "required": true,
        "ref": "#/components/parameters/Owner",
        "schema": {
          "type": "string"
        }
      },
      {
        "name": "repo",
        "in": "path",
        "required": true,
        "ref": "#/components/parameters/Repo",
        "schema": {
          "type": "string"
        }
      },
      {
        "name": "path",
        "in": "query",
        "required": true,
        "schema": {
          "type": "string"
        }
      },
      {
        "name": "ref",
        "in": "query",
        "schema": {
          "type": "string"
        }
      }
    ],
    "responses": [
      {
        "status": "200",
        "contentTypes": [],
        "schemaRefs": [],
        "description": "File content."
      }
    ],
    "summary": "Read repository file content",
    "inputSchema": {
      "type": "object",
      "properties": {
        "accountId": {
          "type": "string"
        },
        "owner": {
          "type": "string"
        },
        "repo": {
          "type": "string"
        },
        "path": {
          "type": "string"
        },
        "ref": {
          "type": "string"
        }
      },
      "required": [
        "accountId",
        "owner",
        "path",
        "repo"
      ],
      "additionalProperties": false
    }
  },
  {
    "method": "get",
    "path": "/api/v1/registries",
    "tags": [
      "Registries"
    ],
    "deprecated": false,
    "security": [],
    "parameters": [
      {
        "name": "projectId",
        "in": "query",
        "schema": {
          "type": "string"
        }
      },
      {
        "name": "page",
        "in": "query",
        "ref": "#/components/parameters/Page",
        "schema": {
          "type": "integer",
          "default": 1,
          "minimum": 1
        }
      },
      {
        "name": "pageSize",
        "in": "query",
        "ref": "#/components/parameters/PageSize",
        "schema": {
          "type": "integer",
          "default": 20,
          "minimum": 1,
          "maximum": 100
        }
      },
      {
        "name": "sortBy",
        "in": "query",
        "schema": {
          "type": "string",
          "enum": [
            "name",
            "scope",
            "createdAt"
          ],
          "default": "createdAt"
        }
      },
      {
        "name": "sortOrder",
        "in": "query",
        "ref": "#/components/parameters/SortOrder",
        "schema": {
          "type": "string",
          "enum": [
            "asc",
            "desc"
          ],
          "default": "desc"
        }
      }
    ],
    "responses": [
      {
        "status": "200",
        "contentTypes": [],
        "schemaRefs": [],
        "description": "Artifact registry list."
      }
    ],
    "summary": "List artifact registries",
    "inputSchema": {
      "type": "object",
      "properties": {
        "projectId": {
          "type": "string"
        },
        "page": {
          "type": "integer",
          "default": 1,
          "minimum": 1
        },
        "pageSize": {
          "type": "integer",
          "default": 20,
          "minimum": 1,
          "maximum": 100
        },
        "sortBy": {
          "type": "string",
          "enum": [
            "name",
            "scope",
            "createdAt"
          ],
          "default": "createdAt"
        },
        "sortOrder": {
          "type": "string",
          "enum": [
            "asc",
            "desc"
          ],
          "default": "desc"
        }
      },
      "additionalProperties": false
    }
  },
  {
    "method": "post",
    "path": "/api/v1/registries",
    "tags": [
      "Registries"
    ],
    "deprecated": false,
    "security": [],
    "parameters": [],
    "responses": [
      {
        "status": "201",
        "contentTypes": [],
        "schemaRefs": [],
        "description": "Created artifact registry."
      }
    ],
    "summary": "Create artifact registry",
    "requestBody": {
      "required": true,
      "contentTypes": [
        "application/json"
      ],
      "schemaRefs": [
        "#/components/schemas/ArtifactRegistryInput"
      ]
    },
    "inputSchema": {
      "type": "object",
      "properties": {
        "body": {
          "ref": "#/components/schemas/ArtifactRegistryInput",
          "type": "object",
          "required": [
            "endpoint",
            "name",
            "provider",
            "scope"
          ],
          "properties": {
            "capabilities": {
              "type": "array",
              "items": {
                "type": "string"
              }
            },
            "endpoint": {
              "type": "string"
            },
            "isDefault": {
              "type": "boolean"
            },
            "name": {
              "type": "string"
            },
            "namespace": {
              "type": "string"
            },
            "ownerRef": {
              "type": "string"
            },
            "provider": {
              "type": "string",
              "enum": [
                "harbor",
                "dockerhub",
                "gitea-registry"
              ]
            },
            "scope": {
              "type": "string",
              "enum": [
                "global",
                "project",
                "user"
              ]
            }
          }
        }
      },
      "required": [
        "body"
      ],
      "additionalProperties": false
    }
  },
  {
    "method": "put",
    "path": "/api/v1/registries/{registryId}",
    "tags": [
      "Registries"
    ],
    "deprecated": false,
    "security": [],
    "parameters": [
      {
        "name": "registryId",
        "in": "path",
        "required": true,
        "ref": "#/components/parameters/RegistryId",
        "schema": {
          "type": "string"
        }
      }
    ],
    "responses": [
      {
        "status": "200",
        "contentTypes": [],
        "schemaRefs": [],
        "description": "Updated artifact registry."
      }
    ],
    "summary": "Update artifact registry",
    "requestBody": {
      "required": true,
      "contentTypes": [
        "application/json"
      ],
      "schemaRefs": [
        "#/components/schemas/ArtifactRegistryInput"
      ]
    },
    "inputSchema": {
      "type": "object",
      "properties": {
        "registryId": {
          "type": "string"
        },
        "body": {
          "ref": "#/components/schemas/ArtifactRegistryInput",
          "type": "object",
          "required": [
            "endpoint",
            "name",
            "provider",
            "scope"
          ],
          "properties": {
            "capabilities": {
              "type": "array",
              "items": {
                "type": "string"
              }
            },
            "endpoint": {
              "type": "string"
            },
            "isDefault": {
              "type": "boolean"
            },
            "name": {
              "type": "string"
            },
            "namespace": {
              "type": "string"
            },
            "ownerRef": {
              "type": "string"
            },
            "provider": {
              "type": "string",
              "enum": [
                "harbor",
                "dockerhub",
                "gitea-registry"
              ]
            },
            "scope": {
              "type": "string",
              "enum": [
                "global",
                "project",
                "user"
              ]
            }
          }
        }
      },
      "required": [
        "body",
        "registryId"
      ],
      "additionalProperties": false
    }
  },
  {
    "method": "delete",
    "path": "/api/v1/registries/{registryId}",
    "tags": [
      "Registries"
    ],
    "deprecated": false,
    "security": [],
    "parameters": [
      {
        "name": "registryId",
        "in": "path",
        "required": true,
        "ref": "#/components/parameters/RegistryId",
        "schema": {
          "type": "string"
        }
      }
    ],
    "responses": [
      {
        "status": "204",
        "contentTypes": [],
        "schemaRefs": [],
        "description": "Deleted artifact registry."
      }
    ],
    "summary": "Delete artifact registry",
    "inputSchema": {
      "type": "object",
      "properties": {
        "registryId": {
          "type": "string"
        }
      },
      "required": [
        "registryId"
      ],
      "additionalProperties": false
    }
  },
  {
    "method": "post",
    "path": "/api/v1/registries/{registryId}/test",
    "tags": [
      "Registries"
    ],
    "deprecated": false,
    "security": [],
    "parameters": [
      {
        "name": "registryId",
        "in": "path",
        "required": true,
        "ref": "#/components/parameters/RegistryId",
        "schema": {
          "type": "string"
        }
      }
    ],
    "responses": [
      {
        "status": "200",
        "contentTypes": [],
        "schemaRefs": [],
        "description": "Registry test result."
      }
    ],
    "summary": "Test artifact registry connectivity",
    "inputSchema": {
      "type": "object",
      "properties": {
        "registryId": {
          "type": "string"
        }
      },
      "required": [
        "registryId"
      ],
      "additionalProperties": false
    }
  },
  {
    "method": "get",
    "path": "/api/v1/registries/{registryId}/credentials",
    "tags": [
      "Registries"
    ],
    "deprecated": false,
    "security": [],
    "parameters": [
      {
        "name": "registryId",
        "in": "path",
        "required": true,
        "ref": "#/components/parameters/RegistryId",
        "schema": {
          "type": "string"
        }
      },
      {
        "name": "page",
        "in": "query",
        "ref": "#/components/parameters/Page",
        "schema": {
          "type": "integer",
          "default": 1,
          "minimum": 1
        }
      },
      {
        "name": "pageSize",
        "in": "query",
        "ref": "#/components/parameters/PageSize",
        "schema": {
          "type": "integer",
          "default": 20,
          "minimum": 1,
          "maximum": 100
        }
      },
      {
        "name": "sortBy",
        "in": "query",
        "schema": {
          "type": "string",
          "enum": [
            "name",
            "username",
            "createdAt"
          ],
          "default": "createdAt"
        }
      },
      {
        "name": "sortOrder",
        "in": "query",
        "ref": "#/components/parameters/SortOrder",
        "schema": {
          "type": "string",
          "enum": [
            "asc",
            "desc"
          ],
          "default": "desc"
        }
      }
    ],
    "responses": [
      {
        "status": "200",
        "contentTypes": [],
        "schemaRefs": [],
        "description": "Registry credential list."
      }
    ],
    "summary": "List registry credentials",
    "inputSchema": {
      "type": "object",
      "properties": {
        "registryId": {
          "type": "string"
        },
        "page": {
          "type": "integer",
          "default": 1,
          "minimum": 1
        },
        "pageSize": {
          "type": "integer",
          "default": 20,
          "minimum": 1,
          "maximum": 100
        },
        "sortBy": {
          "type": "string",
          "enum": [
            "name",
            "username",
            "createdAt"
          ],
          "default": "createdAt"
        },
        "sortOrder": {
          "type": "string",
          "enum": [
            "asc",
            "desc"
          ],
          "default": "desc"
        }
      },
      "required": [
        "registryId"
      ],
      "additionalProperties": false
    }
  },
  {
    "method": "post",
    "path": "/api/v1/registries/{registryId}/credentials",
    "tags": [
      "Registries"
    ],
    "deprecated": false,
    "security": [],
    "parameters": [
      {
        "name": "registryId",
        "in": "path",
        "required": true,
        "ref": "#/components/parameters/RegistryId",
        "schema": {
          "type": "string"
        }
      }
    ],
    "responses": [
      {
        "status": "201",
        "contentTypes": [],
        "schemaRefs": [],
        "description": "Created registry credential."
      }
    ],
    "summary": "Create registry credential",
    "requestBody": {
      "required": true,
      "contentTypes": [
        "application/json"
      ],
      "schemaRefs": [
        "#/components/schemas/RegistryCredentialInput"
      ]
    },
    "inputSchema": {
      "type": "object",
      "properties": {
        "registryId": {
          "type": "string"
        },
        "body": {
          "ref": "#/components/schemas/RegistryCredentialInput",
          "type": "object",
          "properties": {
            "name": {
              "type": "string"
            },
            "password": {
              "type": "string"
            },
            "projectIds": {
              "type": "array",
              "items": {
                "type": "string"
              }
            },
            "repositoryTemplate": {
              "type": "string"
            },
            "scope": {
              "type": "string",
              "enum": [
                "user",
                "project",
                "global"
              ]
            },
            "tagTemplate": {
              "type": "string"
            },
            "token": {
              "type": "string"
            },
            "usage": {
              "type": "string",
              "enum": [
                "pull",
                "push",
                "push-pull"
              ]
            },
            "username": {
              "type": "string"
            }
          }
        }
      },
      "required": [
        "body",
        "registryId"
      ],
      "additionalProperties": false
    }
  },
  {
    "method": "get",
    "path": "/api/v1/registry-credentials",
    "tags": [
      "Registries"
    ],
    "deprecated": false,
    "security": [],
    "parameters": [
      {
        "name": "page",
        "in": "query",
        "ref": "#/components/parameters/Page",
        "schema": {
          "type": "integer",
          "default": 1,
          "minimum": 1
        }
      },
      {
        "name": "pageSize",
        "in": "query",
        "ref": "#/components/parameters/PageSize",
        "schema": {
          "type": "integer",
          "default": 20,
          "minimum": 1,
          "maximum": 100
        }
      },
      {
        "name": "sortBy",
        "in": "query",
        "schema": {
          "type": "string",
          "enum": [
            "name",
            "username",
            "createdAt"
          ],
          "default": "createdAt"
        }
      },
      {
        "name": "sortOrder",
        "in": "query",
        "ref": "#/components/parameters/SortOrder",
        "schema": {
          "type": "string",
          "enum": [
            "asc",
            "desc"
          ],
          "default": "desc"
        }
      }
    ],
    "responses": [
      {
        "status": "200",
        "contentTypes": [],
        "schemaRefs": [],
        "description": "Paginated registry credential list."
      }
    ],
    "summary": "List visible registry credentials across registries",
    "inputSchema": {
      "type": "object",
      "properties": {
        "page": {
          "type": "integer",
          "default": 1,
          "minimum": 1
        },
        "pageSize": {
          "type": "integer",
          "default": 20,
          "minimum": 1,
          "maximum": 100
        },
        "sortBy": {
          "type": "string",
          "enum": [
            "name",
            "username",
            "createdAt"
          ],
          "default": "createdAt"
        },
        "sortOrder": {
          "type": "string",
          "enum": [
            "asc",
            "desc"
          ],
          "default": "desc"
        }
      },
      "additionalProperties": false
    }
  },
  {
    "method": "put",
    "path": "/api/v1/registries/{registryId}/credentials/{credentialId}",
    "tags": [
      "Registries"
    ],
    "deprecated": false,
    "security": [],
    "parameters": [
      {
        "name": "registryId",
        "in": "path",
        "required": true,
        "ref": "#/components/parameters/RegistryId",
        "schema": {
          "type": "string"
        }
      },
      {
        "name": "credentialId",
        "in": "path",
        "required": true,
        "ref": "#/components/parameters/CredentialId",
        "schema": {
          "type": "string"
        }
      }
    ],
    "responses": [
      {
        "status": "200",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/RegistryCredential"
        ],
        "description": "Updated registry credential."
      }
    ],
    "summary": "Update registry credential",
    "requestBody": {
      "required": true,
      "contentTypes": [
        "application/json"
      ],
      "schemaRefs": [
        "#/components/schemas/RegistryCredentialInput"
      ]
    },
    "inputSchema": {
      "type": "object",
      "properties": {
        "registryId": {
          "type": "string"
        },
        "credentialId": {
          "type": "string"
        },
        "body": {
          "ref": "#/components/schemas/RegistryCredentialInput",
          "type": "object",
          "properties": {
            "name": {
              "type": "string"
            },
            "password": {
              "type": "string"
            },
            "projectIds": {
              "type": "array",
              "items": {
                "type": "string"
              }
            },
            "repositoryTemplate": {
              "type": "string"
            },
            "scope": {
              "type": "string",
              "enum": [
                "user",
                "project",
                "global"
              ]
            },
            "tagTemplate": {
              "type": "string"
            },
            "token": {
              "type": "string"
            },
            "usage": {
              "type": "string",
              "enum": [
                "pull",
                "push",
                "push-pull"
              ]
            },
            "username": {
              "type": "string"
            }
          }
        }
      },
      "required": [
        "body",
        "credentialId",
        "registryId"
      ],
      "additionalProperties": false
    }
  },
  {
    "method": "delete",
    "path": "/api/v1/registries/{registryId}/credentials/{credentialId}",
    "tags": [
      "Registries"
    ],
    "deprecated": false,
    "security": [],
    "parameters": [
      {
        "name": "registryId",
        "in": "path",
        "required": true,
        "ref": "#/components/parameters/RegistryId",
        "schema": {
          "type": "string"
        }
      },
      {
        "name": "credentialId",
        "in": "path",
        "required": true,
        "ref": "#/components/parameters/CredentialId",
        "schema": {
          "type": "string"
        }
      }
    ],
    "responses": [
      {
        "status": "204",
        "contentTypes": [],
        "schemaRefs": [],
        "description": "Deleted registry credential."
      }
    ],
    "summary": "Delete registry credential",
    "inputSchema": {
      "type": "object",
      "properties": {
        "registryId": {
          "type": "string"
        },
        "credentialId": {
          "type": "string"
        }
      },
      "required": [
        "credentialId",
        "registryId"
      ],
      "additionalProperties": false
    }
  },
  {
    "method": "get",
    "path": "/api/v1/container-images",
    "tags": [
      "Registries"
    ],
    "deprecated": false,
    "security": [],
    "parameters": [
      {
        "name": "projectId",
        "in": "query",
        "schema": {
          "type": "string"
        }
      },
      {
        "name": "applicationId",
        "in": "query",
        "schema": {
          "type": "string"
        }
      },
      {
        "name": "registryId",
        "in": "query",
        "schema": {
          "type": "string"
        }
      }
    ],
    "responses": [
      {
        "status": "200",
        "contentTypes": [],
        "schemaRefs": [],
        "description": "Container image list."
      }
    ],
    "summary": "List container image records",
    "inputSchema": {
      "type": "object",
      "properties": {
        "projectId": {
          "type": "string"
        },
        "applicationId": {
          "type": "string"
        },
        "registryId": {
          "type": "string"
        }
      },
      "additionalProperties": false
    }
  },
  {
    "method": "post",
    "path": "/api/v1/container-images",
    "tags": [
      "Registries"
    ],
    "deprecated": false,
    "security": [],
    "parameters": [],
    "responses": [
      {
        "status": "201",
        "contentTypes": [],
        "schemaRefs": [],
        "description": "Created container image record."
      }
    ],
    "summary": "Create container image record",
    "requestBody": {
      "required": true,
      "contentTypes": [
        "application/json"
      ],
      "schemaRefs": [
        "#/components/schemas/ContainerImageInput"
      ]
    },
    "inputSchema": {
      "type": "object",
      "properties": {
        "body": {
          "ref": "#/components/schemas/ContainerImageInput",
          "type": "object",
          "required": [
            "registryId",
            "repository",
            "tag"
          ],
          "properties": {
            "applicationId": {
              "type": "string"
            },
            "buildRunId": {
              "type": "string"
            },
            "digest": {
              "type": "string"
            },
            "imageRef": {
              "type": "string"
            },
            "projectId": {
              "type": "string"
            },
            "registryId": {
              "type": "string"
            },
            "repository": {
              "type": "string"
            },
            "scanStatus": {
              "type": "string"
            },
            "sourceCommit": {
              "type": "string"
            },
            "sourceType": {
              "type": "string"
            },
            "tag": {
              "type": "string"
            }
          }
        }
      },
      "required": [
        "body"
      ],
      "additionalProperties": false
    }
  },
  {
    "method": "get",
    "path": "/api/v1/dashboard",
    "tags": [
      "Dashboard"
    ],
    "deprecated": false,
    "security": [],
    "parameters": [],
    "responses": [
      {
        "status": "200",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/DashboardOverview"
        ],
        "description": "Dashboard overview scoped to the current user's visible project spaces and resources."
      },
      {
        "status": "401",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/ErrorResponse"
        ],
        "description": "Authentication is required."
      },
      {
        "status": "500",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/ErrorResponse"
        ],
        "description": "Dashboard aggregation failed (`dashboard.load_failed`)."
      }
    ],
    "summary": "Get the current user's dashboard overview",
    "description": "Returns the task-oriented dashboard aggregation in one response. Future dashboard read models are added to this contract instead of being composed from multiple browser requests.",
    "operationId": "getDashboard",
    "xLunaCli": {
      "command": "dashboard.show",
      "classification": "business-command",
      "risk": "low",
      "requiredScopes": [
        "dashboard:read"
      ]
    }
  },
  {
    "method": "get",
    "path": "/api/v1/projects",
    "tags": [
      "Projects"
    ],
    "deprecated": false,
    "security": [],
    "parameters": [
      {
        "name": "page",
        "in": "query",
        "ref": "#/components/parameters/Page",
        "schema": {
          "type": "integer",
          "default": 1,
          "minimum": 1
        }
      },
      {
        "name": "pageSize",
        "in": "query",
        "ref": "#/components/parameters/PageSize",
        "schema": {
          "type": "integer",
          "default": 20,
          "minimum": 1,
          "maximum": 100
        }
      },
      {
        "name": "sortBy",
        "in": "query",
        "schema": {
          "type": "string",
          "enum": [
            "createdAt",
            "name",
            "identifier"
          ],
          "default": "createdAt"
        }
      },
      {
        "name": "sortOrder",
        "in": "query",
        "ref": "#/components/parameters/SortOrder",
        "schema": {
          "type": "string",
          "enum": [
            "asc",
            "desc"
          ],
          "default": "desc"
        }
      }
    ],
    "responses": [
      {
        "status": "200",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/Project",
          "#/components/schemas/PaginatedProjectList"
        ],
        "description": "Project list or paginated project list."
      }
    ],
    "summary": "List projects",
    "description": "Returns the legacy project array when pagination parameters are omitted. Returns a paginated object when page or pageSize is provided.",
    "inputSchema": {
      "type": "object",
      "properties": {
        "page": {
          "type": "integer",
          "default": 1,
          "minimum": 1
        },
        "pageSize": {
          "type": "integer",
          "default": 20,
          "minimum": 1,
          "maximum": 100
        },
        "sortBy": {
          "type": "string",
          "enum": [
            "createdAt",
            "name",
            "identifier"
          ],
          "default": "createdAt"
        },
        "sortOrder": {
          "type": "string",
          "enum": [
            "asc",
            "desc"
          ],
          "default": "desc"
        }
      },
      "additionalProperties": false
    }
  },
  {
    "method": "post",
    "path": "/api/v1/projects",
    "tags": [
      "Projects"
    ],
    "deprecated": false,
    "security": [],
    "parameters": [],
    "responses": [
      {
        "status": "201",
        "contentTypes": [],
        "schemaRefs": [],
        "description": "Created project."
      }
    ],
    "summary": "Create project",
    "requestBody": {
      "required": true,
      "contentTypes": [
        "application/json"
      ],
      "schemaRefs": [
        "#/components/schemas/ProjectInput"
      ]
    },
    "inputSchema": {
      "type": "object",
      "properties": {
        "body": {
          "ref": "#/components/schemas/ProjectInput",
          "type": "object",
          "required": [
            "identifier",
            "name"
          ],
          "properties": {
            "description": {
              "type": "string"
            },
            "identifier": {
              "type": "string",
              "description": "Immutable project-space identifier used to derive the project ID and Kubernetes Namespace.",
              "minLength": 2,
              "maxLength": 22,
              "pattern": "^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$"
            },
            "maxConcurrentBuilds": {
              "type": "integer",
              "minimum": 1
            },
            "name": {
              "type": "string"
            },
            "namespaceStrategy": {
              "type": "string",
              "enum": [
                "project"
              ]
            },
            "webConsoleEnabled": {
              "type": "boolean",
              "description": "Project-space master switch for release Web Console and runtime exec access. Omission on create defaults to true; omission on update preserves the current value. When false, no deployment target can re-enable Web Console. Project roles and Step-up MFA still apply.",
              "default": true
            }
          }
        }
      },
      "required": [
        "body"
      ],
      "additionalProperties": false
    }
  },
  {
    "method": "get",
    "path": "/api/v1/projects/pins",
    "tags": [
      "Projects"
    ],
    "deprecated": false,
    "security": [],
    "parameters": [],
    "responses": [
      {
        "status": "200",
        "contentTypes": [],
        "schemaRefs": [],
        "description": "Pinned project list."
      }
    ],
    "summary": "List current user's pinned projects"
  },
  {
    "method": "get",
    "path": "/api/v1/projects/{projectId}",
    "tags": [
      "Projects"
    ],
    "deprecated": false,
    "security": [],
    "parameters": [
      {
        "name": "projectId",
        "in": "path",
        "required": true,
        "ref": "#/components/parameters/ProjectId",
        "schema": {
          "type": "string"
        }
      }
    ],
    "responses": [
      {
        "status": "200",
        "contentTypes": [],
        "schemaRefs": [],
        "description": "Project."
      }
    ],
    "summary": "Get project",
    "inputSchema": {
      "type": "object",
      "properties": {
        "projectId": {
          "type": "string"
        }
      },
      "required": [
        "projectId"
      ],
      "additionalProperties": false
    }
  },
  {
    "method": "put",
    "path": "/api/v1/projects/{projectId}",
    "tags": [
      "Projects"
    ],
    "deprecated": false,
    "security": [],
    "parameters": [
      {
        "name": "projectId",
        "in": "path",
        "required": true,
        "ref": "#/components/parameters/ProjectId",
        "schema": {
          "type": "string"
        }
      }
    ],
    "responses": [
      {
        "status": "200",
        "contentTypes": [],
        "schemaRefs": [],
        "description": "Updated project."
      }
    ],
    "summary": "Update project",
    "requestBody": {
      "required": true,
      "contentTypes": [
        "application/json"
      ],
      "schemaRefs": [
        "#/components/schemas/ProjectInput"
      ]
    },
    "inputSchema": {
      "type": "object",
      "properties": {
        "projectId": {
          "type": "string"
        },
        "body": {
          "ref": "#/components/schemas/ProjectInput",
          "type": "object",
          "required": [
            "identifier",
            "name"
          ],
          "properties": {
            "description": {
              "type": "string"
            },
            "identifier": {
              "type": "string",
              "description": "Immutable project-space identifier used to derive the project ID and Kubernetes Namespace.",
              "minLength": 2,
              "maxLength": 22,
              "pattern": "^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$"
            },
            "maxConcurrentBuilds": {
              "type": "integer",
              "minimum": 1
            },
            "name": {
              "type": "string"
            },
            "namespaceStrategy": {
              "type": "string",
              "enum": [
                "project"
              ]
            },
            "webConsoleEnabled": {
              "type": "boolean",
              "description": "Project-space master switch for release Web Console and runtime exec access. Omission on create defaults to true; omission on update preserves the current value. When false, no deployment target can re-enable Web Console. Project roles and Step-up MFA still apply.",
              "default": true
            }
          }
        }
      },
      "required": [
        "body",
        "projectId"
      ],
      "additionalProperties": false
    }
  },
  {
    "method": "delete",
    "path": "/api/v1/projects/{projectId}",
    "tags": [
      "Projects"
    ],
    "deprecated": false,
    "security": [],
    "parameters": [
      {
        "name": "projectId",
        "in": "path",
        "required": true,
        "ref": "#/components/parameters/ProjectId",
        "schema": {
          "type": "string"
        }
      }
    ],
    "responses": [
      {
        "status": "204",
        "contentTypes": [],
        "schemaRefs": [],
        "description": "Deleted project."
      }
    ],
    "summary": "Delete project",
    "inputSchema": {
      "type": "object",
      "properties": {
        "projectId": {
          "type": "string"
        }
      },
      "required": [
        "projectId"
      ],
      "additionalProperties": false
    }
  },
  {
    "method": "put",
    "path": "/api/v1/projects/{projectId}/pin",
    "tags": [
      "Projects"
    ],
    "deprecated": false,
    "security": [],
    "parameters": [
      {
        "name": "projectId",
        "in": "path",
        "required": true,
        "ref": "#/components/parameters/ProjectId",
        "schema": {
          "type": "string"
        }
      }
    ],
    "responses": [
      {
        "status": "200",
        "contentTypes": [],
        "schemaRefs": [],
        "description": "Updated pinned project."
      },
      {
        "status": "201",
        "contentTypes": [],
        "schemaRefs": [],
        "description": "Created pinned project."
      }
    ],
    "summary": "Pin project for current user",
    "inputSchema": {
      "type": "object",
      "properties": {
        "projectId": {
          "type": "string"
        }
      },
      "required": [
        "projectId"
      ],
      "additionalProperties": false
    }
  },
  {
    "method": "delete",
    "path": "/api/v1/projects/{projectId}/pin",
    "tags": [
      "Projects"
    ],
    "deprecated": false,
    "security": [],
    "parameters": [
      {
        "name": "projectId",
        "in": "path",
        "required": true,
        "ref": "#/components/parameters/ProjectId",
        "schema": {
          "type": "string"
        }
      }
    ],
    "responses": [
      {
        "status": "204",
        "contentTypes": [],
        "schemaRefs": [],
        "description": "Unpinned project."
      }
    ],
    "summary": "Unpin project for current user",
    "inputSchema": {
      "type": "object",
      "properties": {
        "projectId": {
          "type": "string"
        }
      },
      "required": [
        "projectId"
      ],
      "additionalProperties": false
    }
  },
  {
    "method": "get",
    "path": "/api/v1/projects/{projectId}/registries/default",
    "tags": [
      "Registries"
    ],
    "deprecated": false,
    "security": [],
    "parameters": [
      {
        "name": "projectId",
        "in": "path",
        "required": true,
        "ref": "#/components/parameters/ProjectId",
        "schema": {
          "type": "string"
        }
      }
    ],
    "responses": [
      {
        "status": "200",
        "contentTypes": [],
        "schemaRefs": [],
        "description": "Default artifact registry."
      }
    ],
    "summary": "Get default artifact registry for a project",
    "inputSchema": {
      "type": "object",
      "properties": {
        "projectId": {
          "type": "string"
        }
      },
      "required": [
        "projectId"
      ],
      "additionalProperties": false
    }
  },
  {
    "method": "get",
    "path": "/api/v1/projects/{projectId}/members",
    "tags": [
      "Projects"
    ],
    "deprecated": false,
    "security": [],
    "parameters": [
      {
        "name": "projectId",
        "in": "path",
        "required": true,
        "ref": "#/components/parameters/ProjectId",
        "schema": {
          "type": "string"
        }
      }
    ],
    "responses": [
      {
        "status": "200",
        "contentTypes": [],
        "schemaRefs": [],
        "description": "Project member list."
      }
    ],
    "summary": "List project members",
    "inputSchema": {
      "type": "object",
      "properties": {
        "projectId": {
          "type": "string"
        }
      },
      "required": [
        "projectId"
      ],
      "additionalProperties": false
    }
  },
  {
    "method": "post",
    "path": "/api/v1/projects/{projectId}/members",
    "tags": [
      "Projects"
    ],
    "deprecated": false,
    "security": [],
    "parameters": [
      {
        "name": "projectId",
        "in": "path",
        "required": true,
        "ref": "#/components/parameters/ProjectId",
        "schema": {
          "type": "string"
        }
      }
    ],
    "responses": [
      {
        "status": "201",
        "contentTypes": [],
        "schemaRefs": [],
        "description": "Created project member."
      }
    ],
    "summary": "Create project member",
    "requestBody": {
      "required": true,
      "contentTypes": [
        "application/json"
      ],
      "schemaRefs": [
        "#/components/schemas/ProjectMemberInput"
      ]
    },
    "inputSchema": {
      "type": "object",
      "properties": {
        "projectId": {
          "type": "string"
        },
        "body": {
          "ref": "#/components/schemas/ProjectMemberInput",
          "type": "object",
          "required": [
            "email",
            "role"
          ],
          "properties": {
            "email": {
              "type": "string"
            },
            "role": {
              "type": "string",
              "enum": [
                "owner",
                "admin",
                "developer",
                "viewer"
              ]
            }
          }
        }
      },
      "required": [
        "body",
        "projectId"
      ],
      "additionalProperties": false
    }
  },
  {
    "method": "put",
    "path": "/api/v1/projects/{projectId}/members/{memberId}",
    "tags": [
      "Projects"
    ],
    "deprecated": false,
    "security": [],
    "parameters": [
      {
        "name": "projectId",
        "in": "path",
        "required": true,
        "ref": "#/components/parameters/ProjectId",
        "schema": {
          "type": "string"
        }
      },
      {
        "name": "memberId",
        "in": "path",
        "required": true,
        "ref": "#/components/parameters/MemberId",
        "schema": {
          "type": "string"
        }
      }
    ],
    "responses": [
      {
        "status": "200",
        "contentTypes": [],
        "schemaRefs": [],
        "description": "Updated project member."
      }
    ],
    "summary": "Update project member",
    "requestBody": {
      "required": true,
      "contentTypes": [
        "application/json"
      ],
      "schemaRefs": [
        "#/components/schemas/ProjectMemberInput"
      ]
    },
    "inputSchema": {
      "type": "object",
      "properties": {
        "projectId": {
          "type": "string"
        },
        "memberId": {
          "type": "string"
        },
        "body": {
          "ref": "#/components/schemas/ProjectMemberInput",
          "type": "object",
          "required": [
            "email",
            "role"
          ],
          "properties": {
            "email": {
              "type": "string"
            },
            "role": {
              "type": "string",
              "enum": [
                "owner",
                "admin",
                "developer",
                "viewer"
              ]
            }
          }
        }
      },
      "required": [
        "body",
        "memberId",
        "projectId"
      ],
      "additionalProperties": false
    }
  },
  {
    "method": "delete",
    "path": "/api/v1/projects/{projectId}/members/{memberId}",
    "tags": [
      "Projects"
    ],
    "deprecated": false,
    "security": [],
    "parameters": [
      {
        "name": "projectId",
        "in": "path",
        "required": true,
        "ref": "#/components/parameters/ProjectId",
        "schema": {
          "type": "string"
        }
      },
      {
        "name": "memberId",
        "in": "path",
        "required": true,
        "ref": "#/components/parameters/MemberId",
        "schema": {
          "type": "string"
        }
      }
    ],
    "responses": [
      {
        "status": "204",
        "contentTypes": [],
        "schemaRefs": [],
        "description": "Deleted project member."
      }
    ],
    "summary": "Delete project member",
    "inputSchema": {
      "type": "object",
      "properties": {
        "projectId": {
          "type": "string"
        },
        "memberId": {
          "type": "string"
        }
      },
      "required": [
        "memberId",
        "projectId"
      ],
      "additionalProperties": false
    }
  },
  {
    "method": "get",
    "path": "/api/v1/projects/{projectId}/applications",
    "tags": [
      "Applications"
    ],
    "deprecated": false,
    "security": [],
    "parameters": [
      {
        "name": "projectId",
        "in": "path",
        "required": true,
        "ref": "#/components/parameters/ProjectId",
        "schema": {
          "type": "string"
        }
      },
      {
        "name": "page",
        "in": "query",
        "ref": "#/components/parameters/Page",
        "schema": {
          "type": "integer",
          "default": 1,
          "minimum": 1
        }
      },
      {
        "name": "pageSize",
        "in": "query",
        "ref": "#/components/parameters/PageSize",
        "schema": {
          "type": "integer",
          "default": 20,
          "minimum": 1,
          "maximum": 100
        }
      },
      {
        "name": "search",
        "in": "query",
        "schema": {
          "type": "string"
        }
      },
      {
        "name": "sortBy",
        "in": "query",
        "schema": {
          "type": "string",
          "enum": [
            "createdAt",
            "name",
            "identifier"
          ],
          "default": "createdAt"
        }
      },
      {
        "name": "sortOrder",
        "in": "query",
        "ref": "#/components/parameters/SortOrder",
        "schema": {
          "type": "string",
          "enum": [
            "asc",
            "desc"
          ],
          "default": "desc"
        }
      }
    ],
    "responses": [
      {
        "status": "200",
        "contentTypes": [],
        "schemaRefs": [],
        "description": "Application list or paginated application list."
      }
    ],
    "summary": "List applications",
    "inputSchema": {
      "type": "object",
      "properties": {
        "projectId": {
          "type": "string"
        },
        "page": {
          "type": "integer",
          "default": 1,
          "minimum": 1
        },
        "pageSize": {
          "type": "integer",
          "default": 20,
          "minimum": 1,
          "maximum": 100
        },
        "search": {
          "type": "string"
        },
        "sortBy": {
          "type": "string",
          "enum": [
            "createdAt",
            "name",
            "identifier"
          ],
          "default": "createdAt"
        },
        "sortOrder": {
          "type": "string",
          "enum": [
            "asc",
            "desc"
          ],
          "default": "desc"
        }
      },
      "required": [
        "projectId"
      ],
      "additionalProperties": false
    }
  },
  {
    "method": "post",
    "path": "/api/v1/projects/{projectId}/applications",
    "tags": [
      "Applications"
    ],
    "deprecated": false,
    "security": [],
    "parameters": [
      {
        "name": "projectId",
        "in": "path",
        "required": true,
        "ref": "#/components/parameters/ProjectId",
        "schema": {
          "type": "string"
        }
      }
    ],
    "responses": [
      {
        "status": "201",
        "contentTypes": [],
        "schemaRefs": [],
        "description": "Created application."
      }
    ],
    "summary": "Create application",
    "requestBody": {
      "required": true,
      "contentTypes": [
        "application/json"
      ],
      "schemaRefs": [
        "#/components/schemas/ApplicationInput"
      ]
    },
    "inputSchema": {
      "type": "object",
      "properties": {
        "projectId": {
          "type": "string"
        },
        "body": {
          "ref": "#/components/schemas/ApplicationInput",
          "type": "object",
          "required": [
            "identifier",
            "name",
            "sourceType"
          ],
          "properties": {
            "buildContext": {
              "type": "string"
            },
            "dockerfilePath": {
              "type": "string"
            },
            "identifier": {
              "type": "string",
              "description": "Immutable application identifier, unique within its project space and used to derive stable resource IDs.",
              "minLength": 2,
              "maxLength": 22,
              "pattern": "^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$"
            },
            "imageReference": {
              "type": "string"
            },
            "name": {
              "type": "string"
            },
            "repositoryUrl": {
              "type": "string"
            },
            "servicePort": {
              "type": "integer"
            },
            "sourceType": {
              "type": "string",
              "enum": [
                "repository",
                "image"
              ]
            }
          }
        }
      },
      "required": [
        "body",
        "projectId"
      ],
      "additionalProperties": false
    }
  },
  {
    "method": "get",
    "path": "/api/v1/projects/{projectId}/applications/{applicationId}",
    "tags": [
      "Applications"
    ],
    "deprecated": false,
    "security": [],
    "parameters": [
      {
        "name": "projectId",
        "in": "path",
        "required": true,
        "ref": "#/components/parameters/ProjectId",
        "schema": {
          "type": "string"
        }
      },
      {
        "name": "applicationId",
        "in": "path",
        "required": true,
        "ref": "#/components/parameters/ApplicationId",
        "schema": {
          "type": "string"
        }
      }
    ],
    "responses": [
      {
        "status": "200",
        "contentTypes": [],
        "schemaRefs": [],
        "description": "Application."
      }
    ],
    "summary": "Get application",
    "inputSchema": {
      "type": "object",
      "properties": {
        "projectId": {
          "type": "string"
        },
        "applicationId": {
          "type": "string"
        }
      },
      "required": [
        "applicationId",
        "projectId"
      ],
      "additionalProperties": false
    }
  },
  {
    "method": "put",
    "path": "/api/v1/projects/{projectId}/applications/{applicationId}",
    "tags": [
      "Applications"
    ],
    "deprecated": false,
    "security": [],
    "parameters": [
      {
        "name": "projectId",
        "in": "path",
        "required": true,
        "ref": "#/components/parameters/ProjectId",
        "schema": {
          "type": "string"
        }
      },
      {
        "name": "applicationId",
        "in": "path",
        "required": true,
        "ref": "#/components/parameters/ApplicationId",
        "schema": {
          "type": "string"
        }
      }
    ],
    "responses": [
      {
        "status": "200",
        "contentTypes": [],
        "schemaRefs": [],
        "description": "Updated application."
      }
    ],
    "summary": "Update application",
    "requestBody": {
      "required": true,
      "contentTypes": [
        "application/json"
      ],
      "schemaRefs": [
        "#/components/schemas/ApplicationInput"
      ]
    },
    "inputSchema": {
      "type": "object",
      "properties": {
        "projectId": {
          "type": "string"
        },
        "applicationId": {
          "type": "string"
        },
        "body": {
          "ref": "#/components/schemas/ApplicationInput",
          "type": "object",
          "required": [
            "identifier",
            "name",
            "sourceType"
          ],
          "properties": {
            "buildContext": {
              "type": "string"
            },
            "dockerfilePath": {
              "type": "string"
            },
            "identifier": {
              "type": "string",
              "description": "Immutable application identifier, unique within its project space and used to derive stable resource IDs.",
              "minLength": 2,
              "maxLength": 22,
              "pattern": "^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$"
            },
            "imageReference": {
              "type": "string"
            },
            "name": {
              "type": "string"
            },
            "repositoryUrl": {
              "type": "string"
            },
            "servicePort": {
              "type": "integer"
            },
            "sourceType": {
              "type": "string",
              "enum": [
                "repository",
                "image"
              ]
            }
          }
        }
      },
      "required": [
        "applicationId",
        "body",
        "projectId"
      ],
      "additionalProperties": false
    }
  },
  {
    "method": "delete",
    "path": "/api/v1/projects/{projectId}/applications/{applicationId}",
    "tags": [
      "Applications"
    ],
    "deprecated": false,
    "security": [],
    "parameters": [
      {
        "name": "projectId",
        "in": "path",
        "required": true,
        "ref": "#/components/parameters/ProjectId",
        "schema": {
          "type": "string"
        }
      },
      {
        "name": "applicationId",
        "in": "path",
        "required": true,
        "ref": "#/components/parameters/ApplicationId",
        "schema": {
          "type": "string"
        }
      }
    ],
    "responses": [
      {
        "status": "204",
        "contentTypes": [],
        "schemaRefs": [],
        "description": "Deleted application."
      }
    ],
    "summary": "Delete application",
    "inputSchema": {
      "type": "object",
      "properties": {
        "projectId": {
          "type": "string"
        },
        "applicationId": {
          "type": "string"
        }
      },
      "required": [
        "applicationId",
        "projectId"
      ],
      "additionalProperties": false
    }
  },
  {
    "method": "get",
    "path": "/api/v1/projects/{projectId}/applications/{applicationId}/topology",
    "tags": [
      "Applications"
    ],
    "deprecated": false,
    "security": [],
    "parameters": [
      {
        "name": "projectId",
        "in": "path",
        "required": true,
        "ref": "#/components/parameters/ProjectId",
        "schema": {
          "type": "string"
        }
      },
      {
        "name": "applicationId",
        "in": "path",
        "required": true,
        "ref": "#/components/parameters/ApplicationId",
        "schema": {
          "type": "string"
        }
      }
    ],
    "responses": [
      {
        "status": "200",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/ApplicationTopology"
        ],
        "description": "Live application topology. Unavailable deployment targets are returned as warnings while readable targets remain available."
      }
    ],
    "summary": "Compute the current Kubernetes resource topology for an application",
    "inputSchema": {
      "type": "object",
      "properties": {
        "projectId": {
          "type": "string"
        },
        "applicationId": {
          "type": "string"
        }
      },
      "required": [
        "applicationId",
        "projectId"
      ],
      "additionalProperties": false
    }
  },
  {
    "method": "get",
    "path": "/api/v1/projects/{projectId}/applications/{applicationId}/deployment-targets",
    "tags": [
      "Deployments"
    ],
    "deprecated": false,
    "security": [],
    "parameters": [
      {
        "name": "projectId",
        "in": "path",
        "required": true,
        "ref": "#/components/parameters/ProjectId",
        "schema": {
          "type": "string"
        }
      },
      {
        "name": "applicationId",
        "in": "path",
        "required": true,
        "ref": "#/components/parameters/ApplicationId",
        "schema": {
          "type": "string"
        }
      }
    ],
    "responses": [
      {
        "status": "200",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/DeploymentTarget"
        ],
        "description": "Deployment target list."
      }
    ],
    "summary": "List deployment targets for an application",
    "inputSchema": {
      "type": "object",
      "properties": {
        "projectId": {
          "type": "string"
        },
        "applicationId": {
          "type": "string"
        }
      },
      "required": [
        "applicationId",
        "projectId"
      ],
      "additionalProperties": false
    }
  },
  {
    "method": "post",
    "path": "/api/v1/projects/{projectId}/applications/{applicationId}/deployment-targets",
    "tags": [
      "Deployments"
    ],
    "deprecated": false,
    "security": [],
    "parameters": [
      {
        "name": "projectId",
        "in": "path",
        "required": true,
        "ref": "#/components/parameters/ProjectId",
        "schema": {
          "type": "string"
        }
      },
      {
        "name": "applicationId",
        "in": "path",
        "required": true,
        "ref": "#/components/parameters/ApplicationId",
        "schema": {
          "type": "string"
        }
      }
    ],
    "responses": [
      {
        "status": "201",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/DeploymentTarget"
        ],
        "description": "Created deployment target."
      }
    ],
    "summary": "Create a deployment target",
    "requestBody": {
      "required": true,
      "contentTypes": [
        "application/json"
      ],
      "schemaRefs": [
        "#/components/schemas/DeploymentTargetInput"
      ]
    },
    "inputSchema": {
      "type": "object",
      "properties": {
        "projectId": {
          "type": "string"
        },
        "applicationId": {
          "type": "string"
        },
        "body": {
          "ref": "#/components/schemas/DeploymentTargetInput",
          "type": "object",
          "description": "Deployment target create/update payload. Other build and Kubernetes fields are accepted by the implemented endpoint; `webConsoleEnabled` is documented here because it has inherited policy semantics.",
          "properties": {
            "buildDefinitionMode": {
              "type": "string",
              "description": "Selects the repository Dockerfile or a platform-rendered template Dockerfile.",
              "enum": [
                "repository_dockerfile",
                "template"
              ],
              "default": "repository_dockerfile"
            },
            "buildSecrets": {
              "type": "object",
              "description": "Optional deployment-level secret updates. Existing keys with an empty value are retained; omitted keys are removed. Values are encrypted and never returned.",
              "writeOnly": true,
              "additionalProperties": {
                "type": "string"
              }
            },
            "buildTemplateId": {
              "type": "string",
              "description": "Required when buildDefinitionMode is template."
            },
            "buildTemplateValues": {
              "type": "string",
              "description": "JSON object containing validated template parameters."
            },
            "buildTemplateVersion": {
              "type": "string",
              "description": "Immutable built-in template version. An empty value selects the current version."
            },
            "buildVariables": {
              "type": "object",
              "description": "Optional deployment-level values that override matching application, project, and global keys.",
              "additionalProperties": {
                "type": "string"
              }
            },
            "stage": {
              "type": "string",
              "description": "Immutable deployment stage identifier, unique within the application.",
              "minLength": 2,
              "maxLength": 12,
              "pattern": "^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$"
            },
            "webConsoleEnabled": {
              "type": [
                "boolean",
                "null"
              ],
              "description": "`null` inherits the project-space master switch and `false` disables Web Console for this deployment target. `true` is normalized to inheritance for compatibility and cannot bypass a disabled project-space switch.",
              "default": null
            }
          },
          "additionalProperties": true
        }
      },
      "required": [
        "applicationId",
        "body",
        "projectId"
      ],
      "additionalProperties": false
    }
  },
  {
    "method": "put",
    "path": "/api/v1/projects/{projectId}/applications/{applicationId}/deployment-targets/{targetId}",
    "tags": [
      "Deployments"
    ],
    "deprecated": false,
    "security": [],
    "parameters": [
      {
        "name": "projectId",
        "in": "path",
        "required": true,
        "ref": "#/components/parameters/ProjectId",
        "schema": {
          "type": "string"
        }
      },
      {
        "name": "applicationId",
        "in": "path",
        "required": true,
        "ref": "#/components/parameters/ApplicationId",
        "schema": {
          "type": "string"
        }
      },
      {
        "name": "targetId",
        "in": "path",
        "required": true,
        "ref": "#/components/parameters/TargetId",
        "schema": {
          "type": "string"
        }
      }
    ],
    "responses": [
      {
        "status": "200",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/DeploymentTarget"
        ],
        "description": "Updated deployment target."
      }
    ],
    "summary": "Update a deployment target",
    "requestBody": {
      "required": true,
      "contentTypes": [
        "application/json"
      ],
      "schemaRefs": [
        "#/components/schemas/DeploymentTargetInput"
      ]
    },
    "inputSchema": {
      "type": "object",
      "properties": {
        "projectId": {
          "type": "string"
        },
        "applicationId": {
          "type": "string"
        },
        "targetId": {
          "type": "string"
        },
        "body": {
          "ref": "#/components/schemas/DeploymentTargetInput",
          "type": "object",
          "description": "Deployment target create/update payload. Other build and Kubernetes fields are accepted by the implemented endpoint; `webConsoleEnabled` is documented here because it has inherited policy semantics.",
          "properties": {
            "buildDefinitionMode": {
              "type": "string",
              "description": "Selects the repository Dockerfile or a platform-rendered template Dockerfile.",
              "enum": [
                "repository_dockerfile",
                "template"
              ],
              "default": "repository_dockerfile"
            },
            "buildSecrets": {
              "type": "object",
              "description": "Optional deployment-level secret updates. Existing keys with an empty value are retained; omitted keys are removed. Values are encrypted and never returned.",
              "writeOnly": true,
              "additionalProperties": {
                "type": "string"
              }
            },
            "buildTemplateId": {
              "type": "string",
              "description": "Required when buildDefinitionMode is template."
            },
            "buildTemplateValues": {
              "type": "string",
              "description": "JSON object containing validated template parameters."
            },
            "buildTemplateVersion": {
              "type": "string",
              "description": "Immutable built-in template version. An empty value selects the current version."
            },
            "buildVariables": {
              "type": "object",
              "description": "Optional deployment-level values that override matching application, project, and global keys.",
              "additionalProperties": {
                "type": "string"
              }
            },
            "stage": {
              "type": "string",
              "description": "Immutable deployment stage identifier, unique within the application.",
              "minLength": 2,
              "maxLength": 12,
              "pattern": "^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$"
            },
            "webConsoleEnabled": {
              "type": [
                "boolean",
                "null"
              ],
              "description": "`null` inherits the project-space master switch and `false` disables Web Console for this deployment target. `true` is normalized to inheritance for compatibility and cannot bypass a disabled project-space switch.",
              "default": null
            }
          },
          "additionalProperties": true
        }
      },
      "required": [
        "applicationId",
        "body",
        "projectId",
        "targetId"
      ],
      "additionalProperties": false
    }
  },
  {
    "method": "delete",
    "path": "/api/v1/projects/{projectId}/applications/{applicationId}/deployment-targets/{targetId}",
    "tags": [
      "Deployments"
    ],
    "deprecated": false,
    "security": [],
    "parameters": [
      {
        "name": "projectId",
        "in": "path",
        "required": true,
        "ref": "#/components/parameters/ProjectId",
        "schema": {
          "type": "string"
        }
      },
      {
        "name": "applicationId",
        "in": "path",
        "required": true,
        "ref": "#/components/parameters/ApplicationId",
        "schema": {
          "type": "string"
        }
      },
      {
        "name": "targetId",
        "in": "path",
        "required": true,
        "ref": "#/components/parameters/TargetId",
        "schema": {
          "type": "string"
        }
      }
    ],
    "responses": [
      {
        "status": "204",
        "contentTypes": [],
        "schemaRefs": [],
        "description": "Deployment target deletion accepted and queued for asynchronous runtime cleanup."
      }
    ],
    "summary": "Delete a deployment target",
    "inputSchema": {
      "type": "object",
      "properties": {
        "projectId": {
          "type": "string"
        },
        "applicationId": {
          "type": "string"
        },
        "targetId": {
          "type": "string"
        }
      },
      "required": [
        "applicationId",
        "projectId",
        "targetId"
      ],
      "additionalProperties": false
    }
  },
  {
    "method": "get",
    "path": "/api/v1/projects/{projectId}/applications/{applicationId}/deployment-targets/{targetId}/data-export",
    "tags": [
      "Deployments"
    ],
    "deprecated": false,
    "security": [
      {
        "SessionCookie": []
      }
    ],
    "parameters": [
      {
        "name": "projectId",
        "in": "path",
        "required": true,
        "ref": "#/components/parameters/ProjectId",
        "schema": {
          "type": "string"
        }
      },
      {
        "name": "applicationId",
        "in": "path",
        "required": true,
        "ref": "#/components/parameters/ApplicationId",
        "schema": {
          "type": "string"
        }
      },
      {
        "name": "targetId",
        "in": "path",
        "required": true,
        "ref": "#/components/parameters/TargetId",
        "schema": {
          "type": "string"
        }
      },
      {
        "name": "ticket",
        "in": "query",
        "required": true,
        "description": "One-time export ticket returned by the authorize endpoint. It expires after 60 seconds and is consumed even when its resource binding does not match.",
        "schema": {
          "type": "string"
        }
      }
    ],
    "responses": [
      {
        "status": "200",
        "contentTypes": [
          "application/gzip"
        ],
        "schemaRefs": [],
        "description": "Gzip-compressed tar archive streamed as an attachment."
      },
      {
        "status": "400",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/ErrorResponse"
        ],
        "description": "Runtime data retention is disabled, the runtime cluster cannot export the target data, or the ticket is missing (`data_export.ticket_required`)."
      },
      {
        "status": "401",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/ErrorResponse"
        ],
        "description": "Interactive session cookie is missing or invalid (`auth.session.missing` or another authentication error)."
      },
      {
        "status": "403",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/ErrorResponse"
        ],
        "description": "A personal access token was used (`auth.interactive_session_required`), the role is insufficient, MFA is required, or the ticket is invalid/expired/consumed/bound to another request (`data_export.ticket_invalid`)."
      },
      {
        "status": "404",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/ErrorResponse"
        ],
        "description": "Project, application, deployment target, or runtime dependency was not found."
      },
      {
        "status": "409",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/ErrorResponse"
        ],
        "description": "The project, application, or deployment target is being deleted and cannot be exported."
      },
      {
        "status": "502",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/ErrorResponse"
        ],
        "description": "The temporary export Pod or archive stream could not be started (`data_export.stream_failed`)."
      },
      {
        "status": "503",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/ErrorResponse"
        ],
        "description": "The shared production ticket store is unavailable (`data_export.ticket_unavailable`)."
      }
    ],
    "summary": "Export persistent runtime data",
    "description": "Consumes a short-lived, one-time export ticket issued by the authorize endpoint, then repeats the interactive session, project Owner/Admin, resource-state, and `data_export` Step-up checks. Personal access tokens are rejected. Each export uses an isolated temporary Pod and streams a gzip archive without persisting the ticket or archive in business tables.",
    "inputSchema": {
      "type": "object",
      "properties": {
        "projectId": {
          "type": "string"
        },
        "applicationId": {
          "type": "string"
        },
        "targetId": {
          "type": "string"
        },
        "ticket": {
          "type": "string"
        }
      },
      "required": [
        "applicationId",
        "projectId",
        "targetId",
        "ticket"
      ],
      "additionalProperties": false
    }
  },
  {
    "method": "post",
    "path": "/api/v1/projects/{projectId}/applications/{applicationId}/deployment-targets/{targetId}/data-export/authorize",
    "tags": [
      "Deployments"
    ],
    "deprecated": false,
    "security": [
      {
        "SessionCookie": []
      }
    ],
    "parameters": [
      {
        "name": "projectId",
        "in": "path",
        "required": true,
        "ref": "#/components/parameters/ProjectId",
        "schema": {
          "type": "string"
        }
      },
      {
        "name": "applicationId",
        "in": "path",
        "required": true,
        "ref": "#/components/parameters/ApplicationId",
        "schema": {
          "type": "string"
        }
      },
      {
        "name": "targetId",
        "in": "path",
        "required": true,
        "ref": "#/components/parameters/TargetId",
        "schema": {
          "type": "string"
        }
      }
    ],
    "responses": [
      {
        "status": "200",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/DataExportAuthorization"
        ],
        "description": "Data-export ticket issued. The download endpoint still repeats authorization and atomically consumes the ticket."
      },
      {
        "status": "400",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/ErrorResponse"
        ],
        "description": "Runtime data retention is disabled or the runtime cluster cannot export the target data."
      },
      {
        "status": "401",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/ErrorResponse"
        ],
        "description": "Interactive browser session is missing or invalid."
      },
      {
        "status": "403",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/ErrorResponse"
        ],
        "description": "Project role is insufficient, a personal access token was used, or `data_export` Step-up verification is required."
      },
      {
        "status": "404",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/ErrorResponse"
        ],
        "description": "Project, application, deployment target, or runtime dependency was not found."
      },
      {
        "status": "409",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/ErrorResponse"
        ],
        "description": "Project, application, or deployment target is being deleted."
      },
      {
        "status": "503",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/ErrorResponse"
        ],
        "description": "The shared production ticket store is unavailable (`data_export.ticket_unavailable`)."
      }
    ],
    "summary": "Authorize a persistent runtime data export",
    "description": "Requires an interactive project Owner/Admin session, a mutable project/application/deployment target, exportable runtime data, and an active `data_export` Step-up assertion when the global policy is enabled. Returns a random 60-second one-time ticket bound to the current user, session, project, application, and deployment target. Production uses the shared Redis ticket store and fails closed when Redis is unavailable.",
    "inputSchema": {
      "type": "object",
      "properties": {
        "projectId": {
          "type": "string"
        },
        "applicationId": {
          "type": "string"
        },
        "targetId": {
          "type": "string"
        }
      },
      "required": [
        "applicationId",
        "projectId",
        "targetId"
      ],
      "additionalProperties": false
    }
  },
  {
    "method": "post",
    "path": "/api/v1/projects/{projectId}/releases/{releaseId}/terminal/authorize",
    "tags": [
      "Deployments"
    ],
    "deprecated": false,
    "security": [
      {
        "SessionCookie": []
      }
    ],
    "parameters": [
      {
        "name": "projectId",
        "in": "path",
        "required": true,
        "ref": "#/components/parameters/ProjectId",
        "schema": {
          "type": "string"
        }
      },
      {
        "name": "releaseId",
        "in": "path",
        "required": true,
        "ref": "#/components/parameters/ReleaseId",
        "schema": {
          "type": "string"
        }
      }
    ],
    "responses": [
      {
        "status": "204",
        "contentTypes": [],
        "schemaRefs": [],
        "description": "Terminal preflight authorized. The WebSocket endpoint must still perform its own authorization checks."
      },
      {
        "status": "401",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/ErrorResponse"
        ],
        "description": "Interactive browser session is missing or invalid."
      },
      {
        "status": "403",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/ErrorResponse"
        ],
        "description": "Project role is insufficient, Web Console is disabled (`runtime.web_console_disabled`), a personal access token was used (`mfa.session_required`), or Step-up verification is required (`mfa_required` with purpose `runtime_terminal`)."
      },
      {
        "status": "404",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/ErrorResponse"
        ],
        "description": "Project, release, or deployment target was not found."
      },
      {
        "status": "409",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/ErrorResponse"
        ],
        "description": "Project or deployment target is being deleted and cannot open Web Console."
      }
    ],
    "summary": "Authorize a release Web Console terminal connection",
    "description": "Normal HTTP preflight used before opening the release terminal WebSocket. It verifies project Owner/Admin/Developer access, project and deployment-target mutation state, the effective project/deployment `webConsoleEnabled` policy, and the `runtime_terminal` Step-up assertion. A missing assertion returns `mfa_required`, allowing the frontend to show the MFA dialog and retry. A 204 authorizes only the preflight; the WebSocket repeats all checks before upgrading and revalidates session, membership, role, resource state, Web Console policy, and assertion every three seconds while connected. Revocation or expiry closes the shell.",
    "inputSchema": {
      "type": "object",
      "properties": {
        "projectId": {
          "type": "string"
        },
        "releaseId": {
          "type": "string"
        }
      },
      "required": [
        "projectId",
        "releaseId"
      ],
      "additionalProperties": false
    }
  },
  {
    "method": "get",
    "path": "/api/v1/projects/{projectId}/applications/{applicationId}/deployment-targets/{targetId}/release-image-candidates",
    "tags": [
      "Applications"
    ],
    "deprecated": false,
    "security": [],
    "parameters": [
      {
        "name": "projectId",
        "in": "path",
        "required": true,
        "ref": "#/components/parameters/ProjectId",
        "schema": {
          "type": "string"
        }
      },
      {
        "name": "applicationId",
        "in": "path",
        "required": true,
        "ref": "#/components/parameters/ApplicationId",
        "schema": {
          "type": "string"
        }
      },
      {
        "name": "targetId",
        "in": "path",
        "required": true,
        "ref": "#/components/parameters/TargetId",
        "schema": {
          "type": "string"
        }
      }
    ],
    "responses": [
      {
        "status": "200",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/ReleaseImageCandidates"
        ],
        "description": "Release image candidates."
      }
    ],
    "summary": "List release image candidates",
    "description": "Reads tags from the target registry first and falls back to saved build records when the registry is unavailable.",
    "inputSchema": {
      "type": "object",
      "properties": {
        "projectId": {
          "type": "string"
        },
        "applicationId": {
          "type": "string"
        },
        "targetId": {
          "type": "string"
        }
      },
      "required": [
        "applicationId",
        "projectId",
        "targetId"
      ],
      "additionalProperties": false
    }
  },
  {
    "method": "get",
    "path": "/api/v1/projects/{projectId}/repository-bindings",
    "tags": [
      "Git"
    ],
    "deprecated": false,
    "security": [],
    "parameters": [
      {
        "name": "projectId",
        "in": "path",
        "required": true,
        "ref": "#/components/parameters/ProjectId",
        "schema": {
          "type": "string"
        }
      }
    ],
    "responses": [
      {
        "status": "200",
        "contentTypes": [],
        "schemaRefs": [],
        "description": "Repository binding list."
      }
    ],
    "summary": "List repository bindings for a project",
    "inputSchema": {
      "type": "object",
      "properties": {
        "projectId": {
          "type": "string"
        }
      },
      "required": [
        "projectId"
      ],
      "additionalProperties": false
    }
  },
  {
    "method": "post",
    "path": "/api/v1/projects/{projectId}/repository-bindings",
    "tags": [
      "Git"
    ],
    "deprecated": false,
    "security": [],
    "parameters": [
      {
        "name": "projectId",
        "in": "path",
        "required": true,
        "ref": "#/components/parameters/ProjectId",
        "schema": {
          "type": "string"
        }
      }
    ],
    "responses": [
      {
        "status": "201",
        "contentTypes": [],
        "schemaRefs": [],
        "description": "Created repository binding."
      }
    ],
    "summary": "Bind an application to a Git repository",
    "requestBody": {
      "required": true,
      "contentTypes": [
        "application/json"
      ],
      "schemaRefs": [
        "#/components/schemas/RepositoryBindingInput"
      ]
    },
    "inputSchema": {
      "type": "object",
      "properties": {
        "projectId": {
          "type": "string"
        },
        "body": {
          "ref": "#/components/schemas/RepositoryBindingInput",
          "type": "object",
          "required": [
            "applicationId",
            "gitAccountId",
            "owner",
            "repo"
          ],
          "properties": {
            "applicationId": {
              "type": "string"
            },
            "cloneUrl": {
              "type": "string"
            },
            "defaultBranch": {
              "type": "string"
            },
            "gitAccountId": {
              "type": "string"
            },
            "owner": {
              "type": "string"
            },
            "repo": {
              "type": "string"
            },
            "webhookStatus": {
              "type": "string",
              "enum": [
                "pending",
                "created",
                "disabled",
                "failed"
              ]
            }
          }
        }
      },
      "required": [
        "body",
        "projectId"
      ],
      "additionalProperties": false
    }
  },
  {
    "method": "put",
    "path": "/api/v1/projects/{projectId}/repository-bindings/{bindingId}",
    "tags": [
      "Git"
    ],
    "deprecated": false,
    "security": [],
    "parameters": [
      {
        "name": "projectId",
        "in": "path",
        "required": true,
        "ref": "#/components/parameters/ProjectId",
        "schema": {
          "type": "string"
        }
      },
      {
        "name": "bindingId",
        "in": "path",
        "required": true,
        "ref": "#/components/parameters/BindingId",
        "schema": {
          "type": "string"
        }
      }
    ],
    "responses": [
      {
        "status": "200",
        "contentTypes": [],
        "schemaRefs": [],
        "description": "Updated repository binding."
      }
    ],
    "summary": "Update repository binding",
    "requestBody": {
      "required": true,
      "contentTypes": [
        "application/json"
      ],
      "schemaRefs": [
        "#/components/schemas/RepositoryBindingInput"
      ]
    },
    "inputSchema": {
      "type": "object",
      "properties": {
        "projectId": {
          "type": "string"
        },
        "bindingId": {
          "type": "string"
        },
        "body": {
          "ref": "#/components/schemas/RepositoryBindingInput",
          "type": "object",
          "required": [
            "applicationId",
            "gitAccountId",
            "owner",
            "repo"
          ],
          "properties": {
            "applicationId": {
              "type": "string"
            },
            "cloneUrl": {
              "type": "string"
            },
            "defaultBranch": {
              "type": "string"
            },
            "gitAccountId": {
              "type": "string"
            },
            "owner": {
              "type": "string"
            },
            "repo": {
              "type": "string"
            },
            "webhookStatus": {
              "type": "string",
              "enum": [
                "pending",
                "created",
                "disabled",
                "failed"
              ]
            }
          }
        }
      },
      "required": [
        "bindingId",
        "body",
        "projectId"
      ],
      "additionalProperties": false
    }
  },
  {
    "method": "delete",
    "path": "/api/v1/projects/{projectId}/repository-bindings/{bindingId}",
    "tags": [
      "Git"
    ],
    "deprecated": false,
    "security": [],
    "parameters": [
      {
        "name": "projectId",
        "in": "path",
        "required": true,
        "ref": "#/components/parameters/ProjectId",
        "schema": {
          "type": "string"
        }
      },
      {
        "name": "bindingId",
        "in": "path",
        "required": true,
        "ref": "#/components/parameters/BindingId",
        "schema": {
          "type": "string"
        }
      }
    ],
    "responses": [
      {
        "status": "204",
        "contentTypes": [],
        "schemaRefs": [],
        "description": "Deleted repository binding."
      }
    ],
    "summary": "Delete repository binding",
    "inputSchema": {
      "type": "object",
      "properties": {
        "projectId": {
          "type": "string"
        },
        "bindingId": {
          "type": "string"
        }
      },
      "required": [
        "bindingId",
        "projectId"
      ],
      "additionalProperties": false
    }
  },
  {
    "method": "post",
    "path": "/api/v1/projects/{projectId}/repository-bindings/{bindingId}/webhook",
    "tags": [
      "Git"
    ],
    "deprecated": false,
    "security": [],
    "parameters": [
      {
        "name": "projectId",
        "in": "path",
        "required": true,
        "ref": "#/components/parameters/ProjectId",
        "schema": {
          "type": "string"
        }
      },
      {
        "name": "bindingId",
        "in": "path",
        "required": true,
        "ref": "#/components/parameters/BindingId",
        "schema": {
          "type": "string"
        }
      }
    ],
    "responses": [
      {
        "status": "200",
        "contentTypes": [],
        "schemaRefs": [],
        "description": "Created repository webhook."
      }
    ],
    "summary": "Create webhook for a repository binding",
    "inputSchema": {
      "type": "object",
      "properties": {
        "projectId": {
          "type": "string"
        },
        "bindingId": {
          "type": "string"
        }
      },
      "required": [
        "bindingId",
        "projectId"
      ],
      "additionalProperties": false
    }
  },
  {
    "method": "get",
    "path": "/api/v1/access-tokens/scopes",
    "tags": [
      "AccessTokens"
    ],
    "deprecated": false,
    "security": [],
    "parameters": [],
    "responses": [
      {
        "status": "200",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/AccessTokenScopeDefinition"
        ],
        "description": "Access-token scope catalog."
      },
      {
        "status": "401",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/ErrorResponse"
        ],
        "description": "Authentication is required."
      }
    ],
    "summary": "List access-token scope definitions",
    "description": "Returns the canonical scope catalog and the current user's scope-creation constraints.",
    "operationId": "listAccessTokenScopes",
    "xLunaCli": {
      "command": "access-token.scope-list",
      "classification": "business-command",
      "risk": "low",
      "requiredScopes": [
        "token:manage"
      ]
    }
  },
  {
    "method": "get",
    "path": "/api/v1/access-tokens",
    "tags": [
      "AccessTokens"
    ],
    "deprecated": false,
    "security": [],
    "parameters": [
      {
        "name": "page",
        "in": "query",
        "ref": "#/components/parameters/Page",
        "schema": {
          "type": "integer",
          "default": 1,
          "minimum": 1
        }
      },
      {
        "name": "pageSize",
        "in": "query",
        "ref": "#/components/parameters/PageSize",
        "schema": {
          "type": "integer",
          "default": 20,
          "minimum": 1,
          "maximum": 100
        }
      },
      {
        "name": "sortBy",
        "in": "query",
        "schema": {
          "type": "string",
          "enum": [
            "createdAt",
            "expiresAt",
            "name",
            "scope",
            "status"
          ],
          "default": "createdAt"
        }
      },
      {
        "name": "sortOrder",
        "in": "query",
        "ref": "#/components/parameters/SortOrder",
        "schema": {
          "type": "string",
          "enum": [
            "asc",
            "desc"
          ],
          "default": "desc"
        }
      }
    ],
    "responses": [
      {
        "status": "200",
        "contentTypes": [],
        "schemaRefs": [],
        "description": "Paginated access token list."
      }
    ],
    "summary": "List access tokens",
    "description": "Returns only non-revoked access tokens.",
    "inputSchema": {
      "type": "object",
      "properties": {
        "page": {
          "type": "integer",
          "default": 1,
          "minimum": 1
        },
        "pageSize": {
          "type": "integer",
          "default": 20,
          "minimum": 1,
          "maximum": 100
        },
        "sortBy": {
          "type": "string",
          "enum": [
            "createdAt",
            "expiresAt",
            "name",
            "scope",
            "status"
          ],
          "default": "createdAt"
        },
        "sortOrder": {
          "type": "string",
          "enum": [
            "asc",
            "desc"
          ],
          "default": "desc"
        }
      },
      "additionalProperties": false
    }
  },
  {
    "method": "post",
    "path": "/api/v1/access-tokens",
    "tags": [
      "AccessTokens"
    ],
    "deprecated": false,
    "security": [],
    "parameters": [],
    "responses": [
      {
        "status": "201",
        "contentTypes": [],
        "schemaRefs": [],
        "description": "Created access token with one-time secret."
      }
    ],
    "summary": "Create access token",
    "requestBody": {
      "required": true,
      "contentTypes": [
        "application/json"
      ],
      "schemaRefs": [
        "#/components/schemas/AccessTokenInput"
      ]
    },
    "inputSchema": {
      "type": "object",
      "properties": {
        "body": {
          "ref": "#/components/schemas/AccessTokenInput",
          "type": "object",
          "required": [
            "name",
            "scope"
          ],
          "properties": {
            "expiresInDays": {
              "type": "integer",
              "description": "0 means never expires.",
              "enum": [
                0,
                7,
                15,
                30,
                90
              ]
            },
            "name": {
              "type": "string"
            },
            "scope": {
              "type": "string",
              "description": "Comma-separated scopes. Wildcard and unknown scopes are rejected. Normal users can create read scopes only."
            }
          }
        }
      },
      "required": [
        "body"
      ],
      "additionalProperties": false
    }
  },
  {
    "method": "delete",
    "path": "/api/v1/access-tokens/{tokenId}",
    "tags": [
      "AccessTokens"
    ],
    "deprecated": false,
    "security": [],
    "parameters": [
      {
        "name": "tokenId",
        "in": "path",
        "required": true,
        "ref": "#/components/parameters/TokenId",
        "schema": {
          "type": "string"
        }
      }
    ],
    "responses": [
      {
        "status": "204",
        "contentTypes": [],
        "schemaRefs": [],
        "description": "Revoked access token."
      }
    ],
    "summary": "Revoke access token",
    "inputSchema": {
      "type": "object",
      "properties": {
        "tokenId": {
          "type": "string"
        }
      },
      "required": [
        "tokenId"
      ],
      "additionalProperties": false
    }
  }
];

// ../packages/api-contract/src/digest.ts
var SHA256_INITIAL_STATE = [
  1779033703,
  3144134277,
  1013904242,
  2773480762,
  1359893119,
  2600822924,
  528734635,
  1541459225
];
var SHA256_ROUND_CONSTANTS = [
  1116352408,
  1899447441,
  3049323471,
  3921009573,
  961987163,
  1508970993,
  2453635748,
  2870763221,
  3624381080,
  310598401,
  607225278,
  1426881987,
  1925078388,
  2162078206,
  2614888103,
  3248222580,
  3835390401,
  4022224774,
  264347078,
  604807628,
  770255983,
  1249150122,
  1555081692,
  1996064986,
  2554220882,
  2821834349,
  2952996808,
  3210313671,
  3336571891,
  3584528711,
  113926993,
  338241895,
  666307205,
  773529912,
  1294757372,
  1396182291,
  1695183700,
  1986661051,
  2177026350,
  2456956037,
  2730485921,
  2820302411,
  3259730800,
  3345764771,
  3516065817,
  3600352804,
  4094571909,
  275423344,
  430227734,
  506948616,
  659060556,
  883997877,
  958139571,
  1322822218,
  1537002063,
  1747873779,
  1955562222,
  2024104815,
  2227730452,
  2361852424,
  2428436474,
  2756734187,
  3204031479,
  3329325298
];
function rotateRight(value, amount) {
  return value >>> amount | value << 32 - amount;
}
function encodeUtf8(value) {
  const bytes = [];
  for (let index = 0; index < value.length; index += 1) {
    let codePoint = value.charCodeAt(index);
    if (codePoint >= 55296 && codePoint <= 56319 && index + 1 < value.length) {
      const trailing = value.charCodeAt(index + 1);
      if (trailing >= 56320 && trailing <= 57343) {
        codePoint = 65536 + (codePoint - 55296 << 10) + (trailing - 56320);
        index += 1;
      }
    }
    if (codePoint >= 55296 && codePoint <= 57343) {
      codePoint = 65533;
    }
    if (codePoint < 128) {
      bytes.push(codePoint);
    } else if (codePoint < 2048) {
      bytes.push(192 | codePoint >>> 6, 128 | codePoint & 63);
    } else if (codePoint < 65536) {
      bytes.push(
        224 | codePoint >>> 12,
        128 | codePoint >>> 6 & 63,
        128 | codePoint & 63
      );
    } else {
      bytes.push(
        240 | codePoint >>> 18,
        128 | codePoint >>> 12 & 63,
        128 | codePoint >>> 6 & 63,
        128 | codePoint & 63
      );
    }
  }
  return bytes;
}
function sha256Hex(value) {
  const bytes = encodeUtf8(value);
  const bitLength = bytes.length * 8;
  const highLength = Math.floor(bitLength / 4294967296);
  const lowLength = bitLength >>> 0;
  bytes.push(128);
  while (bytes.length % 64 !== 56) {
    bytes.push(0);
  }
  bytes.push(
    highLength >>> 24 & 255,
    highLength >>> 16 & 255,
    highLength >>> 8 & 255,
    highLength & 255,
    lowLength >>> 24 & 255,
    lowLength >>> 16 & 255,
    lowLength >>> 8 & 255,
    lowLength & 255
  );
  const state = [...SHA256_INITIAL_STATE];
  const schedule = new Array(64).fill(0);
  for (let offset = 0; offset < bytes.length; offset += 64) {
    for (let index = 0; index < 16; index += 1) {
      const position = offset + index * 4;
      schedule[index] = (bytes[position] ?? 0) << 24 | (bytes[position + 1] ?? 0) << 16 | (bytes[position + 2] ?? 0) << 8 | (bytes[position + 3] ?? 0);
    }
    for (let index = 16; index < 64; index += 1) {
      const first2 = rotateRight(schedule[index - 15] ?? 0, 7) ^ rotateRight(schedule[index - 15] ?? 0, 18) ^ (schedule[index - 15] ?? 0) >>> 3;
      const second = rotateRight(schedule[index - 2] ?? 0, 17) ^ rotateRight(schedule[index - 2] ?? 0, 19) ^ (schedule[index - 2] ?? 0) >>> 10;
      schedule[index] = (schedule[index - 16] ?? 0) + first2 + (schedule[index - 7] ?? 0) + second >>> 0;
    }
    let [a, b, c, d, e, f, g, h] = state;
    for (let index = 0; index < 64; index += 1) {
      const sigmaOne = rotateRight(e, 6) ^ rotateRight(e, 11) ^ rotateRight(e, 25);
      const choose = e & f ^ ~e & g;
      const temporaryOne = h + sigmaOne + choose + (SHA256_ROUND_CONSTANTS[index] ?? 0) + (schedule[index] ?? 0) >>> 0;
      const sigmaZero = rotateRight(a, 2) ^ rotateRight(a, 13) ^ rotateRight(a, 22);
      const majority = a & b ^ a & c ^ b & c;
      const temporaryTwo = sigmaZero + majority >>> 0;
      h = g;
      g = f;
      f = e;
      e = d + temporaryOne >>> 0;
      d = c;
      c = b;
      b = a;
      a = temporaryOne + temporaryTwo >>> 0;
    }
    state[0] = state[0] + a >>> 0;
    state[1] = state[1] + b >>> 0;
    state[2] = state[2] + c >>> 0;
    state[3] = state[3] + d >>> 0;
    state[4] = state[4] + e >>> 0;
    state[5] = state[5] + f >>> 0;
    state[6] = state[6] + g >>> 0;
    state[7] = state[7] + h >>> 0;
  }
  return state.map((part) => part.toString(16).padStart(8, "0")).join("");
}
function serializeJson(value, ancestors, inArray) {
  if (value === null) {
    return "null";
  }
  switch (typeof value) {
    case "string":
    case "boolean":
      return JSON.stringify(value);
    case "number":
      if (!Number.isFinite(value)) {
        throw new TypeError("Cannot digest non-finite numbers");
      }
      return JSON.stringify(value);
    case "undefined":
      return inArray ? "null" : void 0;
    case "bigint":
    case "function":
    case "symbol":
      throw new TypeError(`Cannot digest values of type ${typeof value}`);
    case "object":
      break;
    default:
      throw new TypeError("Cannot digest unsupported value");
  }
  const object = value;
  if (ancestors.has(object)) {
    throw new TypeError("Cannot digest cyclic values");
  }
  ancestors.add(object);
  try {
    if (Array.isArray(value)) {
      return `[${value.map((item) => serializeJson(item, ancestors, true) ?? "null").join(",")}]`;
    }
    const entries = Object.keys(value).sort().flatMap((key) => {
      const serialized = serializeJson(
        value[key],
        ancestors,
        false
      );
      return serialized === void 0 ? [] : [`${JSON.stringify(key)}:${serialized}`];
    });
    return `{${entries.join(",")}}`;
  } finally {
    ancestors.delete(object);
  }
}
function stableStringify(value) {
  return serializeJson(value, /* @__PURE__ */ new Set(), false) ?? "null";
}
function digestJson(value) {
  return `sha256:${sha256Hex(stableStringify(value))}`;
}

// ../packages/api-contract/src/catalog.ts
var TAG_CATEGORY_OVERRIDES = Object.freeze({
  accesstokens: "access-token",
  applications: "application",
  builds: "build",
  configs: "config",
  dataretention: "data-retention",
  deployments: "deployment",
  projects: "project",
  registries: "registry",
  users: "user"
});
function splitWords(value) {
  return value.replace(/([a-z0-9])([A-Z])/g, "$1 $2").replace(/([A-Z]+)([A-Z][a-z])/g, "$1 $2").split(/[^A-Za-z0-9]+/).filter(Boolean).map((part) => part.toLowerCase());
}
function toKebabCase(value) {
  return splitWords(value).join("-");
}
function toSnakeCase(value) {
  return splitWords(value).join("_");
}
function normalizeOpenApiPath(path3) {
  const withLeadingSlash = path3.startsWith("/") ? path3 : `/${path3}`;
  const normalized = withLeadingSlash.replace(/\/:([A-Za-z][A-Za-z0-9_]*)/g, "/{$1}").replace(/\/+/g, "/");
  return normalized.length > 1 ? normalized.replace(/\/+$/, "") : normalized;
}
function createOperationKey(method, path3) {
  return `${method.toUpperCase()} ${normalizeOpenApiPath(path3)}`;
}
function pathSegments(path3) {
  return normalizeOpenApiPath(path3).split("/").filter(Boolean).map((segment) => {
    const parameter2 = segment.match(/^\{(.+)\}$/);
    return parameter2 ? `by-${toKebabCase(parameter2[1] ?? "parameter")}` : toKebabCase(segment);
  }).filter(Boolean);
}
function commandPathSegments(path3) {
  const segments = pathSegments(path3);
  if (segments[0] === "api" && /^v\d+$/.test(segments[1] ?? "")) {
    return segments.slice(2);
  }
  return segments;
}
function categoryFromTag(tag) {
  const normalized = toKebabCase(tag) || "api";
  return TAG_CATEGORY_OVERRIDES[normalized.replaceAll("-", "")] ?? normalized;
}
function createFallbackOperationId(tag, method, path3) {
  const tagPart = toSnakeCase(tag) || "api";
  const pathPart = pathSegments(path3).map(toSnakeCase).filter(Boolean).join("_") || "root";
  return `fallback_${tagPart}_${method}_${pathPart}`;
}
function createFallbackCommand(tag, method, path3) {
  const category = categoryFromTag(tag);
  const resourcePath = commandPathSegments(path3).join("-") || "root";
  const tool = `${method}-${resourcePath}`;
  return {
    canonicalPath: `${category}.${tool}`,
    category,
    tool,
    source: "fallback"
  };
}
function parseExplicitCommand(operation) {
  const command = operation.xLunaCli?.command;
  if (!command) {
    return void 0;
  }
  let category;
  let tool;
  if (typeof command === "string") {
    const separator = command.indexOf(".");
    if (separator > 0 && separator < command.length - 1) {
      category = command.slice(0, separator);
      tool = command.slice(separator + 1);
    }
  } else {
    if (command.path) {
      const separator = command.path.indexOf(".");
      if (separator > 0 && separator < command.path.length - 1) {
        category = command.path.slice(0, separator);
        tool = command.path.slice(separator + 1);
      }
    }
    category ??= command.category;
    tool ??= command.tool;
  }
  const normalizedCategory = toKebabCase(category ?? "");
  const normalizedTool = toKebabCase(tool ?? "");
  if (!normalizedCategory || !normalizedTool) {
    return void 0;
  }
  return {
    canonicalPath: `${normalizedCategory}.${normalizedTool}`,
    category: normalizedCategory,
    tool: normalizedTool,
    source: "explicit"
  };
}
function fallbackRisk(method) {
  switch (method) {
    case "get":
    case "head":
    case "options":
      return "low";
    case "post":
      return "medium";
    case "put":
    case "patch":
      return "high";
    case "delete":
    case "trace":
      return "critical";
  }
}
function requiredScopes(operation) {
  const extensionScopes = operation.xLunaCli?.requiredScopes;
  const scopes = extensionScopes ?? operation.security.flatMap(
    (requirement) => Object.values(requirement).flatMap((values) => values)
  );
  return Object.freeze([...new Set(scopes)].sort());
}
function deepFreeze(value) {
  if (value === null || typeof value !== "object" || Object.isFrozen(value)) {
    return value;
  }
  Object.freeze(value);
  for (const nested of Object.values(value)) {
    deepFreeze(nested);
  }
  return value;
}
function catalogSort(left, right) {
  return left.command.canonicalPath.localeCompare(right.command.canonicalPath) || left.operationKey.localeCompare(right.operationKey);
}
function buildOperationCatalog(snapshots) {
  const entries = snapshots.map((operation) => {
    const normalizedPath = normalizeOpenApiPath(operation.path);
    const primaryTag = operation.tags[0] || "Api";
    const fallbackOperationId = createFallbackOperationId(
      primaryTag,
      operation.method,
      normalizedPath
    );
    const explicitOperationId = operation.operationId?.trim() || void 0;
    const explicitCommand = parseExplicitCommand(operation);
    const fallbackCommand = createFallbackCommand(
      primaryTag,
      operation.method,
      normalizedPath
    );
    const commandBase = explicitCommand ?? fallbackCommand;
    const command = {
      ...commandBase,
      classification: operation.xLunaCli?.classification ?? "unclassified",
      risk: operation.xLunaCli?.risk ?? fallbackRisk(operation.method),
      transport: operation.xLunaCli?.transport ?? "http",
      requiredScopes: requiredScopes(operation),
      hidden: operation.xLunaCli?.hidden ?? false,
      exclusionReason: operation.xLunaCli?.exclusionReason
    };
    return deepFreeze({
      ...operation,
      normalizedPath,
      path: normalizedPath,
      operationKey: createOperationKey(operation.method, normalizedPath),
      primaryTag,
      operationId: explicitOperationId ?? fallbackOperationId,
      operationIdSource: explicitOperationId ? "explicit" : "fallback",
      explicitOperationId,
      fallbackOperationId,
      command
    });
  });
  entries.sort(catalogSort);
  const uniqueOperationKeys = /* @__PURE__ */ new Set();
  const uniqueOperationIds = /* @__PURE__ */ new Set();
  const uniqueCommandPaths = /* @__PURE__ */ new Set();
  for (const entry of entries) {
    if (uniqueOperationKeys.has(entry.operationKey)) {
      throw new Error(`Duplicate OpenAPI operation key: ${entry.operationKey}`);
    }
    if (uniqueOperationIds.has(entry.operationId)) {
      throw new Error(`Duplicate OpenAPI operation ID: ${entry.operationId}`);
    }
    if (uniqueCommandPaths.has(entry.command.canonicalPath)) {
      throw new Error(
        `Duplicate Luna command path: ${entry.command.canonicalPath}`
      );
    }
    uniqueOperationKeys.add(entry.operationKey);
    uniqueOperationIds.add(entry.operationId);
    uniqueCommandPaths.add(entry.command.canonicalPath);
  }
  return deepFreeze(entries);
}
var OPERATION_CATALOG = buildOperationCatalog(
  OPENAPI_OPERATION_SNAPSHOTS
);
var OPERATIONS_BY_ID = new Map(
  OPERATION_CATALOG.map((operation) => [operation.operationId, operation])
);
var OPERATIONS_BY_KEY = new Map(
  OPERATION_CATALOG.map((operation) => [operation.operationKey, operation])
);
var OPERATIONS_BY_COMMAND = new Map(
  OPERATION_CATALOG.map((operation) => [
    operation.command.canonicalPath,
    operation
  ])
);
var catalogDigest = digestJson(OPERATION_CATALOG);
var explicitOperationIdCount = OPERATION_CATALOG.filter(
  ({ operationIdSource }) => operationIdSource === "explicit"
).length;
var explicitCommandCount = OPERATION_CATALOG.filter(
  ({ command }) => command.source === "explicit"
).length;
var OPERATION_CATALOG_METADATA = deepFreeze({
  catalogVersion: `${OPENAPI_SNAPSHOT_METADATA.apiVersion}+catalog.${catalogDigest.slice(7, 19)}`,
  apiVersion: OPENAPI_SNAPSHOT_METADATA.apiVersion,
  openapiVersion: OPENAPI_SNAPSHOT_METADATA.openapiVersion,
  openapiDigest: OPENAPI_SNAPSHOT_METADATA.sourceDigest,
  catalogDigest,
  operationCount: OPERATION_CATALOG.length,
  explicitOperationIdCount,
  fallbackOperationIdCount: OPERATION_CATALOG.length - explicitOperationIdCount,
  explicitCommandCount,
  fallbackCommandCount: OPERATION_CATALOG.length - explicitCommandCount
});
function findOperationById(operationId) {
  return OPERATIONS_BY_ID.get(operationId);
}
function findOperationByRoute(method, path3) {
  return OPERATIONS_BY_KEY.get(createOperationKey(method, path3));
}
function findOperationByCommand(commandPath) {
  return OPERATIONS_BY_COMMAND.get(commandPath);
}
function asArray(value) {
  if (value === void 0) {
    return [];
  }
  return Array.isArray(value) ? value : [value];
}
function includesAny(expected, actual) {
  const values = asArray(expected);
  return values.length === 0 || values.some((value) => actual.includes(value));
}
function searchText(operation) {
  return [
    operation.operationId,
    operation.operationKey,
    operation.command.canonicalPath,
    operation.summary,
    operation.description,
    ...operation.tags,
    ...operation.command.requiredScopes
  ].filter(Boolean).join(" ").toLowerCase();
}
function filterOperationCatalog(filter = {}) {
  const query = filter.query?.trim().toLowerCase();
  const methods = asArray(filter.method);
  const tags = asArray(filter.tag).map((tag) => tag.toLowerCase());
  const categories = asArray(filter.category).map(
    (category) => category.toLowerCase()
  );
  const classifications = asArray(
    filter.classification
  );
  const risks = asArray(filter.risk);
  const transports = asArray(filter.transport);
  const scopes = asArray(filter.scope);
  return OPERATION_CATALOG.filter((operation) => {
    if (!filter.includeDeprecated && operation.deprecated) {
      return false;
    }
    if (!filter.includeHidden && operation.command.hidden) {
      return false;
    }
    if (query && !searchText(operation).includes(query)) {
      return false;
    }
    if (methods.length > 0 && !methods.includes(operation.method)) {
      return false;
    }
    if (tags.length > 0 && !operation.tags.some((tag) => tags.includes(tag.toLowerCase()))) {
      return false;
    }
    if (categories.length > 0 && !categories.includes(operation.command.category.toLowerCase())) {
      return false;
    }
    if (!includesAny(classifications, [operation.command.classification])) {
      return false;
    }
    if (!includesAny(risks, [operation.command.risk])) {
      return false;
    }
    if (!includesAny(transports, [operation.command.transport])) {
      return false;
    }
    if (scopes.length > 0 && !scopes.every((scope) => operation.command.requiredScopes.includes(scope))) {
      return false;
    }
    if (filter.operationIdSource && filter.operationIdSource !== operation.operationIdSource) {
      return false;
    }
    return !(filter.commandSource && filter.commandSource !== operation.command.source);
  });
}
function pageOperationCatalog(filter = {}, options = {}) {
  const offset = Math.max(0, Math.trunc(options.offset ?? 0));
  const limit = Math.min(500, Math.max(1, Math.trunc(options.limit ?? 50)));
  const matching = filterOperationCatalog(filter);
  const items = matching.slice(offset, offset + limit);
  const nextOffset = offset + items.length < matching.length ? offset + items.length : void 0;
  return {
    items,
    total: matching.length,
    offset,
    limit,
    nextOffset
  };
}

// ../packages/api-contract/src/schema.ts
var CONTRACT_SCHEMA_NAME = "luna-api-contract";
var CONTRACT_SCHEMA_VERSION = 1;
var CONTRACT_SCHEMA_IDENTITY = Object.freeze({
  name: CONTRACT_SCHEMA_NAME,
  version: CONTRACT_SCHEMA_VERSION,
  apiVersion: OPERATION_CATALOG_METADATA.apiVersion,
  catalogVersion: OPERATION_CATALOG_METADATA.catalogVersion,
  openapiDigest: OPERATION_CATALOG_METADATA.openapiDigest,
  catalogDigest: OPERATION_CATALOG_METADATA.catalogDigest
});
function isSha256Digest(value) {
  return /^sha256:[a-f0-9]{64}$/.test(value);
}
function isCompatibleContractSchema(identity) {
  return identity.name === CONTRACT_SCHEMA_NAME && identity.version === CONTRACT_SCHEMA_VERSION;
}

// ../packages/api-contract/src/types.ts
var HTTP_METHODS = [
  "get",
  "put",
  "post",
  "delete",
  "options",
  "head",
  "patch",
  "trace"
];

// src/commands/api.ts
import process5 from "process";

// ../packages/api-client/src/body.ts
function isAvailable(name, value) {
  return typeof globalThis === "object" && name in globalThis && value !== void 0;
}
function isBlob(value) {
  return isAvailable("Blob", globalThis.Blob) && value instanceof Blob;
}
function isFormData(value) {
  return isAvailable("FormData", globalThis.FormData) && value instanceof FormData;
}
function isUrlSearchParams(value) {
  return value instanceof URLSearchParams;
}
function normalizeRequestBody(body, inputHeaders) {
  const headers = new Headers(inputHeaders);
  if (body === null || body === void 0) {
    return { body: null, headers };
  }
  if (typeof body === "string" || isBlob(body) || isFormData(body) || isUrlSearchParams(body) || body instanceof ArrayBuffer || ArrayBuffer.isView(body)) {
    return { body, headers };
  }
  if (!headers.has("content-type")) {
    headers.set("content-type", "application/json");
  }
  return { body: JSON.stringify(body), headers };
}

// ../packages/api-client/src/errors.ts
var HttpTransportError = class extends Error {
  code;
  retryable;
  constructor(code, message, retryable = false) {
    super(message);
    this.name = "HttpTransportError";
    this.code = code;
    this.retryable = retryable;
  }
};

// ../packages/api-client/src/fetch-transport.ts
var REDIRECT_STATUSES = /* @__PURE__ */ new Set([301, 302, 303, 307, 308]);
var CROSS_ORIGIN_SENSITIVE_HEADERS = [
  "authorization",
  "cookie",
  "proxy-authorization"
];
var CONTENT_HEADERS = ["content-length", "content-type"];
var FetchHttpResponse = class {
  constructor(response, cleanup, signal, timedOut) {
    this.response = response;
    this.cleanup = cleanup;
    this.signal = signal;
    this.timedOut = timedOut;
  }
  response;
  cleanup;
  signal;
  timedOut;
  responseBody;
  disposed = false;
  get body() {
    if (this.responseBody === void 0) {
      this.responseBody = this.response.body ? this.wrapBody(this.response.body) : null;
    }
    return this.responseBody;
  }
  get headers() {
    return this.response.headers;
  }
  get status() {
    return this.response.status;
  }
  get statusText() {
    return this.response.statusText;
  }
  get url() {
    return this.response.url;
  }
  arrayBuffer() {
    return this.consume(() => this.response.arrayBuffer());
  }
  dispose() {
    if (this.disposed) {
      return;
    }
    this.disposed = true;
    this.cleanup();
  }
  text() {
    return this.consume(() => this.response.text());
  }
  async consume(read) {
    try {
      return await read();
    } catch {
      throw this.readError();
    } finally {
      this.dispose();
    }
  }
  wrapBody(body) {
    const reader = body.getReader();
    return new ReadableStream({
      cancel: async (reason) => {
        try {
          await reader.cancel(reason);
        } finally {
          this.dispose();
        }
      },
      pull: async (controller) => {
        try {
          const chunk = await reader.read();
          if (chunk.done) {
            controller.close();
            this.dispose();
            return;
          }
          controller.enqueue(chunk.value);
        } catch {
          controller.error(this.readError());
          this.dispose();
        }
      }
    });
  }
  readError() {
    if (this.timedOut()) {
      return new HttpTransportError("request_timeout", "The request timed out", true);
    }
    if (this.signal.aborted) {
      return new HttpTransportError("request_aborted", "The request was aborted");
    }
    return new HttpTransportError("network_error", "The response body could not be read", true);
  }
};
function createSignal(externalSignal, timeoutMs) {
  const controller = new AbortController();
  let timeoutTriggered = false;
  const onAbort = () => controller.abort(externalSignal?.reason);
  externalSignal?.addEventListener("abort", onAbort, { once: true });
  let timer;
  if (timeoutMs !== void 0 && timeoutMs > 0) {
    timer = setTimeout(() => {
      timeoutTriggered = true;
      controller.abort(new Error("Request timed out"));
    }, timeoutMs);
  }
  if (externalSignal?.aborted) {
    onAbort();
  }
  return {
    cleanup: () => {
      externalSignal?.removeEventListener("abort", onAbort);
      if (timer !== void 0) {
        clearTimeout(timer);
      }
    },
    signal: controller.signal,
    timedOut: () => timeoutTriggered
  };
}
function redirectMethod(status, method) {
  if (status === 303 && method !== "HEAD") {
    return { method: "GET", removeBody: true };
  }
  if ((status === 301 || status === 302) && method === "POST") {
    return { method: "GET", removeBody: true };
  }
  return { method, removeBody: false };
}
var FetchHttpTransport = class {
  fetchImpl;
  maxRedirects;
  constructor(options = {}) {
    if (!options.fetch && typeof globalThis.fetch !== "function") {
      throw new TypeError("A Fetch implementation is required");
    }
    this.fetchImpl = options.fetch ?? globalThis.fetch.bind(globalThis);
    this.maxRedirects = Math.min(10, Math.max(0, options.maxRedirects ?? 5));
  }
  async send(request) {
    const scopedSignal = createSignal(request.signal, request.timeoutMs);
    try {
      return await this.sendWithRedirects(
        request,
        scopedSignal.signal,
        scopedSignal.cleanup,
        scopedSignal.timedOut
      );
    } catch (error) {
      if (error instanceof HttpTransportError) {
        scopedSignal.cleanup();
        throw error;
      }
      if (scopedSignal.timedOut()) {
        scopedSignal.cleanup();
        throw new HttpTransportError("request_timeout", "The request timed out", true);
      }
      if (request.signal?.aborted || scopedSignal.signal.aborted) {
        scopedSignal.cleanup();
        throw new HttpTransportError("request_aborted", "The request was aborted");
      }
      scopedSignal.cleanup();
      throw new HttpTransportError("network_error", "The request could not reach the server", true);
    }
  }
  async sendWithRedirects(request, signal, cleanup, timedOut) {
    let currentUrl = new URL(request.url);
    let currentMethod = request.method;
    let currentBody = request.body ?? null;
    const headers = new Headers(request.headers);
    for (let redirectCount = 0; ; redirectCount += 1) {
      const response = await this.fetchImpl(currentUrl, {
        body: currentMethod === "GET" || currentMethod === "HEAD" ? null : currentBody,
        headers,
        method: currentMethod,
        redirect: "manual",
        signal
      });
      if (!REDIRECT_STATUSES.has(response.status)) {
        return new FetchHttpResponse(response, cleanup, signal, timedOut);
      }
      const location = response.headers.get("location");
      if (!location) {
        return new FetchHttpResponse(response, cleanup, signal, timedOut);
      }
      if (redirectCount >= this.maxRedirects) {
        response.body?.cancel().catch(() => void 0);
        throw new HttpTransportError(
          "redirect_limit_exceeded",
          "The response exceeded the redirect limit"
        );
      }
      const nextUrl = new URL(location, currentUrl);
      if (nextUrl.username || nextUrl.password) {
        throw new HttpTransportError("unsafe_redirect", "The redirect URL contains credentials");
      }
      if (currentUrl.protocol === "https:" && nextUrl.protocol !== "https:") {
        throw new HttpTransportError(
          "unsafe_redirect",
          "Refusing to redirect from HTTPS to a less secure protocol"
        );
      }
      if (nextUrl.protocol !== "http:" && nextUrl.protocol !== "https:") {
        throw new HttpTransportError("unsafe_redirect", "The redirect protocol is not allowed");
      }
      if (nextUrl.origin !== currentUrl.origin) {
        for (const header of CROSS_ORIGIN_SENSITIVE_HEADERS) {
          headers.delete(header);
        }
      }
      const rewrite = redirectMethod(response.status, currentMethod);
      currentMethod = rewrite.method;
      if (rewrite.removeBody) {
        currentBody = null;
        for (const header of CONTENT_HEADERS) {
          headers.delete(header);
        }
      }
      response.body?.cancel().catch(() => void 0);
      currentUrl = nextUrl;
    }
  }
};

// ../packages/api-client/src/request-id.ts
function createRequestId() {
  const randomUUID3 = globalThis.crypto?.randomUUID;
  if (typeof randomUUID3 === "function") {
    return `req_${randomUUID3.call(globalThis.crypto)}`;
  }
  const random = Math.random().toString(36).slice(2);
  return `req_${Date.now().toString(36)}_${random}`;
}

// ../packages/api-client/src/retry.ts
var SAFE_METHODS = /* @__PURE__ */ new Set(["GET", "HEAD"]);
var RETRYABLE_STATUSES = /* @__PURE__ */ new Set([408, 425, 429, 502, 503, 504]);
function normalizeRetryPolicy(policy) {
  if (policy === false) {
    return false;
  }
  return {
    baseDelayMs: Math.max(0, policy?.baseDelayMs ?? 200),
    jitterRatio: Math.min(1, Math.max(0, policy?.jitterRatio ?? 0.2)),
    maxAttempts: Math.min(5, Math.max(1, policy?.maxAttempts ?? 3)),
    maxDelayMs: Math.max(0, policy?.maxDelayMs ?? 2e3)
  };
}
function isSafeMethod(method) {
  return SAFE_METHODS.has(method);
}
function isRetryableStatus(status) {
  return RETRYABLE_STATUSES.has(status);
}
function parseRetryAfter(value, now = Date.now()) {
  if (!value) {
    return void 0;
  }
  const seconds = Number(value);
  if (Number.isFinite(seconds) && seconds >= 0) {
    return Math.ceil(seconds * 1e3);
  }
  const timestamp = Date.parse(value);
  if (Number.isNaN(timestamp)) {
    return void 0;
  }
  return Math.max(0, timestamp - now);
}
function retryDelay(attempt, policy, retryAfterMs, random = Math.random) {
  if (retryAfterMs !== void 0) {
    return Math.min(policy.maxDelayMs, Math.max(0, retryAfterMs));
  }
  const exponential = Math.min(
    policy.maxDelayMs,
    policy.baseDelayMs * 2 ** Math.max(0, attempt - 1)
  );
  const jitter = exponential * policy.jitterRatio * (random() * 2 - 1);
  return Math.max(0, Math.round(exponential + jitter));
}
async function abortableSleep(ms, signal) {
  if (ms <= 0) {
    return;
  }
  if (signal?.aborted) {
    throw new HttpTransportError("request_aborted", "The request was aborted");
  }
  await new Promise((resolve, reject) => {
    let settled = false;
    const finish = (callback) => {
      if (settled) {
        return;
      }
      settled = true;
      signal?.removeEventListener("abort", onAbort);
      callback();
    };
    const timer = setTimeout(() => finish(resolve), ms);
    const onAbort = () => {
      clearTimeout(timer);
      finish(() => reject(
        new HttpTransportError("request_aborted", "The request was aborted")
      ));
    };
    signal?.addEventListener("abort", onAbort, { once: true });
    if (signal?.aborted) {
      onAbort();
    }
  });
}

// ../packages/api-client/src/response.ts
var SENSITIVE_KEY = /(?:authorization|cookie|credential|password|recovery.?code|secret|token)/i;
function isRecord(value) {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}
function redactSensitive(value, depth = 0) {
  if (depth > 8) {
    return "[REDACTED]";
  }
  if (Array.isArray(value)) {
    return value.map((item) => redactSensitive(item, depth + 1));
  }
  if (!isRecord(value)) {
    return value;
  }
  const result = {};
  for (const [key, item] of Object.entries(value)) {
    result[key] = SENSITIVE_KEY.test(key) ? "[REDACTED]" : redactSensitive(item, depth + 1);
  }
  return result;
}
async function parseJsonOrUndefined(response) {
  const text = await response.text();
  if (text.trim() === "") {
    return void 0;
  }
  return JSON.parse(text);
}
function responseRequestId(response, fallback) {
  return response.headers.get("x-request-id") ?? response.headers.get("x-correlation-id") ?? fallback;
}
async function parseSuccess(response, responseType, requestId) {
  let data;
  if (response.status === 204 || response.status === 205) {
    data = void 0;
    response.dispose();
  } else if (responseType === "text") {
    data = await response.text();
  } else if (responseType === "binary") {
    data = new Uint8Array(await response.arrayBuffer());
  } else if (responseType === "stream") {
    data = response;
  } else {
    try {
      data = await parseJsonOrUndefined(response);
    } catch {
      throw new HttpTransportError(
        "invalid_response",
        "The server returned invalid JSON"
      );
    }
  }
  return {
    data,
    headers: response.headers,
    ok: true,
    requestId: responseRequestId(response, requestId),
    status: response.status
  };
}
function normalizeFields(value) {
  if (!isRecord(value)) {
    return void 0;
  }
  const fields = {};
  for (const [key, reason] of Object.entries(value)) {
    if (typeof reason === "string") {
      fields[key] = reason;
    }
  }
  return Object.keys(fields).length > 0 ? fields : void 0;
}
async function parseFailure(response, fallbackRequestId) {
  let payload;
  try {
    payload = await parseJsonOrUndefined(response);
  } catch {
    payload = void 0;
  }
  const envelope = isRecord(payload) && isRecord(payload.error) ? payload.error : isRecord(payload) ? payload : {};
  const oauthCode = typeof envelope.error === "string" ? envelope.error : void 0;
  const requestId = typeof envelope.requestId === "string" ? envelope.requestId : responseRequestId(response, fallbackRequestId);
  const retryAfterMs = parseRetryAfter(response.headers.get("retry-after")) ?? (typeof envelope.retryAfter === "number" ? envelope.retryAfter * 1e3 : void 0);
  const code = typeof envelope.code === "string" ? envelope.code : oauthCode ?? `http_${response.status}`;
  const diagnostic = typeof envelope.message === "string" ? envelope.message : typeof envelope.detail === "string" ? envelope.detail : typeof envelope.error_description === "string" ? envelope.error_description : `The server returned HTTP ${response.status}`;
  const details = isRecord(envelope.details) ? redactSensitive(envelope.details) : {};
  const error = {
    code,
    details: isRecord(details) ? details : {},
    fields: normalizeFields(envelope.fields),
    message: diagnostic,
    purpose: typeof envelope.purpose === "string" ? envelope.purpose : void 0,
    requestId,
    retryAfterMs,
    retryable: response.status === 408 || response.status === 425 || response.status === 429 || response.status === 502 || response.status === 503 || response.status === 504,
    status: response.status
  };
  return {
    error,
    headers: response.headers,
    ok: false,
    requestId,
    status: response.status
  };
}
function transportFailure(error, requestId) {
  const transportError = error instanceof HttpTransportError ? error : new HttpTransportError("network_error", "The request could not reach the server", true);
  return {
    error: {
      code: transportError.code,
      details: {},
      message: transportError.message,
      requestId,
      retryable: transportError.retryable,
      status: 0
    },
    headers: new Headers(),
    ok: false,
    requestId,
    status: 0
  };
}

// ../node_modules/.pnpm/eventsource-parser@3.1.0/node_modules/eventsource-parser/dist/index.js
var ParseError = class extends Error {
  constructor(message, options) {
    super(message), this.name = "ParseError", this.type = options.type, this.field = options.field, this.value = options.value, this.line = options.line;
  }
};
var LF = 10;
var CR = 13;
var SPACE = 32;
function noop(_arg) {
}
function createParser(config) {
  if (typeof config == "function")
    throw new TypeError(
      "`config` must be an object, got a function instead. Did you mean `createParser({onEvent: fn})`?"
    );
  const { onEvent = noop, onError = noop, onRetry = noop, onComment, maxBufferSize } = config, pendingFragments = [];
  let pendingFragmentsLength = 0, isFirstChunk = true, id, data = "", dataLines = 0, eventType, terminated = false;
  function feed(chunk) {
    if (terminated)
      throw new Error(
        "Cannot feed parser: it was terminated after exceeding the configured max buffer size. Call `reset()` to resume parsing."
      );
    if (isFirstChunk && (isFirstChunk = false, chunk.charCodeAt(0) === 239 && chunk.charCodeAt(1) === 187 && chunk.charCodeAt(2) === 191 && (chunk = chunk.slice(3))), pendingFragments.length === 0) {
      const trailing2 = processLines(chunk);
      trailing2 !== "" && (pendingFragments.push(trailing2), pendingFragmentsLength = trailing2.length), checkBufferSize();
      return;
    }
    if (chunk.indexOf(`
`) === -1 && chunk.indexOf("\r") === -1) {
      pendingFragments.push(chunk), pendingFragmentsLength += chunk.length, checkBufferSize();
      return;
    }
    pendingFragments.push(chunk);
    const input = pendingFragments.join("");
    pendingFragments.length = 0, pendingFragmentsLength = 0;
    const trailing = processLines(input);
    trailing !== "" && (pendingFragments.push(trailing), pendingFragmentsLength = trailing.length), checkBufferSize();
  }
  function checkBufferSize() {
    maxBufferSize !== void 0 && (pendingFragmentsLength + data.length <= maxBufferSize || (terminated = true, pendingFragments.length = 0, pendingFragmentsLength = 0, id = void 0, data = "", dataLines = 0, eventType = void 0, onError(
      new ParseError(`Buffered data exceeded max buffer size of ${maxBufferSize} characters`, {
        type: "max-buffer-size-exceeded"
      })
    )));
  }
  function processLines(chunk) {
    let searchIndex = 0;
    if (chunk.indexOf("\r") === -1) {
      let lfIndex = chunk.indexOf(`
`, searchIndex);
      for (; lfIndex !== -1; ) {
        if (searchIndex === lfIndex) {
          dataLines > 0 && onEvent({ id, event: eventType, data }), id = void 0, data = "", dataLines = 0, eventType = void 0, searchIndex = lfIndex + 1, lfIndex = chunk.indexOf(`
`, searchIndex);
          continue;
        }
        const firstCharCode = chunk.charCodeAt(searchIndex);
        if (isDataPrefix(chunk, searchIndex, firstCharCode)) {
          const valueStart = chunk.charCodeAt(searchIndex + 5) === SPACE ? searchIndex + 6 : searchIndex + 5, value = chunk.slice(valueStart, lfIndex);
          if (dataLines === 0 && chunk.charCodeAt(lfIndex + 1) === LF) {
            onEvent({ id, event: eventType, data: value }), id = void 0, data = "", eventType = void 0, searchIndex = lfIndex + 2, lfIndex = chunk.indexOf(`
`, searchIndex);
            continue;
          }
          data = dataLines === 0 ? value : `${data}
${value}`, dataLines++;
        } else isEventPrefix(chunk, searchIndex, firstCharCode) ? eventType = chunk.slice(
          chunk.charCodeAt(searchIndex + 6) === SPACE ? searchIndex + 7 : searchIndex + 6,
          lfIndex
        ) || void 0 : parseLine(chunk, searchIndex, lfIndex);
        searchIndex = lfIndex + 1, lfIndex = chunk.indexOf(`
`, searchIndex);
      }
      return chunk.slice(searchIndex);
    }
    for (; searchIndex < chunk.length; ) {
      const crIndex = chunk.indexOf("\r", searchIndex), lfIndex = chunk.indexOf(`
`, searchIndex);
      let lineEnd = -1;
      if (crIndex !== -1 && lfIndex !== -1 ? lineEnd = crIndex < lfIndex ? crIndex : lfIndex : crIndex !== -1 ? crIndex === chunk.length - 1 ? lineEnd = -1 : lineEnd = crIndex : lfIndex !== -1 && (lineEnd = lfIndex), lineEnd === -1)
        break;
      parseLine(chunk, searchIndex, lineEnd), searchIndex = lineEnd + 1, chunk.charCodeAt(searchIndex - 1) === CR && chunk.charCodeAt(searchIndex) === LF && searchIndex++;
    }
    return chunk.slice(searchIndex);
  }
  function parseLine(chunk, start, end) {
    if (start === end) {
      dispatchEvent();
      return;
    }
    const firstCharCode = chunk.charCodeAt(start);
    if (isDataPrefix(chunk, start, firstCharCode)) {
      const valueStart = chunk.charCodeAt(start + 5) === SPACE ? start + 6 : start + 5, value2 = chunk.slice(valueStart, end);
      data = dataLines === 0 ? value2 : `${data}
${value2}`, dataLines++;
      return;
    }
    if (isEventPrefix(chunk, start, firstCharCode)) {
      eventType = chunk.slice(chunk.charCodeAt(start + 6) === SPACE ? start + 7 : start + 6, end) || void 0;
      return;
    }
    if (firstCharCode === 105 && chunk.charCodeAt(start + 1) === 100 && chunk.charCodeAt(start + 2) === 58) {
      const value2 = chunk.slice(chunk.charCodeAt(start + 3) === SPACE ? start + 4 : start + 3, end);
      id = value2.includes("\0") ? void 0 : value2;
      return;
    }
    if (firstCharCode === 58) {
      if (onComment) {
        const line2 = chunk.slice(start, end);
        onComment(line2.slice(chunk.charCodeAt(start + 1) === SPACE ? 2 : 1));
      }
      return;
    }
    const line = chunk.slice(start, end), fieldSeparatorIndex = line.indexOf(":");
    if (fieldSeparatorIndex === -1) {
      processField(line, "", line);
      return;
    }
    const field = line.slice(0, fieldSeparatorIndex), offset = line.charCodeAt(fieldSeparatorIndex + 1) === SPACE ? 2 : 1, value = line.slice(fieldSeparatorIndex + offset);
    processField(field, value, line);
  }
  function processField(field, value, line) {
    switch (field) {
      case "event":
        eventType = value || void 0;
        break;
      case "data":
        data = dataLines === 0 ? value : `${data}
${value}`, dataLines++;
        break;
      case "id":
        id = value.includes("\0") ? void 0 : value;
        break;
      case "retry":
        /^\d+$/.test(value) ? onRetry(parseInt(value, 10)) : onError(
          new ParseError(`Invalid \`retry\` value: "${value}"`, {
            type: "invalid-retry",
            value,
            line
          })
        );
        break;
      default:
        onError(
          new ParseError(
            `Unknown field "${field.length > 20 ? `${field.slice(0, 20)}\u2026` : field}"`,
            { type: "unknown-field", field, value, line }
          )
        );
        break;
    }
  }
  function dispatchEvent() {
    dataLines > 0 && onEvent({
      id,
      event: eventType,
      data
    }), id = void 0, data = "", dataLines = 0, eventType = void 0;
  }
  function reset(options = {}) {
    if (options.consume && pendingFragments.length > 0) {
      const incompleteLine = pendingFragments.join("");
      parseLine(incompleteLine, 0, incompleteLine.length);
    }
    isFirstChunk = true, id = void 0, data = "", dataLines = 0, eventType = void 0, pendingFragments.length = 0, pendingFragmentsLength = 0, terminated = false;
  }
  return { feed, reset };
}
function isDataPrefix(chunk, i, firstCharCode) {
  return firstCharCode === 100 && chunk.charCodeAt(i + 1) === 97 && chunk.charCodeAt(i + 2) === 116 && chunk.charCodeAt(i + 3) === 97 && chunk.charCodeAt(i + 4) === 58;
}
function isEventPrefix(chunk, i, firstCharCode) {
  return firstCharCode === 101 && chunk.charCodeAt(i + 1) === 118 && chunk.charCodeAt(i + 2) === 101 && chunk.charCodeAt(i + 3) === 110 && chunk.charCodeAt(i + 4) === 116 && chunk.charCodeAt(i + 5) === 58;
}

// ../packages/api-client/src/sse.ts
async function* parseSseStream(body, options = {}) {
  const decoder = new TextDecoder();
  const reader = body.getReader();
  const events = [];
  let parserError;
  const parser = createParser({
    onError(error) {
      parserError = new Error(`Invalid SSE stream: ${error.message}`);
    },
    onEvent(event) {
      events.push(event);
    }
  });
  const drain = function* () {
    while (events.length > 0) {
      const event = events.shift();
      if (!event) {
        continue;
      }
      let data = event.data;
      if (options.parseData) {
        data = options.parseData(event.data);
      }
      yield {
        data,
        event: event.event || void 0,
        id: event.id || void 0,
        rawData: event.data
      };
    }
  };
  try {
    while (true) {
      const chunk = await reader.read();
      if (chunk.done) {
        parser.feed(decoder.decode());
        break;
      }
      parser.feed(decoder.decode(chunk.value, { stream: true }));
      if (parserError) {
        throw parserError;
      }
      yield* drain();
    }
    if (parserError) {
      throw parserError;
    }
    yield* drain();
  } finally {
    await reader.cancel().catch(() => void 0);
    reader.releaseLock();
  }
}

// ../packages/api-client/src/url.ts
function serializeQueryPrimitive(value) {
  if (value instanceof Date) {
    return value.toISOString();
  }
  return String(value);
}
function normalizeBaseUrl(input) {
  const url = new URL(input);
  if (url.username || url.password) {
    throw new TypeError("baseUrl must not contain credentials");
  }
  if (url.protocol !== "http:" && url.protocol !== "https:") {
    throw new TypeError("baseUrl must use http or https");
  }
  url.hash = "";
  return url;
}
function resolveRequestUrl(baseUrl, path3, query) {
  const url = path3 instanceof URL ? new URL(path3) : new URL(path3, baseUrl);
  if (url.username || url.password) {
    throw new TypeError("request URL must not contain credentials");
  }
  if (url.protocol !== "http:" && url.protocol !== "https:") {
    throw new TypeError("request URL must use http or https");
  }
  url.hash = "";
  if (query instanceof URLSearchParams) {
    for (const [key, value] of query) {
      url.searchParams.append(key, value);
    }
    return url;
  }
  if (query) {
    for (const [key, value] of Object.entries(query)) {
      if (value === null || value === void 0) {
        continue;
      }
      const values = Array.isArray(value) ? value : [value];
      for (const item of values) {
        url.searchParams.append(key, serializeQueryPrimitive(item));
      }
    }
  }
  return url;
}
function sameOrigin(left, right) {
  return left.origin === right.origin;
}

// ../packages/api-client/src/client.ts
var DEFAULT_TIMEOUT_MS = 3e4;
function normalizeMethod(method) {
  return method ?? "GET";
}
function isValidBearerToken(token) {
  return token.length > 0 && !/[\r\n]/.test(token);
}
var LunaClient = class {
  allowCrossOriginRequests;
  baseUrl;
  requestIdFactory;
  retryPolicy;
  timeoutMs;
  tokenProvider;
  transport;
  constructor(options) {
    this.baseUrl = normalizeBaseUrl(options.baseUrl);
    this.allowCrossOriginRequests = options.allowCrossOriginRequests ?? false;
    this.requestIdFactory = options.requestIdFactory ?? createRequestId;
    this.retryPolicy = normalizeRetryPolicy(options.retry);
    this.timeoutMs = Math.max(1, options.timeoutMs ?? DEFAULT_TIMEOUT_MS);
    this.tokenProvider = options.tokenProvider;
    this.transport = options.transport ?? new FetchHttpTransport();
  }
  async request(options) {
    const requestId = options.requestId ?? this.requestIdFactory();
    let request;
    try {
      request = await this.prepareRequest(options, requestId);
    } catch (error) {
      const normalized = error instanceof HttpTransportError ? error : new HttpTransportError(
        "invalid_request",
        error instanceof Error ? error.message : "The request is invalid"
      );
      return transportFailure(normalized, requestId);
    }
    const attempts = this.retryPolicy && isSafeMethod(request.method) ? this.retryPolicy.maxAttempts : 1;
    for (let attempt = 1; attempt <= attempts; attempt += 1) {
      try {
        const response = await this.transport.send(request);
        if (attempt < attempts && this.retryPolicy && isRetryableStatus(response.status)) {
          const retryAfterMs = parseRetryAfter(response.headers.get("retry-after"));
          response.body?.cancel().catch(() => void 0);
          await abortableSleep(
            retryDelay(attempt, this.retryPolicy, retryAfterMs),
            options.signal
          );
          continue;
        }
        if (response.status >= 200 && response.status < 300) {
          return await parseSuccess(
            response,
            options.responseType ?? "json",
            requestId
          );
        }
        return await parseFailure(response, requestId);
      } catch (error) {
        const failure = transportFailure(error, requestId);
        if (attempt >= attempts || !this.retryPolicy || !failure.error.retryable) {
          return failure;
        }
        try {
          await abortableSleep(
            retryDelay(attempt, this.retryPolicy),
            options.signal
          );
        } catch (sleepError) {
          return transportFailure(sleepError, requestId);
        }
      }
    }
    return transportFailure(void 0, requestId);
  }
  async openSse(options) {
    const headers = new Headers(options.headers);
    if (!headers.has("accept")) {
      headers.set("accept", "text/event-stream");
    }
    const result = await this.request({
      ...options,
      headers,
      responseType: "stream"
    });
    if (!result.ok) {
      return result;
    }
    if (!result.data.body) {
      return {
        error: {
          code: "invalid_response",
          details: {},
          message: "The SSE response did not contain a body",
          requestId: result.requestId,
          retryable: false,
          status: result.status
        },
        headers: result.headers,
        ok: false,
        requestId: result.requestId,
        status: result.status
      };
    }
    return {
      ...result,
      data: parseSseStream(result.data.body, { parseData: options.parseData })
    };
  }
  async prepareRequest(options, requestId) {
    const method = normalizeMethod(options.method);
    const url = resolveRequestUrl(this.baseUrl, options.path, options.query);
    if (!this.allowCrossOriginRequests && !sameOrigin(url, this.baseUrl)) {
      throw new HttpTransportError(
        "invalid_request",
        "Cross-origin API requests are disabled"
      );
    }
    const normalized = normalizeRequestBody(options.body, options.headers);
    normalized.headers.set("accept", normalized.headers.get("accept") ?? "application/json");
    normalized.headers.set("x-request-id", requestId);
    if ((options.auth ?? true) && sameOrigin(url, this.baseUrl) && this.tokenProvider) {
      let token;
      try {
        token = await this.tokenProvider.getAccessToken();
      } catch {
        throw new HttpTransportError(
          "invalid_request",
          "The access token provider failed"
        );
      }
      if (token !== void 0) {
        if (!isValidBearerToken(token)) {
          throw new HttpTransportError("invalid_request", "The access token is invalid");
        }
        normalized.headers.set("authorization", `Bearer ${token}`);
      }
    }
    return {
      body: normalized.body,
      headers: normalized.headers,
      method,
      signal: options.signal,
      timeoutMs: options.timeoutMs ?? this.timeoutMs,
      url
    };
  }
};

// src/config/context.ts
import { createHash } from "crypto";

// src/commands/errors.ts
var CliCommandError = class extends Error {
  code;
  status;
  exitCode;
  retryable;
  details;
  constructor(code, message, options = {}) {
    super(message, { cause: options.cause });
    this.name = "CliCommandError";
    this.code = code;
    this.status = options.status ?? 400;
    this.exitCode = options.exitCode ?? exitCodeForStatus(this.status);
    this.retryable = options.retryable ?? false;
    this.details = options.details ?? {};
  }
};
function exitCodeForStatus(status) {
  if (status === 401)
    return 3;
  if (status === 403)
    return 4;
  if (status === 404)
    return 5;
  if (status === 409 || status === 412 || status === 422)
    return 6;
  if (status === 429)
    return 7;
  if (status >= 500)
    return 8;
  if (status >= 400)
    return 2;
  return 1;
}
function toCliCommandError(error) {
  if (error instanceof CliCommandError)
    return error;
  if (isErrorLike(error)) {
    const status = numberValue(error.status) ?? numberValue(error.statusCode) ?? 500;
    const code = stringValue(error.code) ?? "internal_error";
    return new CliCommandError(code, stringValue(error.message) ?? "Command failed.", {
      status,
      retryable: Boolean(error.retryable),
      details: recordValue(error.details),
      cause: error
    });
  }
  return new CliCommandError("internal_error", "Command failed.", {
    status: 500,
    cause: error
  });
}
function isErrorLike(value) {
  return typeof value === "object" && value !== null;
}
function numberValue(value) {
  return typeof value === "number" && Number.isFinite(value) ? value : void 0;
}
function stringValue(value) {
  return typeof value === "string" && value.length > 0 ? value : void 0;
}
function recordValue(value) {
  return isErrorLike(value) ? value : {};
}

// src/config/schema.ts
import { z } from "zod";
var OUTPUT_FORMATS = [
  "table",
  "json",
  "raw-json",
  "yaml",
  "jsonl",
  "name"
];
var userSnapshotSchema = z.object({
  id: z.string().min(1),
  name: z.string().min(1).optional()
}).passthrough();
var credentialBaseSchema = z.object({
  scopes: z.array(z.string().min(1)).default([]),
  user: userSnapshotSchema.optional(),
  expiresAt: z.iso.datetime().optional(),
  createdAt: z.iso.datetime().optional()
});
var oauthCredentialSchema = credentialBaseSchema.extend({
  type: z.literal("oauth"),
  accessToken: z.string().min(1),
  refreshToken: z.string().min(1).optional(),
  tokenType: z.string().min(1).optional()
}).passthrough();
var accessTokenCredentialSchema = credentialBaseSchema.extend({
  type: z.literal("access_token"),
  token: z.string().min(1)
}).passthrough();
var credentialSchema = z.discriminatedUnion("type", [
  oauthCredentialSchema,
  accessTokenCredentialSchema
]);
var instanceSchema = z.object({
  server: z.string().min(1),
  tls: z.object({
    caFile: z.string().default(""),
    insecureSkipVerify: z.boolean().default(false)
  }).passthrough().default({ caFile: "", insecureSkipVerify: false }),
  network: z.object({
    proxy: z.string().default(""),
    noProxy: z.string().default("")
  }).passthrough().default({ proxy: "", noProxy: "" })
}).passthrough();
var projectSnapshotSchema = z.object({
  id: z.string().min(1),
  name: z.string().min(1).optional(),
  identifier: z.string().min(1).optional()
}).passthrough();
var contextSchema = z.object({
  instance: z.string().min(1),
  credential: z.string().min(1).optional(),
  project: projectSnapshotSchema.nullish(),
  language: z.string().default(""),
  output: z.union([z.enum(OUTPUT_FORMATS), z.literal("")]).default("")
}).passthrough();
var configDocumentSchema = z.object({
  version: z.literal(1),
  currentContext: z.string().min(1).nullable().optional(),
  instances: z.record(z.string().min(1), instanceSchema),
  credentials: z.record(z.string().min(1), credentialSchema),
  contexts: z.record(z.string().min(1), contextSchema)
}).superRefine((document, context) => {
  if (document.currentContext && !Object.hasOwn(document.contexts, document.currentContext)) {
    context.addIssue({
      code: "custom",
      path: ["currentContext"],
      message: `Unknown current context "${document.currentContext}".`
    });
  }
  for (const [name, value] of Object.entries(document.contexts)) {
    if (!Object.hasOwn(document.instances, value.instance)) {
      context.addIssue({
        code: "custom",
        path: ["contexts", name, "instance"],
        message: `Context "${name}" references an unknown instance.`
      });
    }
    if (value.credential && !Object.hasOwn(document.credentials, value.credential)) {
      context.addIssue({
        code: "custom",
        path: ["contexts", name, "credential"],
        message: `Context "${name}" references an unknown credential.`
      });
    }
  }
});
function emptyConfigDocument() {
  return {
    version: 1,
    currentContext: null,
    instances: {},
    credentials: {},
    contexts: {}
  };
}
function parseConfigDocument(value) {
  return configDocumentSchema.parse(value);
}

// src/config/store.ts
import { randomUUID } from "crypto";
import { constants as fsConstants } from "fs";
import {
  chmod,
  lstat,
  mkdir,
  open,
  rename,
  rm
} from "fs/promises";
import path2 from "path";
import process3 from "process";

// src/config/paths.ts
import os from "os";
import path from "path";
import process2 from "process";
function resolveConfigPath(options = {}) {
  const env = options.env ?? process2.env;
  const explicitPath = options.configPath ?? env.LUNA_CONFIG;
  if (explicitPath?.trim()) {
    return path.resolve(expandHome(explicitPath.trim(), options.homeDir));
  }
  const home = options.homeDir ?? env.LUNA_HOME ?? os.homedir();
  return path.join(path.resolve(home), ".luna", "auth.json");
}
function expandHome(value, homeDir) {
  if (value === "~")
    return homeDir ?? os.homedir();
  if (value.startsWith("~/") || value.startsWith("~\\")) {
    return path.join(homeDir ?? os.homedir(), value.slice(2));
  }
  return value;
}

// src/config/store.ts
var FileConfigStore = class {
  path;
  #lockTimeoutMs;
  #lockRetryMs;
  #staleLockMs;
  #platform;
  #now;
  #randomId;
  constructor(options = {}) {
    this.path = resolveConfigPath(options);
    this.#lockTimeoutMs = options.lockTimeoutMs ?? 5e3;
    this.#lockRetryMs = options.lockRetryMs ?? 25;
    this.#staleLockMs = options.staleLockMs ?? 3e4;
    this.#platform = options.platform ?? process3.platform;
    this.#now = options.now ?? Date.now;
    this.#randomId = options.randomId ?? randomUUID;
  }
  async read() {
    await this.#prepareDirectory(false);
    await this.#assertSafeFile(this.path, 384);
    try {
      const content = await readFileWithoutFollowingLinks(this.path);
      return parseConfigDocument(JSON.parse(content));
    } catch (error) {
      if (isNodeError(error, "ENOENT"))
        return emptyConfigDocument();
      if (error instanceof SyntaxError) {
        throw new CliCommandError(
          "config_invalid_json",
          `Configuration file "${this.path}" is not valid JSON.`,
          { status: 422, cause: error }
        );
      }
      if (error instanceof CliCommandError)
        throw error;
      if (isZodError(error)) {
        throw new CliCommandError(
          "config_schema_invalid",
          `Configuration file "${this.path}" does not match the supported schema.`,
          { status: 422, details: { issues: error.issues }, cause: error }
        );
      }
      throw configIoError("read", this.path, error);
    }
  }
  async write(config) {
    await this.#withLock(async () => {
      await this.#writeUnlocked(parseConfigDocument(config));
    });
  }
  async update(mutator) {
    return this.#withLock(async () => {
      const current = await this.#readUnlocked();
      const working = structuredClone(current);
      const replacement = mutator(working);
      const next = parseConfigDocument(replacement ?? working);
      await this.#writeUnlocked(next);
      return next;
    });
  }
  async #readUnlocked() {
    await this.#assertSafeFile(this.path, 384);
    try {
      const content = await readFileWithoutFollowingLinks(this.path);
      return parseConfigDocument(JSON.parse(content));
    } catch (error) {
      if (isNodeError(error, "ENOENT"))
        return emptyConfigDocument();
      if (error instanceof SyntaxError) {
        throw new CliCommandError(
          "config_invalid_json",
          `Configuration file "${this.path}" is not valid JSON.`,
          { status: 422, cause: error }
        );
      }
      if (isZodError(error)) {
        throw new CliCommandError(
          "config_schema_invalid",
          `Configuration file "${this.path}" does not match the supported schema.`,
          { status: 422, details: { issues: error.issues }, cause: error }
        );
      }
      if (error instanceof CliCommandError)
        throw error;
      throw configIoError("read", this.path, error);
    }
  }
  async #writeUnlocked(config) {
    await this.#prepareDirectory(true);
    await this.#assertSafeFile(this.path, 384);
    if (this.#platform === "win32") {
      throw new CliCommandError(
        "secure_storage_unavailable",
        "Secure credential persistence requires a Windows DACL backend.",
        {
          status: 501,
          details: {
            path: this.path,
            remediation: "Use LUNA_TOKEN without store=true until the DACL backend is available."
          }
        }
      );
    }
    const directory = path2.dirname(this.path);
    const temporaryPath = path2.join(
      directory,
      `.${path2.basename(this.path)}.${process3.pid}.${this.#randomId()}.tmp`
    );
    const flags = fsConstants.O_CREAT | fsConstants.O_EXCL | fsConstants.O_WRONLY | (fsConstants.O_NOFOLLOW ?? 0);
    let handle;
    try {
      handle = await open(temporaryPath, flags, 384);
      await handle.writeFile(`${JSON.stringify(config, null, 2)}
`, "utf8");
      await handle.sync();
      await handle.close();
      handle = void 0;
      await rename(temporaryPath, this.path);
      await chmod(this.path, 384);
      await syncDirectory(directory);
    } catch (error) {
      await handle?.close().catch(() => void 0);
      await rm(temporaryPath, { force: true }).catch(() => void 0);
      throw configIoError("write", this.path, error);
    }
  }
  async #withLock(operation) {
    await this.#prepareDirectory(true);
    const lockPath = `${this.path}.lock`;
    const deadline = this.#now() + this.#lockTimeoutMs;
    let handle;
    while (!handle) {
      await this.#assertLockPathSafe(lockPath);
      try {
        const flags = fsConstants.O_CREAT | fsConstants.O_EXCL | fsConstants.O_WRONLY | (fsConstants.O_NOFOLLOW ?? 0);
        handle = await open(lockPath, flags, 384);
        await handle.writeFile(
          JSON.stringify({ pid: process3.pid, createdAt: new Date(this.#now()).toISOString() }),
          "utf8"
        );
        await handle.sync();
      } catch (error) {
        if (!isNodeError(error, "EEXIST")) {
          throw configIoError("lock", this.path, error);
        }
        if (await this.#removeStaleLock(lockPath))
          continue;
        if (this.#now() >= deadline) {
          throw new CliCommandError(
            "config_lock_timeout",
            `Timed out waiting for the configuration lock "${lockPath}".`,
            { status: 409, retryable: true }
          );
        }
        await delay(this.#lockRetryMs);
      }
    }
    try {
      return await operation();
    } finally {
      await handle.close().catch(() => void 0);
      await rm(lockPath, { force: true }).catch(() => void 0);
    }
  }
  async #prepareDirectory(create) {
    const directory = path2.dirname(this.path);
    if (create)
      await mkdir(directory, { recursive: true, mode: 448 });
    try {
      const stats = await lstat(directory);
      if (stats.isSymbolicLink() || !stats.isDirectory()) {
        throw unsafePathError(directory);
      }
      await this.#tightenPermissions(directory, stats.mode, stats.uid, 448);
    } catch (error) {
      if (!create && isNodeError(error, "ENOENT"))
        return;
      if (error instanceof CliCommandError)
        throw error;
      throw configIoError("inspect", directory, error);
    }
  }
  async #assertSafeFile(filePath, mode) {
    try {
      const stats = await lstat(filePath);
      if (stats.isSymbolicLink() || !stats.isFile())
        throw unsafePathError(filePath);
      if (this.#platform === "win32") {
        throw new CliCommandError(
          "secure_storage_unavailable",
          "Secure credential persistence requires a Windows DACL backend.",
          {
            status: 501,
            details: {
              path: filePath,
              remediation: "Use LUNA_TOKEN until the DACL backend is available."
            }
          }
        );
      }
      await this.#tightenPermissions(filePath, stats.mode, stats.uid, mode);
    } catch (error) {
      if (isNodeError(error, "ENOENT"))
        return;
      if (error instanceof CliCommandError)
        throw error;
      throw configIoError("inspect", filePath, error);
    }
  }
  async #assertLockPathSafe(lockPath) {
    try {
      const stats = await lstat(lockPath);
      if (stats.isSymbolicLink() || !stats.isFile())
        throw unsafePathError(lockPath);
      await this.#tightenPermissions(lockPath, stats.mode, stats.uid, 384);
    } catch (error) {
      if (isNodeError(error, "ENOENT"))
        return;
      if (error instanceof CliCommandError)
        throw error;
      throw configIoError("inspect", lockPath, error);
    }
  }
  async #removeStaleLock(lockPath) {
    try {
      const stats = await lstat(lockPath);
      if (stats.isSymbolicLink() || !stats.isFile())
        throw unsafePathError(lockPath);
      await this.#tightenPermissions(lockPath, stats.mode, stats.uid, 384);
      if (this.#now() - stats.mtimeMs <= this.#staleLockMs)
        return false;
      await rm(lockPath);
      return true;
    } catch (error) {
      if (isNodeError(error, "ENOENT"))
        return true;
      if (error instanceof CliCommandError)
        throw error;
      throw configIoError("inspect", lockPath, error);
    }
  }
  async #tightenPermissions(target, currentMode, owner, requiredMode) {
    if (this.#platform === "win32")
      return;
    const currentUser = typeof process3.getuid === "function" ? process3.getuid() : void 0;
    if (currentUser !== void 0 && owner !== currentUser) {
      throw new CliCommandError(
        "config_owner_mismatch",
        `Configuration path "${target}" is not owned by the current user.`,
        { status: 403 }
      );
    }
    if ((currentMode & 511) !== requiredMode) {
      try {
        await chmod(target, requiredMode);
      } catch (error) {
        throw new CliCommandError(
          "config_permissions_insecure",
          `Unable to restrict permissions for "${target}".`,
          { status: 403, cause: error }
        );
      }
    }
  }
};
async function updateConfig(store, mutator) {
  if ("update" in store && typeof store.update === "function") {
    return store.update(mutator);
  }
  const current = parseConfigDocument(await store.read());
  const working = structuredClone(current);
  const replacement = mutator(working);
  const next = parseConfigDocument(replacement ?? working);
  await store.write(next);
  return next;
}
function configIoError(operation, target, cause) {
  return new CliCommandError(
    "config_io_error",
    `Unable to ${operation} configuration path "${target}".`,
    { status: 500, cause }
  );
}
function unsafePathError(target) {
  return new CliCommandError(
    "config_path_unsafe",
    `Refusing to use unsafe configuration path "${target}".`,
    { status: 403 }
  );
}
function isNodeError(error, code) {
  return error instanceof Error && "code" in error && error.code === code;
}
function isZodError(error) {
  return typeof error === "object" && error !== null && "issues" in error && Array.isArray(error.issues);
}
async function syncDirectory(directory) {
  try {
    const handle = await open(directory, "r");
    await handle.sync();
    await handle.close();
  } catch (error) {
    if (isNodeError(error, "EINVAL") || isNodeError(error, "ENOTSUP") || isNodeError(error, "EISDIR")) {
      return;
    }
    throw error;
  }
}
async function readFileWithoutFollowingLinks(filePath) {
  const flags = fsConstants.O_RDONLY | (fsConstants.O_NOFOLLOW ?? 0);
  const handle = await open(filePath, flags);
  try {
    return await handle.readFile("utf8");
  } finally {
    await handle.close();
  }
}
function delay(milliseconds) {
  return new Promise((resolve) => setTimeout(resolve, milliseconds));
}

// src/config/context.ts
function upsertContext(config, input) {
  const existing = config.contexts[input.name];
  let instanceName = existing?.instance;
  let originChanged = false;
  if (input.server !== void 0) {
    const origin = normalizeServerOrigin(input.server);
    const previousOrigin = existing ? normalizeServerOrigin(config.instances[existing.instance].server) : void 0;
    instanceName = ensureInstance(config, origin);
    originChanged = previousOrigin !== void 0 && previousOrigin !== origin;
  }
  if (!instanceName) {
    throw new CliCommandError(
      "context_server_required",
      `Context "${input.name}" requires a server.`,
      { status: 422 }
    );
  }
  const credential = input.credential === null ? void 0 : input.credential ?? (originChanged ? void 0 : existing?.credential);
  if (credential && !Object.hasOwn(config.credentials, credential)) {
    throw new CliCommandError(
      "credential_not_found",
      `Credential "${credential}" does not exist.`,
      { status: 404 }
    );
  }
  return {
    ...existing,
    instance: instanceName,
    credential,
    project: input.project === void 0 ? originChanged ? null : existing?.project : input.project === null ? null : { ...input.project },
    language: input.language ?? existing?.language ?? "",
    output: input.output ?? existing?.output ?? ""
  };
}
function normalizeServerOrigin(server) {
  let url;
  try {
    url = new URL(server);
  } catch (error) {
    throw new CliCommandError(
      "server_url_invalid",
      `Server "${server}" is not a valid absolute URL.`,
      { status: 422, cause: error }
    );
  }
  if (!["http:", "https:"].includes(url.protocol)) {
    throw new CliCommandError(
      "server_url_invalid",
      "Server URL must use http or https.",
      { status: 422 }
    );
  }
  if (url.username || url.password || url.hash || url.search) {
    throw new CliCommandError(
      "server_url_invalid",
      "Server URL cannot contain credentials, query parameters, or a fragment.",
      { status: 422 }
    );
  }
  if (url.pathname !== "/" && url.pathname !== "") {
    throw new CliCommandError(
      "server_url_subpath_unsupported",
      "Server URL must not contain a path.",
      { status: 422 }
    );
  }
  return url.origin;
}
function ensureInstance(config, origin) {
  const existing = Object.entries(config.instances).find(
    ([, instance]) => normalizeServerOrigin(instance.server) === origin
  );
  if (existing)
    return existing[0];
  const base = `instance-${createHash("sha256").update(origin).digest("hex").slice(0, 12)}`;
  let candidate = base;
  let suffix = 2;
  while (Object.hasOwn(config.instances, candidate)) {
    candidate = `${base}-${suffix}`;
    suffix += 1;
  }
  config.instances[candidate] = defaultInstance(origin);
  return candidate;
}
function defaultInstance(server) {
  return {
    server,
    tls: { caFile: "", insecureSkipVerify: false },
    network: { proxy: "", noProxy: "" }
  };
}
function normalizeContextName(name) {
  const normalized = name.trim();
  if (!/^[A-Z0-9][\w.-]{0,62}$/i.test(normalized)) {
    throw new CliCommandError(
      "context_name_invalid",
      "Context names must be 1-63 characters using letters, numbers, dot, underscore, or hyphen.",
      { status: 422 }
    );
  }
  return normalized;
}
function pruneUnreferencedContextResources(config, references) {
  removeUnreferencedCredential(config, references.credential);
  if (references.instance)
    removeUnreferencedInstance(config, references.instance);
}
function removeUnreferencedCredential(config, credential) {
  if (credential && !Object.values(config.contexts).some((context) => context.credential === credential)) {
    delete config.credentials[credential];
  }
}
function removeUnreferencedInstance(config, instance) {
  if (!Object.values(config.contexts).some((context) => context.instance === instance)) {
    delete config.instances[instance];
  }
}

// src/config/resolve.ts
import process4 from "process";
function resolveRuntimeContext(rawConfig, options = {}) {
  const config = parseConfigDocument(rawConfig);
  const env = options.env ?? process4.env;
  const explicitContext = nonEmpty(options.context);
  const environmentContext = nonEmpty(env.LUNA_CONTEXT);
  const contextName = explicitContext ?? environmentContext ?? config.currentContext ?? void 0;
  const context = contextName ? config.contexts[contextName] : void 0;
  if (contextName && !context) {
    throw new CliCommandError(
      "context_not_found",
      `Context "${contextName}" does not exist.`,
      { status: 404 }
    );
  }
  const contextInstance = context ? config.instances[context.instance] : void 0;
  const explicitServer = nonEmpty(options.server);
  const environmentServer = nonEmpty(env.LUNA_SERVER);
  const serverOverride = explicitServer ?? environmentServer;
  const server = serverOverride ? normalizeServerOrigin(serverOverride) : contextInstance ? normalizeServerOrigin(contextInstance.server) : void 0;
  const sameOrigin2 = Boolean(
    server && contextInstance && server === normalizeServerOrigin(contextInstance.server)
  );
  const environmentToken = nonEmpty(env.LUNA_TOKEN);
  const credential = environmentToken ? {
    type: "access_token",
    token: environmentToken,
    scopes: []
  } : sameOrigin2 && context?.credential ? config.credentials[context.credential] : void 0;
  const explicitProject = nonEmpty(options.project);
  const environmentProject = nonEmpty(env.LUNA_PROJECT);
  const projectOverride = explicitProject ?? environmentProject;
  const project = projectOverride ? { id: projectOverride } : sameOrigin2 ? context?.project ?? void 0 : void 0;
  const explicitOutput = options.output === "" ? void 0 : options.output;
  const environmentOutput = outputValue(env.LUNA_OUTPUT);
  const contextOutput = context?.output || void 0;
  const output = explicitOutput ?? environmentOutput ?? contextOutput;
  const explicitLanguage = nonEmpty(options.language);
  const environmentLanguage = nonEmpty(env.LUNA_LANG);
  const contextLanguage = nonEmpty(context?.language);
  const language = explicitLanguage ?? environmentLanguage ?? contextLanguage;
  return {
    contextName,
    context,
    instance: sameOrigin2 ? contextInstance : server ? { server } : void 0,
    server,
    project,
    credential,
    output,
    language,
    sources: {
      context: explicitContext ? "argument" : environmentContext ? "environment" : contextName ? "context" : "none",
      server: explicitServer ? "argument" : environmentServer ? "environment" : contextInstance ? "context" : "none",
      project: explicitProject ? "argument" : environmentProject ? "environment" : project ? "context" : "none",
      credential: environmentToken ? "environment" : credential ? "context" : "none",
      output: explicitOutput ? "argument" : environmentOutput ? "environment" : contextOutput ? "context" : "default",
      language: explicitLanguage ? "argument" : environmentLanguage ? "environment" : contextLanguage ? "context" : "default"
    }
  };
}
function nonEmpty(value) {
  const normalized = value?.trim();
  return normalized || void 0;
}
function outputValue(value) {
  const normalized = nonEmpty(value);
  if (!normalized)
    return void 0;
  if (!OUTPUT_FORMATS.includes(normalized)) {
    throw new CliCommandError(
      "output_format_invalid",
      `Unsupported output format "${normalized}".`,
      { status: 422 }
    );
  }
  return normalized;
}

// src/commands/api.ts
var HTTP_METHODS2 = /* @__PURE__ */ new Set([
  "DELETE",
  "GET",
  "HEAD",
  "OPTIONS",
  "PATCH",
  "POST",
  "PUT"
]);
var QUERY_METHODS = /* @__PURE__ */ new Set(["DELETE", "GET", "HEAD", "OPTIONS"]);
var PROJECT_PARAMETER_NAMES = /* @__PURE__ */ new Set(["project", "projectId", "projectID"]);
var LunaApiAdapter = class {
  #config;
  #env;
  #clientFactory;
  constructor(options) {
    this.#config = options.config;
    this.#env = options.env ?? process5.env;
    this.#clientFactory = options.clientFactory ?? ((clientOptions) => new LunaClient(clientOptions));
  }
  async execute(request) {
    if (!request.metadata.path || !request.metadata.method) {
      throw new CliCommandError(
        "command_transport_invalid",
        `Command "${request.metadata.canonicalPath}" has no HTTP method or path.`,
        { status: 500, details: { command: request.metadata.canonicalPath } }
      );
    }
    assertSupportedTransport(request.metadata);
    const planned = planOpenApiRequest(request);
    if (request.globals.dryRun === "client") {
      return {
        schemaVersion: request.metadata.schemaVersion ?? "dry-run/v1",
        data: {
          dryRun: "client",
          operationId: request.operationId,
          request: planned
        }
      };
    }
    const result = await this.#send(planned, request.globals);
    const projectId = requestProjectId(request);
    return {
      schemaVersion: request.metadata.schemaVersion,
      data: result.data,
      meta: {
        requestId: result.requestId,
        status: result.status,
        ...projectId ? { projectId } : {}
      }
    };
  }
  async request(request) {
    const method = normalizeMethod2(request.method);
    const { body, ...remaining } = request.params;
    const planned = {
      method,
      path: validateDiagnosticPath(request.path),
      ...body !== void 0 ? { body } : {},
      ...Object.keys(remaining).length > 0 ? QUERY_METHODS.has(method) ? { query: toQueryInput(remaining) } : { body: mergeDiagnosticBody(body, remaining) } : {}
    };
    if (request.globals.dryRun === "client") {
      return {
        schemaVersion: "api.request/v1",
        data: { dryRun: "client", request: planned }
      };
    }
    const result = await this.#send(planned, request.globals);
    return {
      schemaVersion: "api.request/v1",
      data: result.data,
      meta: {
        requestId: result.requestId,
        status: result.status
      }
    };
  }
  async validateAccessToken(server, token, globals) {
    const client = this.#clientFactory({
      baseUrl: server,
      timeoutMs: globals.timeoutMs,
      tokenProvider: { getAccessToken: () => token }
    });
    const result = await client.request({
      method: "GET",
      path: "/api/v1/users/me",
      requestId: globals.requestId,
      timeoutMs: globals.timeoutMs
    });
    if (!result.ok)
      throw apiFailure(result.error);
    return asRecord(result.data);
  }
  async resolveProject(value, globals) {
    const client = await this.#client(globals);
    const result = await client.request({
      method: "GET",
      path: "/api/v1/projects",
      query: {
        page: 1,
        pageSize: 100,
        query: value
      },
      requestId: globals.requestId,
      timeoutMs: globals.timeoutMs
    });
    if (!result.ok)
      throw apiFailure(result.error);
    const candidates = listItems(result.data).filter((project2) => [project2.id, project2.identifier, project2.slug, project2.name].includes(value));
    if (candidates.length === 0) {
      throw new CliCommandError("project_not_found", `Project "${value}" was not found.`, {
        status: 404,
        details: { value }
      });
    }
    if (candidates.length > 1) {
      throw new CliCommandError("project_ambiguous", `Project "${value}" is ambiguous.`, {
        status: 409,
        details: {
          value,
          candidates: candidates.map((project2) => project2.id)
        }
      });
    }
    const project = candidates[0];
    return {
      id: project.id,
      ...project.name ? { name: project.name } : {},
      ...project.identifier ?? project.slug ? { identifier: project.identifier ?? project.slug } : {}
    };
  }
  async #send(planned, globals) {
    const client = await this.#client(globals);
    const headers = new Headers(planned.headers);
    if (globals.idempotencyKey)
      headers.set("idempotency-key", globals.idempotencyKey);
    const result = await client.request({
      method: planned.method,
      path: planned.path,
      query: planned.query,
      body: planned.body,
      headers,
      requestId: globals.requestId,
      timeoutMs: globals.timeoutMs
    });
    if (!result.ok)
      throw apiFailure(result.error);
    return result;
  }
  async #client(globals) {
    if (globals.insecureSkipTlsVerify) {
      throw new CliCommandError(
        "insecure_tls_unsupported",
        "This runtime cannot safely isolate insecure TLS verification for one request.",
        {
          status: 501,
          details: {
            remediation: "Configure a trusted CA for the selected instance."
          }
        }
      );
    }
    const config = await this.#config.read();
    const runtime = resolveRuntimeContext(config, {
      context: globals.context,
      server: globals.server,
      project: globals.project,
      output: globals.output,
      language: globals.lang,
      env: this.#env
    });
    if (!runtime.server) {
      throw new CliCommandError(
        "server_required",
        "No Luna server is configured. Use context set with --server or set LUNA_SERVER.",
        { status: 400, exitCode: 2 }
      );
    }
    const credential = runtime.credential;
    const token = credential?.type === "oauth" ? credential.accessToken : credential?.type === "access_token" ? credential.token : void 0;
    return this.#clientFactory({
      baseUrl: runtime.server,
      timeoutMs: globals.timeoutMs,
      tokenProvider: token ? { getAccessToken: () => token } : void 0
    });
  }
};
function requestProjectId(request) {
  for (const parameter2 of request.metadata.parameters) {
    if (!PROJECT_PARAMETER_NAMES.has(parameter2.name))
      continue;
    const value = request.params[parameter2.name];
    if (typeof value === "string" && value.trim())
      return value;
  }
  return void 0;
}
function planOpenApiRequest(request) {
  const method = normalizeMethod2(request.metadata.method);
  const pathParameters = {};
  const query = {};
  const headers = {};
  const bodyFields = {};
  const consumed = /* @__PURE__ */ new Set();
  let explicitBody;
  for (const parameter2 of request.metadata.parameters) {
    const value = parameterValue(parameter2, request.params, request.globals);
    if (value === void 0)
      continue;
    consumed.add(parameter2.name);
    switch (parameter2.location) {
      case "path":
        pathParameters[parameter2.name] = value;
        break;
      case "header":
        headers[parameter2.name] = headerValue(value, parameter2.name);
        break;
      case "cookie":
        throw new CliCommandError(
          "cookie_parameter_unsupported",
          `Cookie parameter "${parameter2.name}" is not supported by the CLI.`,
          { status: 501, details: { parameter: parameter2.name } }
        );
      case "body":
        if (parameter2.name === "body")
          explicitBody = value;
        else bodyFields[parameter2.name] = value;
        break;
      case "query":
        query[parameter2.name] = queryValue(value, parameter2.name);
        break;
      default:
        if (QUERY_METHODS.has(method))
          query[parameter2.name] = queryValue(value, parameter2.name);
        else bodyFields[parameter2.name] = value;
    }
  }
  for (const [name, value] of Object.entries(request.params)) {
    if (consumed.has(name))
      continue;
    if (name === "params" && isRecord2(value)) {
      for (const [nestedName, nestedValue] of Object.entries(value)) {
        if (QUERY_METHODS.has(method))
          query[nestedName] = queryValue(nestedValue, nestedName);
        else bodyFields[nestedName] = nestedValue;
      }
      continue;
    }
    if (QUERY_METHODS.has(method))
      query[name] = queryValue(value, name);
    else bodyFields[name] = value;
  }
  if (request.globals.dryRun === "server")
    query.dryRun = true;
  const path3 = interpolatePath(request.metadata.path, pathParameters);
  const body = explicitBody === void 0 ? Object.keys(bodyFields).length > 0 ? bodyFields : void 0 : Object.keys(bodyFields).length > 0 ? mergeBody(explicitBody, bodyFields) : explicitBody;
  return {
    method,
    path: path3,
    ...Object.keys(query).length > 0 ? { query } : {},
    ...Object.keys(headers).length > 0 ? { headers } : {},
    ...body !== void 0 ? { body } : {}
  };
}
function assertSupportedTransport(metadata) {
  if (metadata.transport !== "http") {
    throw new CliCommandError(
      "transport_not_implemented",
      `Transport "${metadata.transport}" is not implemented by the generic executor.`,
      {
        status: 501,
        details: {
          command: metadata.canonicalPath,
          transport: metadata.transport
        }
      }
    );
  }
}
function parameterValue(parameter2, params, globals) {
  const { name } = parameter2;
  if (Object.hasOwn(params, name))
    return params[name];
  if (parameter2.required && PROJECT_PARAMETER_NAMES.has(name))
    return globals.project;
  return void 0;
}
function interpolatePath(template, values) {
  const missing = [];
  const path3 = template.replace(/\{([^}]+)\}/g, (_match, name) => {
    const value = values[name];
    if (value === void 0 || value === null || value === "") {
      missing.push(name);
      return "";
    }
    return encodeURIComponent(String(value));
  });
  if (missing.length > 0) {
    throw new CliCommandError(
      "missing_path_parameter",
      `Missing path parameter${missing.length > 1 ? "s" : ""}: ${missing.join(", ")}.`,
      { status: 400, exitCode: 2, details: { parameters: missing } }
    );
  }
  return path3;
}
function normalizeMethod2(value) {
  const method = value?.toUpperCase();
  if (!method || !HTTP_METHODS2.has(method)) {
    throw new CliCommandError("http_method_invalid", `Unsupported HTTP method "${value ?? ""}".`, {
      status: 400,
      exitCode: 2,
      details: { method: value }
    });
  }
  return method;
}
function validateDiagnosticPath(value) {
  if (!value.startsWith("/api/") || value.startsWith("//") || value.includes("\\") || /^[a-z][a-z0-9+.-]*:/i.test(value)) {
    throw new CliCommandError(
      "diagnostic_path_invalid",
      "Diagnostic paths must be relative Luna API paths beginning with /api/.",
      { status: 400, exitCode: 2, details: { path: value } }
    );
  }
  return value;
}
function queryValue(value, name) {
  if (value === void 0 || value === null)
    return value;
  if (Array.isArray(value)) {
    if (value.every(isQueryPrimitive))
      return value;
  } else if (isQueryPrimitive(value)) {
    return value;
  }
  throw new CliCommandError(
    "query_parameter_invalid",
    `Query parameter "${name}" must be a primitive value or a primitive array.`,
    { status: 400, exitCode: 2, details: { parameter: name } }
  );
}
function toQueryInput(values) {
  return Object.fromEntries(
    Object.entries(values).map(([name, value]) => [name, queryValue(value, name)])
  );
}
function isQueryPrimitive(value) {
  return typeof value === "string" || typeof value === "number" || typeof value === "boolean" || value instanceof Date;
}
function headerValue(value, name) {
  if (typeof value === "string" || typeof value === "number" || typeof value === "boolean") {
    const rendered = String(value);
    if (!/[\r\n]/.test(rendered))
      return rendered;
  }
  throw new CliCommandError(
    "header_parameter_invalid",
    `Header parameter "${name}" must be a scalar value without line breaks.`,
    { status: 400, exitCode: 2, details: { parameter: name } }
  );
}
function mergeBody(explicitBody, fields) {
  if (!isRecord2(explicitBody)) {
    throw new CliCommandError(
      "request_body_conflict",
      "A non-object request body cannot be combined with body fields.",
      { status: 400, exitCode: 2 }
    );
  }
  return { ...explicitBody, ...fields };
}
function mergeDiagnosticBody(explicitBody, fields) {
  return explicitBody === void 0 ? fields : mergeBody(explicitBody, fields);
}
function apiFailure(error) {
  return new CliCommandError(error.code, error.message, {
    status: error.status,
    retryable: error.retryable,
    details: {
      ...error.details,
      requestId: error.requestId,
      ...error.purpose ? { purpose: error.purpose } : {}
    }
  });
}
function listItems(value) {
  const array = Array.isArray(value) ? value : isRecord2(value) && Array.isArray(value.items) ? value.items : [];
  return array.filter(isRecord2).flatMap((item) => {
    if (typeof item.id !== "string")
      return [];
    return [{
      id: item.id,
      ...typeof item.name === "string" ? { name: item.name } : {},
      ...typeof item.identifier === "string" ? { identifier: item.identifier } : {},
      ...typeof item.slug === "string" ? { slug: item.slug } : {}
    }];
  });
}
function asRecord(value) {
  if (typeof value !== "object" || value === null || Array.isArray(value)) {
    throw new CliCommandError(
      "api_response_invalid",
      "The Luna server returned an invalid current-user response.",
      { status: 502 }
    );
  }
  return value;
}
function isRecord2(value) {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

// src/commands/arguments.ts
import { Buffer as Buffer3 } from "buffer";
import { readFile as readFile2 } from "fs/promises";
import { extname as extname2 } from "path";
import process7 from "process";
import { confirm } from "@inquirer/prompts";
import { parse as parseYaml2 } from "yaml";

// src/errors/sanitize.ts
var ESCAPE_CHARACTER = String.fromCharCode(27);
var BELL_CHARACTER = String.fromCharCode(7);
var ANSI_SEQUENCE_PATTERN = new RegExp(
  [
    ESCAPE_CHARACTER,
    `\\][^${BELL_CHARACTER}${ESCAPE_CHARACTER}]*`,
    `(?:${BELL_CHARACTER}|${ESCAPE_CHARACTER}\\\\)`,
    "|",
    ESCAPE_CHARACTER,
    "(?:\\[[\\x30-\\x3F]*[\\x20-\\x2F]*[\\x40-\\x7E]|[\\x40-\\x5F])"
  ].join(""),
  "gu"
);
var BIDI_CONTROL_PATTERN = /[\u061C\u200E\u200F\u202A-\u202E\u2066-\u2069]/gu;
var SENSITIVE_KEY_PATTERN = /(?:authorization|cookie|credential|kubeconfig|otp|pass(?:word|phrase|wd)?|private[-_]?key|recovery[-_]?code|refresh[-_]?token|secret|session[-_]?id)$/iu;
var VALUE_SENSITIVE_KEY_PATTERN = /(?:access[-_]?token|id[-_]?token|token)$/iu;
var URL_PATTERN = /\bhttps?:\/\/[^\s"'<>]+/giu;
var AUTHORIZATION_PATTERN = /\b(Bearer|Basic)\s+[\w.~+/=-]+/giu;
var ASSIGNMENT_PATTERN = /\b(access[_-]?token|api[_-]?key|client[_-]?secret|password|refresh[_-]?token|secret|token)(?:[ \t]*=[ \t]*|[ \t]*:[ \t]+)([^\s,;]+)/giu;
var SENSITIVE_QUERY_KEYS = /* @__PURE__ */ new Set([
  "access_token",
  "api_key",
  "apikey",
  "authorization",
  "client_secret",
  "code",
  "id_token",
  "key",
  "password",
  "refresh_token",
  "secret",
  "sig",
  "signature",
  "token"
]);
var REDACTED_VALUE = "[REDACTED]";
function isSensitiveKey(key, additionalKeys) {
  const normalized = key.trim().toLocaleLowerCase();
  return Boolean(additionalKeys?.has(normalized)) || SENSITIVE_KEY_PATTERN.test(normalized);
}
function redactValue(value, options = {}) {
  const maxDepth = options.maxDepth ?? 12;
  const maxEntries = options.maxEntries ?? 1e4;
  const seen = /* @__PURE__ */ new WeakSet();
  let entries = 0;
  function visit2(current, depth, key) {
    entries += 1;
    if (entries > maxEntries)
      return "[TRUNCATED]";
    if (key && (typeof current !== "boolean" && isSensitiveKey(key, options.sensitiveKeys) || typeof current === "string" && VALUE_SENSITIVE_KEY_PATTERN.test(key))) {
      return REDACTED_VALUE;
    }
    if (typeof current === "string")
      return redactSensitiveText(current);
    if (typeof current !== "object" || current === null)
      return current;
    if (depth >= maxDepth)
      return "[MAX_DEPTH]";
    if (seen.has(current)) {
      return "[CIRCULAR]";
    }
    seen.add(current);
    if (Array.isArray(current)) {
      const result2 = current.map((item) => visit2(item, depth + 1));
      seen.delete(current);
      return result2;
    }
    const result = {};
    for (const [childKey, childValue] of Object.entries(current)) {
      result[childKey] = visit2(childValue, depth + 1, childKey);
    }
    seen.delete(current);
    return result;
  }
  return visit2(value, 0);
}
function redactSensitiveText(value) {
  return value.replace(AUTHORIZATION_PATTERN, "$1 [REDACTED]").replace(ASSIGNMENT_PATTERN, "$1=[REDACTED]").replace(URL_PATTERN, redactUrl);
}
function sanitizeTerminalText(value) {
  const withoutSequences = redactSensitiveText(value).replace(ANSI_SEQUENCE_PATTERN, "").replace(BIDI_CONTROL_PATTERN, "");
  return [...withoutSequences].filter((character) => !isUnsafeControlCharacter(character.codePointAt(0))).join("");
}
function escapeUnsafeJsonCharacters(value) {
  return value.replace(/[\u007F-\u009F\u061C\u200E\u200F\u2028\u2029\u202A-\u202E\u2066-\u2069]/gu, (character) => `\\u${character.codePointAt(0).toString(16).padStart(4, "0")}`);
}
function redactUrl(value) {
  try {
    const url = new URL(value);
    if (url.username)
      url.username = REDACTED_VALUE;
    if (url.password)
      url.password = REDACTED_VALUE;
    for (const key of [...url.searchParams.keys()]) {
      if (SENSITIVE_QUERY_KEYS.has(key.toLocaleLowerCase())) {
        url.searchParams.set(key, REDACTED_VALUE);
      }
    }
    return url.toString();
  } catch {
    return value;
  }
}
function isUnsafeControlCharacter(codePoint) {
  return codePoint >= 0 && codePoint <= 8 || codePoint === 11 || codePoint === 12 || codePoint >= 14 && codePoint <= 31 || codePoint >= 127 && codePoint <= 159;
}

// src/errors/luna-error.ts
var EXIT_CODE = Object.freeze({
  success: 0,
  unknown: 1,
  invalidInput: 2,
  unauthenticated: 3,
  forbidden: 4,
  notFound: 5,
  conflict: 6,
  retryLater: 7,
  serviceFailure: 8,
  partialSuccess: 9
});
var CODE_EXIT_MAP = {
  authentication_failed: EXIT_CODE.unauthenticated,
  token_expired: EXIT_CODE.unauthenticated,
  token_refresh_failed: EXIT_CODE.unauthenticated,
  unauthenticated: EXIT_CODE.unauthenticated,
  forbidden: EXIT_CODE.forbidden,
  mfa_required: EXIT_CODE.forbidden,
  permission_denied: EXIT_CODE.forbidden,
  not_found: EXIT_CODE.notFound,
  resource_not_found: EXIT_CODE.notFound,
  conflict: EXIT_CODE.conflict,
  precondition_failed: EXIT_CODE.conflict,
  resource_version_conflict: EXIT_CODE.conflict,
  state_conflict: EXIT_CODE.conflict,
  rate_limited: EXIT_CODE.retryLater,
  retry_later: EXIT_CODE.retryLater,
  dependency_unavailable: EXIT_CODE.serviceFailure,
  network_error: EXIT_CODE.serviceFailure,
  server_error: EXIT_CODE.serviceFailure,
  service_unavailable: EXIT_CODE.serviceFailure,
  partial_success: EXIT_CODE.partialSuccess
};
var LunaError = class extends Error {
  code;
  status;
  exitCode;
  retryable;
  requestId;
  retryAfter;
  purpose;
  fields;
  details;
  constructor(code, message, options = {}) {
    super(sanitizeTerminalText(message), { cause: options.cause });
    this.name = "LunaError";
    this.code = sanitizeTerminalText(code);
    this.status = options.status ?? 400;
    this.exitCode = options.exitCode ?? exitCodeFor(code, this.status);
    this.retryable = options.retryable ?? false;
    this.requestId = options.requestId ? sanitizeTerminalText(options.requestId) : void 0;
    this.retryAfter = options.retryAfter;
    this.purpose = options.purpose ? sanitizeTerminalText(options.purpose) : void 0;
    this.fields = options.fields ? redactValue(options.fields) : void 0;
    this.details = asRecord2(redactValue(options.details ?? {}));
  }
};
function invalidInput(code, message, options = {}) {
  return new LunaError(code, message, {
    ...options,
    status: 400,
    exitCode: EXIT_CODE.invalidInput
  });
}
function exitCodeFor(code, status) {
  const mapped = CODE_EXIT_MAP[code];
  if (mapped !== void 0)
    return mapped;
  if (status === 401)
    return EXIT_CODE.unauthenticated;
  if (status === 403)
    return EXIT_CODE.forbidden;
  if (status === 404)
    return EXIT_CODE.notFound;
  if (status === 409 || status === 412 || status === 422)
    return EXIT_CODE.conflict;
  if (status === 429)
    return EXIT_CODE.retryLater;
  if (status >= 500)
    return EXIT_CODE.serviceFailure;
  if (status >= 400)
    return EXIT_CODE.invalidInput;
  return EXIT_CODE.unknown;
}
function normalizeLunaError(error) {
  if (error instanceof LunaError)
    return error;
  if (isRecord3(error)) {
    const nested = isRecord3(error.error) ? error.error : error;
    const status = finiteNumber(nested.status) ?? finiteNumber(nested.statusCode) ?? 500;
    const code = nonEmptyString(nested.code) ?? "internal_error";
    const message = nonEmptyString(nested.message) ?? nonEmptyString(nested.detail) ?? "The command failed.";
    return new LunaError(code, message, {
      status,
      retryable: nested.retryable === true,
      requestId: nonEmptyString(nested.requestId),
      retryAfter: finiteNumber(nested.retryAfter),
      purpose: nonEmptyString(nested.purpose),
      fields: normalizeFields2(nested.fields),
      details: isRecord3(nested.details) ? nested.details : {},
      cause: error
    });
  }
  if (error instanceof Error) {
    return new LunaError("internal_error", error.message || "The command failed.", {
      status: 500,
      cause: error
    });
  }
  return new LunaError("internal_error", "The command failed.", {
    status: 500,
    cause: error
  });
}
function toErrorDocument(error) {
  const normalized = normalizeLunaError(error);
  const document = {
    error: {
      code: normalized.code,
      message: sanitizeTerminalText(normalized.message),
      status: normalized.status,
      retryable: normalized.retryable,
      details: asRecord2(redactValue(normalized.details)),
      ...normalized.requestId ? { requestId: normalized.requestId } : {},
      ...normalized.retryAfter !== void 0 ? { retryAfter: normalized.retryAfter } : {},
      ...normalized.purpose ? { purpose: normalized.purpose } : {},
      ...normalized.fields ? { fields: redactValue(normalized.fields) } : {}
    }
  };
  return document;
}
function normalizeFields2(value) {
  if (Array.isArray(value)) {
    return value.filter(isRecord3).map((field) => ({
      key: nonEmptyString(field.key) ?? "",
      code: nonEmptyString(field.code) ?? "invalid",
      ...field.expected !== void 0 ? { expected: field.expected } : {},
      ...field.actual !== void 0 ? { actual: field.actual } : {},
      ...nonEmptyString(field.message) ? { message: nonEmptyString(field.message) } : {}
    }));
  }
  if (!isRecord3(value))
    return void 0;
  return Object.fromEntries(
    Object.entries(value).filter((entry) => typeof entry[1] === "string")
  );
}
function isRecord3(value) {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}
function asRecord2(value) {
  return isRecord3(value) ? value : {};
}
function nonEmptyString(value) {
  return typeof value === "string" && value.length > 0 ? value : void 0;
}
function finiteNumber(value) {
  return typeof value === "number" && Number.isFinite(value) ? value : void 0;
}

// src/input/primitives.ts
var INTEGER_PATTERN = /^-?(?:0|[1-9]\d*)$/u;
var NUMBER_PATTERN = /^-?(?:(?:0|[1-9]\d*)(?:\.\d+)?|\.\d+)(?:e[+-]?\d+)?$/iu;
var DURATION_PATTERN = /^[+-]?(?:0|(?:\d+(?:\.\d+)?(?:ns|us|µs|μs|ms|[smh]))+)$/u;
var DURATION_PART_PATTERN = /(\d+(?:\.\d+)?)(ns|us|µs|μs|ms|[smh])/gu;
var RFC3339_PATTERN = /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/u;
var DURATION_MULTIPLIERS = {
  ns: 1e-6,
  us: 1e-3,
  \u00B5s: 1e-3,
  \u03BCs: 1e-3,
  ms: 1,
  s: 1e3,
  m: 6e4,
  h: 36e5
};
function parseBoolean(value, key = "value") {
  if (value === "true")
    return true;
  if (value === "false")
    return false;
  throw invalidInput("invalid_boolean", `${key} must be "true" or "false".`, {
    fields: [{ key, code: "boolean", expected: ["true", "false"], actual: value }]
  });
}
function parseInteger(value, key = "value") {
  if (!INTEGER_PATTERN.test(value)) {
    throw invalidInput("invalid_integer", `${key} must be a decimal integer.`, {
      fields: [{ key, code: "integer", actual: value }]
    });
  }
  const parsed = Number(value);
  if (!Number.isSafeInteger(parsed)) {
    throw invalidInput("integer_out_of_range", `${key} is outside the safe integer range.`, {
      fields: [{ key, code: "safe_integer", actual: value }]
    });
  }
  return parsed;
}
function parseNumberValue(value, key = "value") {
  if (!NUMBER_PATTERN.test(value)) {
    throw invalidInput("invalid_number", `${key} must be a finite decimal number.`, {
      fields: [{ key, code: "number", actual: value }]
    });
  }
  const parsed = Number(value);
  if (!Number.isFinite(parsed)) {
    throw invalidInput("invalid_number", `${key} must be a finite decimal number.`, {
      fields: [{ key, code: "finite", actual: value }]
    });
  }
  return parsed;
}
function parseDurationMilliseconds(value, key = "duration") {
  if (!DURATION_PATTERN.test(value)) {
    throw invalidInput("invalid_duration", `${key} must use Go duration syntax.`, {
      fields: [{ key, code: "duration", expected: "30s, 5m or 2h", actual: value }]
    });
  }
  if (value === "0" || value === "+0" || value === "-0")
    return 0;
  const sign = value.startsWith("-") ? -1 : 1;
  const unsigned = value.replace(/^[+-]/u, "");
  let consumed = "";
  let milliseconds = 0;
  for (const match of unsigned.matchAll(DURATION_PART_PATTERN)) {
    const [, quantity, unit] = match;
    consumed += match[0];
    milliseconds += Number(quantity) * DURATION_MULTIPLIERS[unit];
  }
  if (consumed !== unsigned || !Number.isFinite(milliseconds)) {
    throw invalidInput("invalid_duration", `${key} must use Go duration syntax.`, {
      fields: [{ key, code: "duration", actual: value }]
    });
  }
  return sign * milliseconds;
}
function parseRfc3339(value, key = "value") {
  if (!RFC3339_PATTERN.test(value) || Number.isNaN(Date.parse(value))) {
    throw invalidInput("invalid_date_time", `${key} must be an RFC 3339 timestamp.`, {
      fields: [{ key, code: "date-time", actual: value }]
    });
  }
  return value;
}
function parseInlinePrimitive(value, schema = {}, key = "value") {
  const types = schemaTypes(schema);
  if (value === "null" && types.includes("null"))
    return null;
  const type = preferredType(types);
  let parsed;
  switch (type) {
    case "boolean":
      parsed = parseBoolean(value, key);
      break;
    case "integer":
      parsed = parseInteger(value, key);
      break;
    case "number":
      parsed = parseNumberValue(value, key);
      break;
    case "object":
    case "array":
      parsed = parseJson(value, key);
      break;
    case "binary":
      throw invalidInput("binary_inline_forbidden", `${key} must be read from a file.`, {
        fields: [{ key, code: "value_source", expected: "file", actual: "inline" }]
      });
    default:
      parsed = parseStringFormat(value, schema, key);
  }
  validatePrimitiveConstraints(parsed, schema, key);
  return parsed;
}
function schemaTypes(schema) {
  if (Array.isArray(schema.type))
    return schema.type;
  return typeof schema.type === "string" ? [schema.type] : ["string"];
}
function preferredType(types) {
  return types.find((type) => type !== "null") ?? "null";
}
function parseStringFormat(value, schema, key) {
  if (schema.format === "duration") {
    parseDurationMilliseconds(value, key);
  } else if (schema.format === "date-time") {
    parseRfc3339(value, key);
  }
  return value;
}
function parseJson(value, key) {
  try {
    return JSON.parse(value);
  } catch (error) {
    throw invalidInput("invalid_json", `${key} must contain valid JSON.`, {
      fields: [{ key, code: "json" }],
      cause: error
    });
  }
}
function validatePrimitiveConstraints(value, schema, key) {
  if (schema.enum && !schema.enum.some((candidate) => Object.is(candidate, value))) {
    throw invalidInput("invalid_enum", `${key} is not an allowed value.`, {
      fields: [{ key, code: "enum", expected: schema.enum, actual: value }]
    });
  }
  if (typeof value === "string") {
    if (schema.minLength !== void 0 && value.length < schema.minLength) {
      throw invalidInput("string_too_short", `${key} is shorter than allowed.`, {
        fields: [{ key, code: "minLength", expected: schema.minLength, actual: value.length }]
      });
    }
    if (schema.maxLength !== void 0 && value.length > schema.maxLength) {
      throw invalidInput("string_too_long", `${key} is longer than allowed.`, {
        fields: [{ key, code: "maxLength", expected: schema.maxLength, actual: value.length }]
      });
    }
  }
  if (typeof value === "number") {
    if (schema.minimum !== void 0 && value < schema.minimum) {
      throw invalidInput("number_too_small", `${key} is smaller than allowed.`, {
        fields: [{ key, code: "minimum", expected: schema.minimum, actual: value }]
      });
    }
    if (schema.maximum !== void 0 && value > schema.maximum) {
      throw invalidInput("number_too_large", `${key} is larger than allowed.`, {
        fields: [{ key, code: "maximum", expected: schema.maximum, actual: value }]
      });
    }
  }
}

// src/input/globals.ts
var OUTPUT_FORMATS2 = Object.freeze([
  "table",
  "json",
  "raw-json",
  "yaml",
  "jsonl",
  "name"
]);
var GLOBAL_CONTROL_KEYS = Object.freeze([
  "context",
  "server",
  "project",
  "output",
  "lang",
  "color",
  "interactive",
  "yes",
  "quiet",
  "agent",
  "dryRun",
  "timeout",
  "debug",
  "requestId",
  "idempotencyKey",
  "insecureSkipTlsVerify"
]);
var GLOBAL_KEY_SET = new Set(GLOBAL_CONTROL_KEYS);

// src/input/sources.ts
import { Buffer as Buffer2 } from "buffer";
import { readFile, stat } from "fs/promises";
import { extname } from "path";
import process6 from "process";
import { parse as parseYaml } from "yaml";

// src/input/types.ts
var DEFAULT_INPUT_LIMITS = Object.freeze({
  inlineBytes: 4 * 1024,
  fileBytes: 10 * 1024 * 1024,
  stdinBytes: 10 * 1024 * 1024,
  paramsBytes: 1024 * 1024
});

// src/input/sources.ts
var NodeInputSourceReader = class {
  #stdin;
  constructor(stdin = process6.stdin) {
    this.#stdin = stdin;
  }
  async readFile(path3, maxBytes) {
    try {
      const info = await stat(path3);
      if (!info.isFile()) {
        throw invalidInput("input_not_file", `Input source "${path3}" is not a regular file.`);
      }
      if (info.size > maxBytes) {
        throw sourceTooLarge("file", maxBytes, info.size);
      }
      const content = await readFile(path3);
      if (content.byteLength > maxBytes) {
        throw sourceTooLarge("file", maxBytes, content.byteLength);
      }
      return content;
    } catch (error) {
      if (error instanceof LunaError)
        throw error;
      throw invalidInput("input_source_read_failed", `Unable to read input file "${path3}".`, {
        fields: [{ key: "file", code: "read_failed" }],
        cause: error
      });
    }
  }
  async readStdin(maxBytes) {
    try {
      const chunks = [];
      let total = 0;
      for await (const chunk of this.#stdin) {
        const bytes = Buffer2.from(chunk);
        total += bytes.byteLength;
        if (total > maxBytes)
          throw sourceTooLarge("stdin", maxBytes, total);
        chunks.push(bytes);
      }
      return Buffer2.concat(chunks);
    } catch (error) {
      if (error instanceof LunaError)
        throw error;
      throw invalidInput("input_source_read_failed", "Unable to read input from stdin.", {
        fields: [{ key: "stdin", code: "read_failed" }],
        cause: error
      });
    }
  }
};
function parseValueSource(rawValue) {
  if (rawValue.startsWith("@@")) {
    return { kind: "inline", inlineValue: rawValue.slice(1) };
  }
  if (rawValue === "@-")
    return { kind: "stdin" };
  if (rawValue.startsWith("@")) {
    const path3 = rawValue.slice(1);
    if (!path3)
      throw invalidInput("empty_input_path", "File input path cannot be empty.");
    return { kind: "file", path: path3 };
  }
  return { kind: "inline", inlineValue: rawValue };
}
function isSensitiveField(field) {
  return Boolean(
    field.sensitive || field.schema?.writeOnly || field.schema?.["x-sensitive"] || field.schema?.format === "password" || isSensitiveKey(field.name)
  );
}
function assertAllowedSource(field, source) {
  if (isSensitiveField(field) && source === "inline") {
    throw invalidInput(
      "sensitive_inline_forbidden",
      `${field.name} must be read from stdin, a protected file, or a secure prompt.`,
      { fields: [{ key: field.name, code: "value_source", expected: ["file", "stdin"], actual: source }] }
    );
  }
  const allowed = field.valueSources;
  if (allowed && !allowed.includes(source)) {
    throw invalidInput("input_source_not_allowed", `${field.name} does not allow ${source} input.`, {
      fields: [{ key: field.name, code: "value_source", expected: allowed, actual: source }]
    });
  }
  if (schemaTypes(field.schema ?? {}).includes("binary") && source !== "file") {
    throw invalidInput("binary_inline_forbidden", `${field.name} must be read from a file.`, {
      fields: [{ key: field.name, code: "value_source", expected: "file", actual: source }]
    });
  }
}
async function resolveFieldValue(rawValue, field, reader, limits = DEFAULT_INPUT_LIMITS) {
  const source = parseValueSource(rawValue);
  assertAllowedSource(field, source.kind);
  if (source.kind === "inline") {
    const value2 = source.inlineValue ?? "";
    const bytes2 = Buffer2.byteLength(value2);
    if (bytes2 > limits.inlineBytes || /[\r\n]/u.test(value2)) {
      throw invalidInput(
        "inline_value_too_large",
        `${field.name} must use file or stdin input when it contains newlines or exceeds ${limits.inlineBytes} bytes.`,
        { fields: [{ key: field.name, code: "maxBytes", expected: limits.inlineBytes, actual: bytes2 }] }
      );
    }
    return {
      value: parseInlinePrimitive(value2, field.schema, field.name),
      source: source.kind
    };
  }
  const bytes = source.kind === "stdin" ? await reader.readStdin(limits.stdinBytes) : await reader.readFile(source.path, limits.fileBytes);
  const value = parseBytes(bytes, field, source.path);
  return { value, source: source.kind };
}
function parseStructuredBytes(bytes, path3, key = "params") {
  const text = Buffer2.from(bytes).toString("utf8");
  try {
    const extension = path3 ? extname(path3).toLocaleLowerCase() : "";
    return extension === ".yaml" || extension === ".yml" ? parseYaml(text) : JSON.parse(text);
  } catch (error) {
    throw invalidInput("invalid_structured_input", `${key} must contain valid JSON or YAML.`, {
      fields: [{ key, code: "json_or_yaml" }],
      cause: error
    });
  }
}
function parseBytes(bytes, field, path3) {
  const types = schemaTypes(field.schema ?? {});
  if (types.includes("binary"))
    return bytes;
  if (types.includes("object") || types.includes("array")) {
    return parseStructuredBytes(bytes, path3, field.name);
  }
  const text = Buffer2.from(bytes).toString("utf8");
  return parseInlinePrimitive(text, field.schema, field.name);
}
function sourceTooLarge(source, limit, actual) {
  return invalidInput("input_source_too_large", `${source} input exceeds the configured byte limit.`, {
    fields: [{ key: source, code: "maxBytes", expected: limit, actual }]
  });
}

// src/input/validate.ts
function validateInputSchema(value, schema, rootKey = "params") {
  const errors = [];
  visit(value, schema, rootKey, errors);
  return errors;
}
function visit(value, schema, path3, errors) {
  const types = schemaTypes(schema);
  if (!matchesAnyType(value, types)) {
    errors.push({ key: path3, code: "type", expected: types, actual: jsonType(value) });
    return;
  }
  if (value === null)
    return;
  if (schema.enum && !schema.enum.some((candidate) => Object.is(candidate, value))) {
    errors.push({ key: path3, code: "enum", expected: schema.enum, actual: value });
  }
  if (typeof value === "string") {
    if (schema.minLength !== void 0 && value.length < schema.minLength) {
      errors.push({ key: path3, code: "minLength", expected: schema.minLength, actual: value.length });
    }
    if (schema.maxLength !== void 0 && value.length > schema.maxLength) {
      errors.push({ key: path3, code: "maxLength", expected: schema.maxLength, actual: value.length });
    }
    if (schema.format === "duration")
      capture(() => parseDurationMilliseconds(value, path3), path3, "duration", errors);
    if (schema.format === "date-time")
      capture(() => parseRfc3339(value, path3), path3, "date-time", errors);
  }
  if (typeof value === "number") {
    if (!Number.isFinite(value))
      errors.push({ key: path3, code: "finite", actual: value });
    if (types.includes("integer") && !Number.isSafeInteger(value)) {
      errors.push({ key: path3, code: "integer", actual: value });
    }
    if (schema.minimum !== void 0 && value < schema.minimum) {
      errors.push({ key: path3, code: "minimum", expected: schema.minimum, actual: value });
    }
    if (schema.maximum !== void 0 && value > schema.maximum) {
      errors.push({ key: path3, code: "maximum", expected: schema.maximum, actual: value });
    }
  }
  if (Array.isArray(value)) {
    if (schema.minItems !== void 0 && value.length < schema.minItems) {
      errors.push({ key: path3, code: "minItems", expected: schema.minItems, actual: value.length });
    }
    if (schema.maxItems !== void 0 && value.length > schema.maxItems) {
      errors.push({ key: path3, code: "maxItems", expected: schema.maxItems, actual: value.length });
    }
    if (schema.items)
      value.forEach((item, index) => visit(item, schema.items, `${path3}[${index}]`, errors));
  }
  if (isRecord4(value)) {
    for (const required of schema.required ?? []) {
      if (!(required in value))
        errors.push({ key: `${path3}.${required}`, code: "required" });
    }
    for (const [key, child] of Object.entries(value)) {
      const childSchema = schema.properties?.[key];
      if (childSchema) {
        visit(child, childSchema, `${path3}.${key}`, errors);
      } else if (schema.additionalProperties === false) {
        errors.push({ key: `${path3}.${key}`, code: "additionalProperties" });
      } else if (isRecord4(schema.additionalProperties)) {
        visit(child, schema.additionalProperties, `${path3}.${key}`, errors);
      }
    }
  }
}
function capture(action, key, code, errors) {
  try {
    action();
  } catch {
    errors.push({ key, code });
  }
}
function matchesAnyType(value, types) {
  return types.some((type) => {
    switch (type) {
      case "null":
        return value === null;
      case "array":
        return Array.isArray(value);
      case "object":
        return isRecord4(value);
      case "integer":
        return typeof value === "number" && Number.isSafeInteger(value);
      case "number":
        return typeof value === "number" && Number.isFinite(value);
      case "binary":
        return value instanceof Uint8Array;
      case "boolean":
        return typeof value === "boolean";
      case "string":
        return typeof value === "string";
      default:
        return false;
    }
  });
}
function jsonType(value) {
  if (value === null)
    return "null";
  if (Array.isArray(value))
    return "array";
  if (value instanceof Uint8Array)
    return "binary";
  return typeof value;
}
function isRecord4(value) {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

// src/input/parser.ts
var KEY_PATTERN = /^[A-Za-z][A-Za-z0-9]*$/u;
async function parseCommandInput(tokens, spec, options = {}) {
  const reader = options.reader ?? new NodeInputSourceReader();
  const limits = { ...DEFAULT_INPUT_LIMITS, ...options.limits };
  const fields = new Map(spec.fields.map((field) => [field.name, field]));
  const values = {};
  const errors = [];
  let stdinUsed = false;
  let paramsSeen = false;
  for (const token of tokens) {
    let pair;
    try {
      pair = splitKeyValue(token);
    } catch (error) {
      collectError(errors, error);
      continue;
    }
    if (pair.key === "params") {
      paramsSeen = true;
      try {
        if (Object.keys(values).length > 0) {
          throw invalidInput("params_conflict", "params cannot be combined with individual arguments.", {
            fields: [{ key: "params", code: "conflict" }]
          });
        }
        const source = parseValueSource(pair.value);
        if (source.kind === "inline") {
          throw invalidInput("params_inline_forbidden", "params must use @file or @- input.", {
            fields: [{ key: "params", code: "value_source", expected: ["file", "stdin"], actual: "inline" }]
          });
        }
        if (source.kind === "stdin" && stdinUsed) {
          throw invalidInput("stdin_already_used", "Only one argument may read from stdin.");
        }
        const bytes = source.kind === "stdin" ? await reader.readStdin(limits.paramsBytes) : await reader.readFile(source.path, limits.paramsBytes);
        stdinUsed ||= source.kind === "stdin";
        const params = parseStructuredBytes(bytes, source.path, "params");
        if (!isRecord5(params)) {
          throw invalidInput("params_must_be_object", "params must contain a JSON object.", {
            fields: [{ key: "params", code: "type", expected: "object", actual: jsonType2(params) }]
          });
        }
        if (spec.paramsSchema)
          errors.push(...validateInputSchema(params, spec.paramsSchema));
        values.params = params;
      } catch (error) {
        collectError(errors, error);
      }
      continue;
    }
    const field = fields.get(pair.key);
    if (!field) {
      errors.push({ key: pair.key, code: "unknown" });
      continue;
    }
    if (paramsSeen) {
      errors.push({ key: pair.key, code: "params_conflict" });
      continue;
    }
    if (pair.key in values && !field.repeated) {
      errors.push({ key: pair.key, code: "duplicate" });
      continue;
    }
    try {
      const source = parseValueSource(pair.value);
      if (source.kind === "stdin" && stdinUsed) {
        throw invalidInput("stdin_already_used", "Only one argument may read from stdin.", {
          fields: [{ key: pair.key, code: "stdin_already_used" }]
        });
      }
      const resolved = await resolveFieldValue(pair.value, field, reader, limits);
      stdinUsed ||= resolved.source === "stdin";
      appendValue(values, field, resolved.value);
    } catch (error) {
      collectError(errors, error, field);
    }
  }
  if (!paramsSeen) {
    for (const field of spec.fields) {
      if (field.required && !(field.name in values)) {
        errors.push({ key: field.name, code: "required" });
      }
    }
  }
  if (errors.length > 0) {
    throw new LunaError("invalid_arguments", "Input validation failed.", {
      status: 400,
      exitCode: 2,
      fields: deduplicateErrors(errors),
      details: spec.command ? { command: spec.command } : {}
    });
  }
  return { values, stdinUsed };
}
function splitKeyValue(token) {
  const separator = token.indexOf("=");
  if (separator <= 0) {
    throw invalidInput("invalid_argument_syntax", `Argument "${token}" must use key=value syntax.`);
  }
  const key = token.slice(0, separator);
  if (!KEY_PATTERN.test(key)) {
    throw invalidInput("invalid_argument_key", `Argument key "${key}" is invalid.`, {
      fields: [{ key, code: "pattern", expected: "[A-Za-z][A-Za-z0-9]*" }]
    });
  }
  return { key, value: token.slice(separator + 1) };
}
function appendValue(target, field, value) {
  if (!field.repeated) {
    target[field.name] = value;
    return;
  }
  const existing = target[field.name];
  target[field.name] = existing === void 0 ? [value] : [...existing, value];
}
function collectError(errors, error, field) {
  if (error instanceof LunaError && Array.isArray(error.fields)) {
    errors.push(...error.fields.map((item) => ({
      ...item,
      ...field && isSensitiveField(field) && item.actual !== void 0 ? { actual: REDACTED_VALUE } : {}
    })));
    return;
  }
  errors.push({
    key: field?.name ?? "arguments",
    code: error instanceof LunaError ? error.code : "invalid"
  });
}
function deduplicateErrors(errors) {
  const seen = /* @__PURE__ */ new Set();
  return errors.filter((error) => {
    const signature = `${error.key}:${error.code}`;
    if (seen.has(signature))
      return false;
    seen.add(signature);
    return true;
  });
}
function isRecord5(value) {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}
function jsonType2(value) {
  if (value === null)
    return "null";
  if (Array.isArray(value))
    return "array";
  return typeof value;
}

// src/commands/arguments.ts
var KEY_PATTERN2 = /^[A-Z][A-Z0-9]*$/i;
var INLINE_LIMIT_BYTES = 4 * 1024;
var OUTPUT_FORMATS3 = /* @__PURE__ */ new Set([
  "table",
  "json",
  "raw-json",
  "yaml",
  "jsonl",
  "name"
]);
var GLOBAL_KEYS = /* @__PURE__ */ new Set([
  "context",
  "server",
  "project",
  "output",
  "lang",
  "color",
  "interactive",
  "yes",
  "quiet",
  "agent",
  "dryRun",
  "timeout",
  "debug",
  "requestId",
  "idempotencyKey",
  "insecureSkipTlsVerify"
]);
var DefaultInputPort = class {
  async parse(tokens, metadata) {
    if (metadata.inputSchema?.additionalProperties !== true) {
      const parsed = await parseCommandInput(tokens, {
        command: metadata.canonicalPath,
        fields: metadata.parameters.map((parameter2) => ({
          name: parameter2.name,
          required: parameter2.required,
          repeated: parameter2.repeated,
          sensitive: parameter2.sensitive,
          valueSources: parameter2.valueSources,
          schema: parameter2.schema
        })),
        paramsSchema: metadata.inputSchema
      });
      return parsed.values;
    }
    return parseBusinessArguments(tokens, metadata);
  }
  confirm(message) {
    return confirm({ message, default: false });
  }
};
function splitGlobalTokens(tokens) {
  const canonicalGlobals = {};
  const businessTokens = [];
  const explicitGlobalKeys = /* @__PURE__ */ new Set();
  for (const token of tokens) {
    const { key, value } = splitKeyValue2(token);
    if (!GLOBAL_KEYS.has(key)) {
      businessTokens.push(token);
      continue;
    }
    if (key in canonicalGlobals) {
      throw invalidArguments(`Global argument "${key}" may only be provided once.`, { key });
    }
    canonicalGlobals[key] = value;
    explicitGlobalKeys.add(key);
  }
  return { businessTokens, canonicalGlobals, explicitGlobalKeys };
}
async function parseBusinessArguments(tokens, metadata) {
  const definitions = new Map(metadata.parameters.map((parameter2) => [parameter2.name, parameter2]));
  const values = {};
  let stdinUsed = false;
  for (const token of tokens) {
    const { key, value } = splitKeyValue2(token);
    const definition = definitions.get(key);
    if (!definition && metadata.inputSchema?.additionalProperties !== true) {
      throw invalidArguments(`Unknown argument "${key}" for ${metadata.canonicalPath}.`, {
        key,
        command: metadata.canonicalPath
      });
    }
    const parsed = await parseValueSource2(value, definition);
    if (parsed.stdin) {
      if (stdinUsed) {
        throw invalidArguments("Only one argument may read from stdin.", {
          code: "stdin_already_used"
        });
      }
      stdinUsed = true;
    }
    appendValue2(values, key, parsed.value, Boolean(definition?.repeated));
  }
  for (const parameter2 of metadata.parameters) {
    if (parameter2.required && !(parameter2.name in values)) {
      throw invalidArguments(`Missing required argument "${parameter2.name}".`, {
        key: parameter2.name,
        code: "required"
      });
    }
  }
  if ("params" in values && Object.keys(values).length > 1) {
    throw invalidArguments("params cannot be combined with individual business arguments.", {
      code: "params_conflict"
    });
  }
  return values;
}
function resolveGlobalOptions(canonical, flags, options) {
  assertNoConflicts(canonical, flags);
  const env = options.env;
  const agent = booleanOption(
    first(canonical.agent, flagBoolean(flags.agent), env.LUNA_AGENT),
    false,
    "agent"
  );
  const outputCandidate = first(
    canonical.output,
    flags.output,
    env.LUNA_OUTPUT,
    options.context?.output,
    options.isTTY ? "table" : "json"
  );
  const output = agent ? options.streaming ? "jsonl" : "json" : outputCandidate;
  if (!OUTPUT_FORMATS3.has(output)) {
    throw invalidArguments(`Unsupported output format "${output}".`, { output });
  }
  if (agent && output === "raw-json") {
    throw invalidArguments("Agent mode does not allow raw-json output.", {
      code: "agent_raw_json_forbidden"
    });
  }
  return {
    context: first(canonical.context, flags.context, env.LUNA_CONTEXT),
    server: first(canonical.server, flags.server, env.LUNA_SERVER),
    project: first(
      canonical.project,
      flags.project,
      env.LUNA_PROJECT,
      options.context?.project?.id
    ),
    output,
    lang: first(canonical.lang, flags.lang, env.LUNA_LANG, options.context?.language),
    color: agent ? false : booleanOption(first(canonical.color, flagBoolean(flags.color), env.LUNA_COLOR), true, "color"),
    interactive: agent ? false : booleanOption(
      first(canonical.interactive, flagBoolean(flags.interactive), env.LUNA_INTERACTIVE),
      options.isTTY,
      "interactive"
    ),
    yes: booleanOption(first(canonical.yes, flagBoolean(flags.yes), env.LUNA_YES), false, "yes"),
    quiet: agent ? true : booleanOption(first(canonical.quiet, flagBoolean(flags.quiet), env.LUNA_QUIET), false, "quiet"),
    agent,
    dryRun: dryRunOption(first(canonical.dryRun, flags.dryRun, env.LUNA_DRY_RUN)),
    timeoutMs: durationMilliseconds(
      first(canonical.timeout, flags.timeout, env.LUNA_TIMEOUT, "30s")
    ),
    debug: booleanOption(
      first(canonical.debug, flagBoolean(flags.debug), env.LUNA_DEBUG),
      false,
      "debug"
    ),
    requestId: first(canonical.requestId, flags.requestId, env.LUNA_REQUEST_ID),
    idempotencyKey: first(
      canonical.idempotencyKey,
      flags.idempotencyKey,
      env.LUNA_IDEMPOTENCY_KEY
    ),
    insecureSkipTlsVerify: booleanOption(
      first(
        canonical.insecureSkipTlsVerify,
        flagBoolean(flags.insecureSkipTlsVerify),
        env.LUNA_INSECURE_SKIP_TLS_VERIFY
      ),
      false,
      "insecureSkipTlsVerify"
    )
  };
}
function splitKeyValue2(token) {
  const separator = token.indexOf("=");
  if (separator <= 0) {
    throw invalidArguments(`Argument "${token}" must use key=value syntax.`, { token });
  }
  const key = token.slice(0, separator);
  if (!KEY_PATTERN2.test(key)) {
    throw invalidArguments(`Invalid argument key "${key}".`, { key });
  }
  return { key, value: token.slice(separator + 1) };
}
async function parseValueSource2(raw, definition) {
  if (raw.startsWith("@@")) {
    return { value: convertInline(raw.slice(1), definition), stdin: false };
  }
  if (raw === "@-") {
    if (definition?.valueSources && !definition.valueSources.includes("stdin")) {
      throw invalidArguments(`Argument "${definition.name}" does not accept stdin.`, {
        key: definition.name
      });
    }
    const content = await readStdin();
    return { value: parseFileContent(content, void 0, definition), stdin: true };
  }
  if (raw.startsWith("@")) {
    if (definition?.valueSources && !definition.valueSources.includes("file")) {
      throw invalidArguments(`Argument "${definition.name}" does not accept file input.`, {
        key: definition.name
      });
    }
    const path3 = raw.slice(1);
    if (!path3)
      throw invalidArguments("File input path cannot be empty.");
    const content = await readFile2(path3, "utf8");
    return { value: parseFileContent(content, extname2(path3), definition), stdin: false };
  }
  if (definition?.sensitive || definition?.valueSources && !definition.valueSources.includes("inline")) {
    throw invalidArguments(`Argument "${definition?.name ?? "value"}" cannot be provided inline.`, {
      key: definition?.name,
      code: "sensitive_inline_forbidden"
    });
  }
  if (Buffer3.byteLength(raw, "utf8") > INLINE_LIMIT_BYTES || raw.includes("\n")) {
    throw invalidArguments("Inline value is too large; use @file or @-.", {
      code: "inline_value_too_large",
      limitBytes: INLINE_LIMIT_BYTES
    });
  }
  return { value: convertInline(raw, definition), stdin: false };
}
function convertInline(raw, definition) {
  const schema = definition?.schema ?? {};
  const type = schema.type;
  if (raw === "null") {
    if (schema.nullable === true || Array.isArray(type) && type.includes("null"))
      return null;
    return raw;
  }
  if (type === "boolean") {
    if (raw === "true")
      return true;
    if (raw === "false")
      return false;
    throw invalidArguments(`Argument "${definition?.name}" must be true or false.`);
  }
  if (type === "integer" || type === "number") {
    const value = Number(raw);
    if (!Number.isFinite(value) || type === "integer" && !Number.isInteger(value)) {
      throw invalidArguments(`Argument "${definition?.name}" must be a valid ${type}.`);
    }
    return value;
  }
  if (type === "object" || type === "array") {
    try {
      return JSON.parse(raw);
    } catch (cause) {
      throw new CliCommandError("invalid_arguments", `Argument "${definition?.name}" must be JSON.`, {
        status: 400,
        exitCode: 2,
        cause
      });
    }
  }
  return raw;
}
function parseFileContent(content, extension, definition) {
  const type = definition?.schema?.type;
  if (extension === ".yaml" || extension === ".yml")
    return parseYaml2(content);
  if (extension === ".json" || type === "object" || type === "array") {
    try {
      return JSON.parse(content);
    } catch (cause) {
      throw new CliCommandError("invalid_arguments", "Input is not valid JSON.", {
        status: 400,
        exitCode: 2,
        cause
      });
    }
  }
  return content;
}
function appendValue2(target, key, value, repeated) {
  if (!(key in target)) {
    target[key] = repeated ? [value] : value;
    return;
  }
  if (!repeated) {
    throw invalidArguments(`Argument "${key}" may only be provided once.`, { key });
  }
  target[key].push(value);
}
async function readStdin() {
  const chunks = [];
  for await (const chunk of process7.stdin) {
    chunks.push(Buffer3.isBuffer(chunk) ? chunk : Buffer3.from(chunk));
  }
  return Buffer3.concat(chunks).toString("utf8");
}
function assertNoConflicts(canonical, flags) {
  const flagValues = {
    context: flags.context,
    server: flags.server,
    project: flags.project,
    output: flags.output,
    lang: flags.lang,
    color: flagBoolean(flags.color),
    interactive: flagBoolean(flags.interactive),
    yes: flagBoolean(flags.yes),
    quiet: flagBoolean(flags.quiet),
    agent: flagBoolean(flags.agent),
    dryRun: flags.dryRun,
    timeout: flags.timeout,
    debug: flagBoolean(flags.debug),
    requestId: flags.requestId,
    idempotencyKey: flags.idempotencyKey,
    insecureSkipTlsVerify: flagBoolean(flags.insecureSkipTlsVerify)
  };
  for (const [key, canonicalValue] of Object.entries(canonical)) {
    const flagValue = flagValues[key];
    if (flagValue !== void 0 && canonicalValue !== flagValue) {
      throw invalidArguments(`Conflicting values were provided for "${key}".`, {
        key,
        canonicalValue,
        flagValue
      });
    }
  }
}
function first(...values) {
  return values.find((value) => value !== void 0 && value !== "");
}
function flagBoolean(value) {
  return value === void 0 ? void 0 : String(value);
}
function booleanOption(value, fallback, key) {
  if (value === void 0)
    return fallback;
  if (value === "true" || value === "1")
    return true;
  if (value === "false" || value === "0")
    return false;
  throw invalidArguments(`Global option "${key}" must be true or false.`, { key, value });
}
function dryRunOption(value) {
  if (value === void 0)
    return void 0;
  if (value === "client" || value === "server")
    return value;
  throw invalidArguments("dryRun must be client or server.", { value });
}
function durationMilliseconds(value) {
  if (!value)
    return 3e4;
  const match = /^(\d+)(ms|[smh])$/.exec(value);
  if (!match)
    throw invalidArguments(`Invalid duration "${value}".`, { value });
  const amount = Number(match[1]);
  const multiplier = { ms: 1, s: 1e3, m: 6e4, h: 36e5 }[match[2]];
  const result = amount * multiplier;
  if (!Number.isSafeInteger(result) || result <= 0) {
    throw invalidArguments(`Invalid duration "${value}".`, { value });
  }
  return result;
}
function invalidArguments(message, details = {}) {
  return new CliCommandError("invalid_arguments", message, {
    status: 400,
    exitCode: 2,
    details
  });
}

// src/commands/completion.ts
function generateCompletion(shell, registry) {
  const categories = registry.categories();
  const toolsByCategory = Object.fromEntries(
    categories.map((category) => [
      category,
      registry.list({ category, includeHidden: false }).map((command) => command.metadata.tool)
    ])
  );
  switch (shell) {
    case "bash":
      return bashCompletion(categories, toolsByCategory);
    case "zsh":
      return zshCompletion(categories, toolsByCategory);
    case "fish":
      return fishCompletion(categories, toolsByCategory);
    case "powershell":
      return powershellCompletion(categories, toolsByCategory);
  }
}
function bashCompletion(categories, tools) {
  const cases = Object.entries(tools).map(([category, items]) => `      ${category}) COMPREPLY=( $(compgen -W "${items.join(" ")}" -- "$cur") ) ;;`).join("\n");
  return `# Luna CLI completion (generated)
_luna_completion() {
  local cur category
  cur="\${COMP_WORDS[COMP_CWORD]}"
  category="\${COMP_WORDS[1]}"
  if [[ \${COMP_CWORD} -eq 1 ]]; then
    COMPREPLY=( $(compgen -W "${categories.join(" ")}" -- "$cur") )
    return
  fi
  if [[ \${COMP_CWORD} -eq 2 ]]; then
    case "$category" in
${cases}
    esac
  fi
}
complete -F _luna_completion luna
`;
}
function zshCompletion(categories, tools) {
  const cases = Object.entries(tools).map(([category, items]) => `    ${category}) _values 'tool' ${items.join(" ")} ;;`).join("\n");
  return `#compdef luna
_luna() {
  local category
  if (( CURRENT == 2 )); then
    _values 'category' ${categories.join(" ")}
    return
  fi
  category="$words[2]"
  if (( CURRENT == 3 )); then
    case "$category" in
${cases}
    esac
  fi
}
_luna "$@"
`;
}
function fishCompletion(categories, tools) {
  const categoryLines = categories.map((category) => `complete -c luna -n '__fish_use_subcommand' -a '${category}'`).join("\n");
  const toolLines = Object.entries(tools).flatMap(
    ([category, items]) => items.map(
      (tool) => `complete -c luna -n '__fish_seen_subcommand_from ${category}' -a '${tool}'`
    )
  ).join("\n");
  return `# Luna CLI completion (generated)
${categoryLines}
${toolLines}
`;
}
function powershellCompletion(categories, tools) {
  const serialized = JSON.stringify(tools);
  return `# Luna CLI completion (generated)
Register-ArgumentCompleter -Native -CommandName luna -ScriptBlock {
  param($wordToComplete, $commandAst, $cursorPosition)
  $elements = @($commandAst.CommandElements | ForEach-Object { $_.Value })
  $tools = ConvertFrom-Json '${serialized}' -AsHashtable
  $candidates = if ($elements.Count -le 2) {
    @(${categories.map((value) => `'${value}'`).join(", ")})
  } elseif ($elements.Count -eq 3 -and $tools.ContainsKey($elements[1])) {
    @($tools[$elements[1]])
  } else {
    @()
  }
  $candidates | Where-Object { $_ -like "$wordToComplete*" } |
    ForEach-Object { [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_) }
}
`;
}

// src/commands/executor.ts
import process8 from "process";
import { Command, CommanderError, Option } from "commander";
var DEFAULT_GLOBALS = Object.freeze({
  output: "table",
  color: true,
  interactive: true,
  yes: false,
  quiet: false,
  agent: false,
  timeoutMs: 3e4,
  debug: false,
  insecureSkipTlsVerify: false
});
function createCliProgram(options) {
  const program = new Command().name(options.name ?? "luna").description(options.description ?? "Luna DevOps command-line client").version(options.ports.version ?? "0.1.0", "-V, --version").showHelpAfterError().allowExcessArguments(false).allowUnknownOption(false).addHelpCommand(false).helpOption("-h, --help", "Show command help");
  addGlobalOptions(program);
  for (const category of options.registry.categories()) {
    const categoryCommand = program.command(category).description(`${category} commands`).addHelpCommand(false);
    for (const categoryAlias of options.registry.categoryAliases(category)) {
      categoryCommand.alias(categoryAlias);
    }
    const commands = options.registry.list({ category, includeHidden: true });
    for (const registered of commands) {
      const tool = categoryCommand.command(registered.metadata.tool, { hidden: registered.metadata.hidden }).description(registered.metadata.summary ?? registered.metadata.canonicalPath).argument("[arguments...]", "Business arguments in key=value form").addHelpCommand(false).allowUnknownOption(false).action(async (tokens, _localOptions, command) => {
        const invokedPath = invokedCommandPath(command);
        await executeRegistered(
          registered,
          tokens ?? [],
          explicitCommanderOptions(command),
          options.ports,
          invokedPath
        );
      });
      for (const alias of registered.metadata.aliases) tool.alias(alias);
    }
  }
  return program;
}
async function runCli(program, argv = process8.argv, fallbackOutput) {
  const fallbackGlobals = inferFallbackGlobals(argv);
  const restoreCommanderOutput = suppressCommanderOutput(
    program,
    isMachineOutput(fallbackGlobals)
  );
  for (const command of commandTree(program))
    command.exitOverride();
  try {
    await program.parseAsync([...argv], { from: "node" });
    return { exitCode: 0 };
  } catch (error) {
    if (error instanceof CommanderError && isExpectedCommanderExit(error)) {
      return { exitCode: 0 };
    }
    const normalized = commanderFailure(error);
    await fallbackOutput?.writeError(normalized, fallbackGlobals);
    return { exitCode: normalized.exitCode, error: normalized };
  } finally {
    restoreCommanderOutput();
  }
}
async function executeRegistered(registered, tokens, flagOptions, ports, invokedPath) {
  const parsed = splitGlobalTokens(tokens);
  const config = await ports.config.read();
  const selectedContextName = parsed.canonicalGlobals.context ?? flagOptions.context ?? ports.env?.LUNA_CONTEXT ?? config.currentContext ?? void 0;
  const context = selectedContextName ? config.contexts[selectedContextName] : void 0;
  const globals = resolveGlobalOptions(parsed.canonicalGlobals, flagOptions, {
    env: ports.env ?? process8.env,
    context,
    isTTY: ports.isTTY ?? Boolean(process8.stdout.isTTY),
    streaming: registered.metadata.streaming ?? false
  });
  const inputMetadata = metadataWithResolvedProjectRequirement(registered, globals);
  const parsedParams = await ports.input.parse(parsed.businessTokens, inputMetadata);
  const params = resolveProjectParameters(parsedParams, registered, globals);
  enforceExecutionScope(
    registered,
    invokedPath,
    globals,
    parsed.explicitGlobalKeys,
    parsedParams,
    params
  );
  await enforceRiskPolicy(registered, invokedPath, globals, ports);
  const invocation = {
    metadata: registered.metadata,
    params,
    globals,
    explicitGlobalKeys: parsed.explicitGlobalKeys,
    canonicalGlobalValues: parsed.canonicalGlobals
  };
  const result = normalizeResult(
    await registered.handler(invocation, ports),
    registered.metadata.schemaVersion
  );
  await ports.output.writeSuccess(registered.metadata, result, globals);
}
function metadataWithResolvedProjectRequirement(registered, globals) {
  if (!globals.project)
    return registered.metadata;
  const parameters = registered.metadata.parameters.map(
    (parameter2) => parameter2.required && isProjectParameter(parameter2.name) ? { ...parameter2, required: false } : parameter2
  );
  return { ...registered.metadata, parameters };
}
function resolveProjectParameters(parsedParams, registered, globals) {
  const requiredProjectParameters = registered.metadata.parameters.filter((parameter2) => parameter2.required && isProjectParameter(parameter2.name));
  if (requiredProjectParameters.length === 0)
    return parsedParams;
  const params = { ...parsedParams };
  const structured = isRecord6(params.params) ? { ...params.params } : void 0;
  for (const parameter2 of requiredProjectParameters) {
    const explicitValue = params[parameter2.name] ?? structured?.[parameter2.name];
    const value = explicitValue ?? globals.project;
    if (value === void 0 || value === null || value === "") {
      throw new CliCommandError(
        "invalid_arguments",
        "Input validation failed.",
        {
          status: 400,
          exitCode: 2,
          details: {
            command: registered.metadata.canonicalPath,
            fields: [{ key: parameter2.name, code: "required" }]
          }
        }
      );
    }
    params[parameter2.name] = value;
    if (structured)
      delete structured[parameter2.name];
  }
  if (structured)
    params.params = structured;
  return params;
}
function isProjectParameter(name) {
  return name === "project" || name === "projectId" || name === "projectID";
}
function enforceExecutionScope(registered, requestedPath, globals, explicitGlobalKeys, explicitParams, params) {
  if (globals.agent && requestedPath !== registered.metadata.canonicalPath) {
    throw new CliCommandError(
      "agent_alias_forbidden",
      `Agent mode requires the canonical command "${registered.metadata.canonicalPath}".`,
      {
        status: 400,
        exitCode: 2,
        details: {
          command: registered.metadata.canonicalPath,
          invokedAs: requestedPath
        }
      }
    );
  }
  if (globals.agent && !registered.metadata.agentAllowed) {
    throw new CliCommandError(
      "agent_command_forbidden",
      `Command "${requestedPath}" is not available in agent mode.`,
      { status: 403, details: { command: requestedPath } }
    );
  }
  if (registered.metadata.projectContext === "required" && !hasProjectSelection(globals, params)) {
    throw new CliCommandError(
      "project_required",
      `Command "${requestedPath}" requires a project.`,
      { status: 400, exitCode: 2, details: { command: requestedPath } }
    );
  }
  if (registered.metadata.projectContext === "none" && explicitGlobalKeys.has("project")) {
    throw new CliCommandError(
      "project_not_supported",
      `Command "${requestedPath}" does not accept a project context.`,
      { status: 400, exitCode: 2, details: { command: requestedPath } }
    );
  }
  if (globals.agent && registered.metadata.projectContext === "required" && registered.metadata.risk !== "low" && !hasExplicitProjectSelection(explicitGlobalKeys, explicitParams)) {
    throw new CliCommandError(
      "explicit_project_required",
      "Agent mode requires an explicit project=<id> for project-scoped mutations.",
      { status: 400, exitCode: 2, details: { command: requestedPath } }
    );
  }
  if (globals.agent && globals.interactive) {
    throw new CliCommandError(
      "agent_interactive_forbidden",
      "Agent mode cannot enable interactive input.",
      { status: 400, exitCode: 2 }
    );
  }
}
function hasProjectSelection(globals, params) {
  return Boolean(globals.project) || projectValue(params) !== void 0;
}
function hasExplicitProjectSelection(explicitGlobalKeys, params) {
  return explicitGlobalKeys.has("project") || projectValue(params) !== void 0;
}
function projectValue(params) {
  const structured = isRecord6(params.params) ? params.params : void 0;
  for (const name of ["project", "projectId", "projectID"]) {
    const value = params[name] ?? structured?.[name];
    if (value !== void 0 && value !== null && value !== "")
      return value;
  }
  return void 0;
}
async function enforceRiskPolicy(registered, requestedPath, globals, ports) {
  const risk = registered.metadata.risk;
  if (risk === "low" || globals.dryRun)
    return;
  if (registered.metadata.source !== "local" && (risk === "high" || risk === "critical")) {
    throw new CliCommandError(
      "server_plan_required",
      `Command "${requestedPath}" requires a server-issued execution plan before it can run.`,
      {
        status: 412,
        exitCode: 6,
        details: {
          command: registered.metadata.canonicalPath,
          risk,
          requirement: "server_plan"
        }
      }
    );
  }
  if (registered.metadata.source === "local" && risk === "medium")
    return;
  if (globals.yes)
    return;
  if (!globals.interactive || !ports.input.confirm) {
    throw new CliCommandError(
      "confirmation_required",
      `Command "${requestedPath}" requires confirmation. Re-run interactively or pass yes=true.`,
      {
        status: 412,
        exitCode: 6,
        details: {
          command: registered.metadata.canonicalPath,
          risk
        }
      }
    );
  }
  const prompt = ports.translate?.(
    "confirm.execute",
    `Run ${registered.metadata.canonicalPath}?`,
    globals.lang
  ) ?? `Run ${registered.metadata.canonicalPath}?`;
  if (!await ports.input.confirm(prompt)) {
    throw new CliCommandError(
      "operation_cancelled",
      "Operation cancelled.",
      {
        status: 409,
        exitCode: 6,
        details: { command: registered.metadata.canonicalPath }
      }
    );
  }
}
function normalizeResult(value, schemaVersion) {
  if (isRecord6(value) && "data" in value && ("schemaVersion" in value || "meta" in value)) {
    return value;
  }
  return { data: value, schemaVersion };
}
function addGlobalOptions(program) {
  program.option("--context <name>", "Select a saved context").option("--server <url>", "Override the Luna server origin").option("--project <id>", "Select a project for this command").addOption(new Option("-o, --output <format>", "Output format").choices(["table", "json", "raw-json", "yaml", "jsonl", "name"])).option("--lang <locale>", "Output language").option("--no-color", "Disable terminal colors").option("--no-interactive", "Disable prompts").option("-y, --yes", "Approve supported confirmation prompts").option("--quiet", "Suppress informational diagnostics").option("--agent", "Enable strict machine-readable agent mode").addOption(new Option("--dry-run <mode>", "Preview without applying").choices(["client", "server"])).option("--timeout <duration>", "Request timeout").option("--debug", "Enable debug diagnostics").option("--request-id <id>", "Use a request correlation ID").option("--idempotency-key <key>", "Use an idempotency key").option("--insecure-skip-tls-verify", "Disable TLS verification when supported");
}
function invokedCommandPath(command) {
  const canonicalCategory = command.parent?.name() ?? "";
  const canonicalTool = command.name();
  const rootOperands = command.parent?.parent?.args ?? [];
  const invokedCategory = typeof rootOperands[0] === "string" ? rootOperands[0] : canonicalCategory;
  const invokedTool = typeof rootOperands[1] === "string" ? rootOperands[1] : canonicalTool;
  return `${invokedCategory}.${invokedTool}`;
}
function explicitCommanderOptions(command) {
  const values = command.optsWithGlobals();
  return Object.fromEntries(
    Object.entries(values).filter(([key]) => command.getOptionValueSourceWithGlobals(key) !== "default")
  );
}
function commanderFailure(error) {
  if (!(error instanceof CommanderError))
    return toCliCommandError(error);
  return new CliCommandError(
    error.code === "commander.unknownCommand" ? "unknown_command" : "invalid_arguments",
    cleanCommanderMessage(error.message),
    {
      status: 400,
      exitCode: 2,
      details: { commanderCode: error.code },
      cause: error
    }
  );
}
function inferFallbackGlobals(argv) {
  const agent = argv.includes("--agent") || argv.includes("agent=true");
  const outputToken = argv.find((token) => token.startsWith("output="));
  const outputFlagIndex = argv.findIndex((token) => token === "--output" || token === "-o");
  const output = outputToken?.slice("output=".length) ?? (outputFlagIndex >= 0 ? argv[outputFlagIndex + 1] : void 0);
  return {
    ...DEFAULT_GLOBALS,
    agent,
    output: isOutput(output) ? output : agent ? "json" : "table"
  };
}
function suppressCommanderOutput(program, suppress) {
  if (!suppress)
    return () => void 0;
  const snapshots = commandTree(program).map((command) => ({
    command,
    output: command.configureOutput()
  }));
  for (const snapshot of snapshots) {
    snapshot.command.configureOutput({
      writeOut: () => void 0,
      writeErr: () => void 0,
      outputError: () => void 0
    });
  }
  return () => {
    for (const snapshot of snapshots)
      snapshot.command.configureOutput(snapshot.output);
  };
}
function commandTree(root) {
  return [root, ...root.commands.flatMap(commandTree)];
}
function isMachineOutput(globals) {
  return Boolean(globals.agent || globals.output !== "table");
}
function isExpectedCommanderExit(error) {
  return error.code === "commander.helpDisplayed" || error.code === "commander.version";
}
function cleanCommanderMessage(value) {
  return value.replace(/^error:\s*/i, "").trim() || "Invalid command arguments.";
}
function isOutput(value) {
  return value === "table" || value === "json" || value === "raw-json" || value === "yaml" || value === "jsonl" || value === "name";
}
function isRecord6(value) {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

// src/commands/help.ts
import { Buffer as Buffer4 } from "buffer";
var DEFAULT_LIMIT = 20;
var MAX_LIMIT = 100;
function catalogResult(registry, params) {
  const limit = integerParam(params.limit, DEFAULT_LIMIT, 1, MAX_LIMIT);
  const offset = decodeCursor(stringParam(params.cursor));
  const commands = registry.list({
    query: stringParam(params.query),
    category: stringParam(params.category),
    risk: stringParam(params.risk),
    scope: stringParam(params.scope),
    transport: stringParam(params.transport),
    includeHidden: booleanParam(params.all, false)
  });
  const items = commands.slice(offset, offset + limit).map(({ metadata }) => compactEntry(metadata));
  const nextOffset = offset + items.length;
  return {
    schemaVersion: "help.catalog/v1",
    data: {
      ...registry.catalogMetadata,
      items,
      nextCursor: nextOffset < commands.length ? encodeCursor(nextOffset) : null,
      total: commands.length
    }
  };
}
function commandHelpResult(registry, params) {
  const path3 = requiredStringParam(params.path, "path");
  const metadata = registry.require(path3).metadata;
  return {
    schemaVersion: "help.command/v1",
    data: {
      ...registry.catalogMetadata,
      command: fullEntry(metadata)
    }
  };
}
function compactEntry(metadata) {
  return {
    path: metadata.canonicalPath,
    category: metadata.category,
    tool: metadata.tool,
    source: metadata.source,
    summary: metadata.summary ?? "",
    risk: metadata.risk,
    transport: metadata.transport,
    projectContext: metadata.projectContext,
    scopes: metadata.scopes,
    serverSupported: null
  };
}
function fullEntry(metadata) {
  return {
    ...compactEntry(metadata),
    aliases: metadata.aliases,
    operationId: metadata.operationId ?? null,
    consumedOperations: metadata.consumedOperations ?? [],
    description: metadata.description ?? "",
    parameters: metadata.parameters,
    inputSchema: metadata.inputSchema ?? {},
    outputSchema: metadata.outputSchema ?? {},
    errorSchema: metadata.errorSchema ?? {},
    schemaVersion: metadata.schemaVersion ?? "unversioned",
    schemaDigest: metadata.schemaDigest ?? "unavailable",
    mfaPurpose: metadata.mfaPurpose ?? null,
    agentAllowed: metadata.agentAllowed,
    examples: metadata.examples ?? []
  };
}
function encodeCursor(offset) {
  return Buffer4.from(`v1:${offset}`, "utf8").toString("base64url");
}
function decodeCursor(value) {
  if (!value)
    return 0;
  try {
    const decoded = Buffer4.from(value, "base64url").toString("utf8");
    const match = /^v1:(\d+)$/.exec(decoded);
    if (!match)
      throw new Error("invalid cursor");
    return Number(match[1]);
  } catch (cause) {
    throw new CliCommandError("invalid_arguments", "Invalid catalog cursor.", {
      status: 400,
      exitCode: 2,
      details: { key: "cursor" },
      cause
    });
  }
}
function stringParam(value) {
  return typeof value === "string" && value.length > 0 ? value : void 0;
}
function requiredStringParam(value, key) {
  const result = stringParam(value);
  if (!result) {
    throw new CliCommandError("invalid_arguments", `Missing required argument "${key}".`, {
      status: 400,
      exitCode: 2,
      details: { key, code: "required" }
    });
  }
  return result;
}
function booleanParam(value, fallback) {
  return typeof value === "boolean" ? value : fallback;
}
function integerParam(value, fallback, minimum, maximum) {
  if (value === void 0)
    return fallback;
  if (typeof value !== "number" || !Number.isInteger(value) || value < minimum || value > maximum) {
    throw new CliCommandError(
      "invalid_arguments",
      `Value must be an integer between ${minimum} and ${maximum}.`,
      { status: 400, exitCode: 2 }
    );
  }
  return value;
}

// src/commands/local.ts
import process10 from "process";

// src/auth/access-token.ts
import process9 from "process";

// src/auth/validation.ts
function normalizeCredentialName(name) {
  const normalized = name.trim();
  if (!/^[A-Z0-9][\w.-]{0,95}$/i.test(normalized)) {
    throw new CliCommandError(
      "credential_name_invalid",
      "Credential names must be 1-96 characters using letters, numbers, dot, underscore, or hyphen.",
      { status: 422 }
    );
  }
  return normalized;
}
function normalizeScopes(scopes) {
  const normalized = [...new Set(
    (scopes ?? []).map((scope) => scope.trim()).filter(Boolean)
  )].sort();
  for (const scope of normalized) {
    if (/\s/.test(scope)) {
      throw new CliCommandError(
        "credential_scope_invalid",
        `Credential scope "${scope}" must not contain whitespace.`,
        { status: 422 }
      );
    }
  }
  return normalized;
}
function assertIsoDate(value) {
  if (value === void 0)
    return;
  const timestamp = Date.parse(value);
  if (!Number.isFinite(timestamp) || new Date(timestamp).toISOString() !== value) {
    throw new CliCommandError(
      "credential_expiry_invalid",
      "Credential expiration must be an ISO 8601 UTC timestamp.",
      { status: 422 }
    );
  }
}

// src/auth/access-token.ts
async function storeValidatedAccessToken(store, input) {
  const token = input.token.trim();
  if (!token) {
    throw new CliCommandError(
      "access_token_required",
      "A validated access token is required.",
      { status: 422 }
    );
  }
  assertIsoDate(input.expiresAt);
  const contextName = normalizeContextName(input.context);
  return updateConfig(store, (config) => {
    const origin = normalizeServerOrigin(input.server);
    ensureInstance(config, origin);
    const previousContext = config.contexts[contextName];
    const previousCredential = previousContext?.credential;
    const credentialName = input.credential ? normalizeCredentialName(input.credential) : reusableCredentialName(config, contextName, previousCredential);
    const credential = {
      type: "access_token",
      token,
      scopes: normalizeScopes(input.scopes),
      user: input.user,
      expiresAt: input.expiresAt,
      createdAt: (/* @__PURE__ */ new Date()).toISOString()
    };
    config.credentials[credentialName] = credential;
    config.contexts[contextName] = upsertContext(config, {
      name: contextName,
      server: origin,
      credential: credentialName,
      project: input.project
    });
    if (input.makeCurrent ?? !config.currentContext) {
      config.currentContext = contextName;
    }
    pruneUnreferencedContextResources(config, {
      credential: previousCredential === credentialName ? void 0 : previousCredential,
      instance: previousContext?.instance
    });
  });
}
function accessTokenFromEnvironment(env = process9.env) {
  const token = env.LUNA_TOKEN?.trim();
  if (!token)
    return void 0;
  return {
    type: "access_token",
    token,
    scopes: []
  };
}
function reusableCredentialName(config, contextName, previousCredential) {
  if (previousCredential && config.credentials[previousCredential]?.type === "access_token") {
    return previousCredential;
  }
  const base = normalizeCredentialName(`${contextName}-access-token`);
  if (!Object.hasOwn(config.credentials, base))
    return base;
  let suffix = 2;
  while (Object.hasOwn(config.credentials, `${base}-${suffix}`)) suffix += 1;
  return `${base}-${suffix}`;
}

// src/auth/logout.ts
async function logoutLocal(store, options = {}) {
  let result = { contexts: [], removedCredentials: [] };
  await updateConfig(store, (config) => {
    const contexts = options.all ? Object.keys(config.contexts).sort() : [options.context ?? config.currentContext].filter(
      (name) => Boolean(name)
    );
    if (contexts.length === 0) {
      result = { contexts: [], removedCredentials: [] };
      return;
    }
    const credentials = /* @__PURE__ */ new Set();
    for (const name of contexts) {
      const context = config.contexts[name];
      if (!context) {
        throw new CliCommandError(
          "context_not_found",
          `Context "${name}" does not exist.`,
          { status: 404 }
        );
      }
      if (context.credential)
        credentials.add(context.credential);
      delete context.credential;
    }
    const removed = [];
    for (const credential of credentials) {
      const stillReferenced = Object.values(config.contexts).some(
        (context) => context.credential === credential
      );
      if (!stillReferenced) {
        delete config.credentials[credential];
        removed.push(credential);
      }
    }
    result = { contexts, removedCredentials: removed.sort() };
  });
  return result;
}

// src/auth/oauth.ts
async function beginOAuthLogin(_request) {
  throw oauthUnavailable("OAuth login");
}
function oauthUnavailable(capability) {
  return new CliCommandError(
    "oauth_server_capability_unavailable",
    `${capability} is unavailable until the Luna server exposes the native CLI OAuth endpoints.`,
    {
      status: 501,
      details: {
        capability,
        fallback: "Use a personal access token through stdin or LUNA_TOKEN."
      }
    }
  );
}

// src/auth/status.ts
async function getAuthStatus(store, options = {}) {
  const config = parseConfigDocument(await store.read());
  const environmentCredential = accessTokenFromEnvironment(options.env);
  const names = options.all ? Object.keys(config.contexts).sort() : [options.context ?? config.currentContext].filter(
    (name) => Boolean(name)
  );
  if (names.length === 0)
    return [];
  return names.map((name) => {
    const context = config.contexts[name];
    if (!context) {
      throw new CliCommandError(
        "context_not_found",
        `Context "${name}" does not exist.`,
        { status: 404 }
      );
    }
    const instance = config.instances[context.instance];
    const storedCredential = context.credential ? config.credentials[context.credential] : void 0;
    const credential = environmentCredential ?? storedCredential;
    const source = environmentCredential ? "environment" : "stored";
    return {
      context: name,
      current: config.currentContext === name,
      server: normalizeServerOrigin(instance.server),
      authenticated: credential !== void 0 && !isExpired(credential, options.now),
      credential: credential && context.credential ? {
        name: environmentCredential ? "LUNA_TOKEN" : context.credential,
        type: credential.type,
        scopes: [...credential.scopes],
        user: credential.user,
        expiresAt: credential.expiresAt,
        expired: isExpired(credential, options.now),
        source
      } : credential ? {
        name: "LUNA_TOKEN",
        type: credential.type,
        scopes: [...credential.scopes],
        user: credential.user,
        expiresAt: credential.expiresAt,
        expired: false,
        source
      } : void 0
    };
  });
}
function isExpired(credential, now = /* @__PURE__ */ new Date()) {
  return credential.expiresAt !== void 0 && Date.parse(credential.expiresAt) <= now.getTime();
}

// src/commands/local.ts
var stringSchema = { type: "string" };
var booleanSchema = { type: "boolean" };
var integerSchema = { type: "integer" };
function registerLocalCommands(registry) {
  registerVersion(registry);
  registerHelp(registry);
  registerCompletion(registry);
  registerAuth(registry);
  registerContext(registry);
  registerProjectContext(registry);
  registerApiDiagnostic(registry);
}
function registerAuth(registry) {
  registry.register(localMetadata("auth", "login", {
    summary: "Authenticate a Luna context with an access token.",
    schemaVersion: "auth.login/v1",
    risk: "medium",
    parameters: [
      parameter("mode"),
      parameter("token", {
        sensitive: true,
        valueSources: ["file", "stdin"]
      }),
      parameter("scope", { repeated: true })
    ]
  }), async (invocation, ports) => {
    const mode = optionalString(invocation.params.mode) ?? "access-token";
    if (mode === "device-code") {
      return beginOAuthLogin({
        server: invocation.globals.server ?? "",
        context: invocation.globals.context ?? "default",
        scopes: stringList(invocation.params.scope),
        mode: "device_code"
      });
    }
    if (mode !== "access-token") {
      throw invalidArguments2(
        "mode must be access-token or device-code.",
        "mode"
      );
    }
    const config = await ports.config.read();
    const selected = selectedContext(config, invocation.globals.context);
    const context = invocation.globals.context ?? selected.name ?? "default";
    const server = invocation.globals.server ?? selected.instance?.server;
    if (!server) {
      throw invalidArguments2(
        "auth.login requires server=<https://luna.example.com> when no context is configured.",
        "server"
      );
    }
    const token = optionalString(invocation.params.token)?.trim() ?? optionalString(ports.env?.LUNA_TOKEN)?.trim();
    if (!token) {
      throw invalidArguments2(
        "auth.login requires token=@- or the LUNA_TOKEN environment variable.",
        "token"
      );
    }
    if (!ports.api.validateAccessToken) {
      throw new CliCommandError(
        "unsupported_feature",
        "The API client cannot validate access tokens.",
        { status: 501 }
      );
    }
    const user = await ports.api.validateAccessToken(server, token, invocation.globals);
    const userId = optionalString(user.id);
    await storeValidatedAccessToken(ports.config, {
      context,
      server,
      token,
      scopes: stringList(invocation.params.scope),
      user: userId ? {
        id: userId,
        ...user
      } : void 0,
      makeCurrent: true
    });
    return {
      schemaVersion: "auth.login/v1",
      data: {
        context,
        server,
        authenticated: true,
        user
      }
    };
  });
  registry.register(localMetadata("auth", "status", {
    summary: "Show authentication status without exposing credentials.",
    schemaVersion: "auth.status/v1",
    parameters: [parameter("all", { schema: booleanSchema })]
  }), async (invocation, ports) => ({
    schemaVersion: "auth.status/v1",
    data: await getAuthStatus(ports.config, {
      context: invocation.globals.context,
      all: invocation.params.all === true,
      env: ports.env
    })
  }));
  registry.register(localMetadata("auth", "logout", {
    summary: "Remove credentials from one or all local Luna contexts.",
    schemaVersion: "auth.logout/v1",
    risk: "medium",
    parameters: [parameter("all", { schema: booleanSchema })]
  }), async (invocation, ports) => ({
    schemaVersion: "auth.logout/v1",
    data: await logoutLocal(ports.config, {
      context: invocation.globals.context,
      all: invocation.params.all === true
    })
  }));
}
function registerVersion(registry) {
  registry.register(localMetadata("version", "show", {
    summary: "Show Luna CLI version and runtime information.",
    schemaVersion: "version.show/v1"
  }), async (_invocation, ports) => ({
    schemaVersion: "version.show/v1",
    data: {
      version: ports.version ?? "0.1.0",
      distribution: ports.distribution ?? (typeof process10.versions.bun === "string" ? "binary" : "source"),
      runtime: typeof process10.versions.bun === "string" ? `bun-${process10.versions.bun}` : `node-${process10.versions.node}`,
      platform: process10.platform,
      arch: process10.arch
    }
  }));
}
function registerHelp(registry) {
  registry.register(localMetadata("help", "catalog", {
    summary: "List commands from the machine-readable command catalog.",
    schemaVersion: "help.catalog/v1",
    parameters: [
      parameter("query"),
      parameter("category"),
      parameter("risk"),
      parameter("scope"),
      parameter("transport"),
      parameter("limit", { schema: integerSchema }),
      parameter("cursor"),
      parameter("all", { schema: booleanSchema })
    ]
  }), async (invocation) => catalogResult(registry, invocation.params));
  registry.register(localMetadata("help", "command", {
    summary: "Show the complete machine-readable contract for one command.",
    schemaVersion: "help.command/v1",
    parameters: [parameter("path", { required: true })]
  }), async (invocation) => commandHelpResult(registry, invocation.params));
}
function registerCompletion(registry) {
  for (const shell of ["bash", "zsh", "fish", "powershell"]) {
    registry.register(localMetadata("completion", shell, {
      summary: `Generate ${shell} completion for Luna CLI.`,
      schemaVersion: `completion.${shell}/v1`
    }), async () => ({
      schemaVersion: `completion.${shell}/v1`,
      data: {
        shell,
        script: generateCompletion(shell, registry)
      }
    }));
  }
}
function registerContext(registry) {
  registry.register(localMetadata("context", "list", {
    summary: "List configured Luna contexts.",
    schemaVersion: "context.list/v1"
  }), async (_invocation, ports) => {
    const config = await ports.config.read();
    return {
      schemaVersion: "context.list/v1",
      data: Object.entries(config.contexts).sort(([left], [right]) => left.localeCompare(right)).map(([name, context]) => contextView(name, context, config, config.currentContext === name))
    };
  });
  registry.register(localMetadata("context", "current", {
    summary: "Show the current Luna context.",
    schemaVersion: "context.current/v1"
  }), async (_invocation, ports) => {
    const config = await ports.config.read();
    if (!config.currentContext) {
      return { schemaVersion: "context.current/v1", data: null };
    }
    const context = config.contexts[config.currentContext];
    if (!context) {
      throw new CliCommandError(
        "context_invalid",
        `Current context "${config.currentContext}" does not exist.`,
        { status: 409, details: { context: config.currentContext } }
      );
    }
    return {
      schemaVersion: "context.current/v1",
      data: contextView(config.currentContext, context, config, true)
    };
  });
  registry.register(localMetadata("context", "use", {
    summary: "Switch the current Luna context.",
    schemaVersion: "context.use/v1",
    parameters: [parameter("name", { required: true })]
  }), async (invocation, ports) => {
    const name = requiredString(invocation.params.name, "name");
    const config = await ports.config.read();
    if (!config.contexts[name]) {
      throw new CliCommandError("context_not_found", `Context "${name}" was not found.`, {
        status: 404,
        details: { name }
      });
    }
    await ports.config.write({ ...config, currentContext: name });
    return {
      schemaVersion: "context.use/v1",
      data: contextView(name, config.contexts[name], config, true)
    };
  });
  registry.register(localMetadata("context", "set", {
    summary: "Create or update a Luna context.",
    schemaVersion: "context.set/v1",
    projectContext: "optional",
    parameters: [
      parameter("name", { required: true }),
      parameter("credential"),
      parameter("projectName"),
      parameter("projectIdentifier"),
      parameter("language")
    ]
  }), async (invocation, ports) => {
    const name = requiredString(invocation.params.name, "name");
    const config = await ports.config.read();
    const existing = config.contexts[name];
    const requestedServer = invocation.explicitGlobalKeys.has("server") ? optionalString(invocation.globals.server) : void 0;
    const existingInstance = existing ? config.instances[existing.instance] : void 0;
    const server = requestedServer ? normalizeServer(requestedServer) : existingInstance?.server;
    if (!server) {
      throw invalidArguments2("server is required when creating a context.", "server");
    }
    const instance = findInstanceName(config.instances, server) ?? uniqueInstanceName(name, config.instances);
    const serverChanged = Boolean(existingInstance && existingInstance.server !== server);
    const projectId = invocation.explicitGlobalKeys.has("project") ? optionalString(invocation.globals.project) : void 0;
    const project = projectId ? {
      id: projectId,
      name: optionalString(invocation.params.projectName),
      identifier: optionalString(invocation.params.projectIdentifier)
    } : serverChanged ? null : existing?.project;
    const output = invocation.explicitGlobalKeys.has("output") ? outputFormat(invocation.canonicalGlobalValues.output) : void 0;
    const context = {
      ...existing,
      instance,
      credential: serverChanged ? void 0 : optionalString(invocation.params.credential) ?? existing?.credential,
      project,
      output: output ?? existing?.output,
      language: optionalString(invocation.params.language) ?? existing?.language
    };
    const next = {
      ...config,
      currentContext: config.currentContext ?? name,
      instances: {
        ...config.instances,
        [instance]: { ...config.instances[instance] ?? {}, server }
      },
      contexts: { ...config.contexts, [name]: context }
    };
    await ports.config.write(next);
    return {
      schemaVersion: "context.set/v1",
      data: contextView(name, context, next, next.currentContext === name)
    };
  });
  registry.register(localMetadata("context", "rename", {
    summary: "Rename a Luna context.",
    schemaVersion: "context.rename/v1",
    parameters: [
      parameter("name", { required: true }),
      parameter("newName", { required: true })
    ]
  }), async (invocation, ports) => {
    const name = requiredString(invocation.params.name, "name");
    const newName = requiredString(invocation.params.newName, "newName");
    const config = await ports.config.read();
    const context = config.contexts[name];
    if (!context)
      throw notFound(name);
    if (config.contexts[newName]) {
      throw new CliCommandError("context_exists", `Context "${newName}" already exists.`, {
        status: 409,
        details: { newName }
      });
    }
    const contexts = { ...config.contexts };
    delete contexts[name];
    contexts[newName] = context;
    const next = {
      ...config,
      currentContext: config.currentContext === name ? newName : config.currentContext,
      contexts
    };
    await ports.config.write(next);
    return {
      schemaVersion: "context.rename/v1",
      data: contextView(newName, context, next, next.currentContext === newName)
    };
  });
  registry.register(localMetadata("context", "delete", {
    summary: "Delete a Luna context.",
    schemaVersion: "context.delete/v1",
    risk: "high",
    parameters: [parameter("name", { required: true })]
  }), async (invocation, ports) => {
    const name = requiredString(invocation.params.name, "name");
    const config = await ports.config.read();
    if (!config.contexts[name])
      throw notFound(name);
    if (config.currentContext === name && !invocation.globals.yes) {
      throw new CliCommandError(
        "confirmation_required",
        "Deleting the current context requires yes=true or --yes.",
        { status: 409, details: { name } }
      );
    }
    const contexts = { ...config.contexts };
    delete contexts[name];
    await ports.config.write({
      ...config,
      currentContext: config.currentContext === name ? null : config.currentContext,
      contexts
    });
    return { schemaVersion: "context.delete/v1", data: { name, deleted: true } };
  });
  registry.register(localMetadata("context", "view", {
    summary: "Show the redacted Luna configuration.",
    schemaVersion: "context.view/v1"
  }), async (_invocation, ports) => ({
    schemaVersion: "context.view/v1",
    data: redactConfig(await ports.config.read())
  }));
}
function registerProjectContext(registry) {
  registry.register(localMetadata("project", "current", {
    summary: "Show the project selected by the current context.",
    schemaVersion: "project.current/v1",
    projectContext: "optional"
  }), async (invocation, ports) => {
    const config = await ports.config.read();
    const selected = selectedContext(config, invocation.globals.context);
    return {
      schemaVersion: "project.current/v1",
      data: {
        context: selected.name,
        server: selected.instance?.server ?? invocation.globals.server ?? null,
        project: invocation.explicitGlobalKeys.has("project") ? invocation.globals.project ? { id: invocation.globals.project } : null : selected.context?.project ?? null,
        source: invocation.explicitGlobalKeys.has("project") ? "argument" : ports.env?.LUNA_PROJECT ? "environment" : selected.context?.project ? "context" : "none"
      }
    };
  });
  registry.register(localMetadata("project", "use", {
    summary: "Set the current context project after server validation.",
    schemaVersion: "project.use/v1",
    projectContext: "optional"
  }), async (invocation, ports) => {
    if (!invocation.explicitGlobalKeys.has("project")) {
      throw invalidArguments2("project.use requires an explicit project=<id-or-identifier>.", "project");
    }
    const value = requiredString(invocation.globals.project, "project");
    if (!ports.api.resolveProject) {
      throw new CliCommandError(
        "unsupported_feature",
        "The API client does not provide project resolution.",
        { status: 501 }
      );
    }
    const config = await ports.config.read();
    const selected = selectedContext(config, invocation.globals.context);
    if (!selected.name || !selected.context) {
      throw new CliCommandError("context_required", "A current context is required.", {
        status: 400,
        exitCode: 2
      });
    }
    const project = await ports.api.resolveProject(value, invocation.globals);
    const context = { ...selected.context, project };
    const next = {
      ...config,
      contexts: { ...config.contexts, [selected.name]: context }
    };
    await ports.config.write(next);
    return { schemaVersion: "project.use/v1", data: project };
  });
  registry.register(localMetadata("project", "unset", {
    summary: "Clear the project selected by the current context.",
    schemaVersion: "project.unset/v1"
  }), async (invocation, ports) => {
    const config = await ports.config.read();
    const selected = selectedContext(config, invocation.globals.context);
    if (!selected.name || !selected.context) {
      throw new CliCommandError("context_required", "A current context is required.", {
        status: 400,
        exitCode: 2
      });
    }
    const context = { ...selected.context, project: null };
    await ports.config.write({
      ...config,
      contexts: { ...config.contexts, [selected.name]: context }
    });
    return { schemaVersion: "project.unset/v1", data: { context: selected.name, project: null } };
  });
}
function registerApiDiagnostic(registry) {
  registry.register({
    ...localMetadata("api", "request", {
      summary: "Send a diagnostic request to a Luna API path.",
      schemaVersion: "api.request/v1",
      risk: "medium",
      agentAllowed: false,
      transport: "http",
      parameters: [
        parameter("method", { required: true }),
        parameter("path", { required: true }),
        parameter("body", {
          valueSources: ["file", "stdin"],
          schema: { type: ["object", "array", "string", "null"] }
        }),
        parameter("allowDiagnostic", { schema: booleanSchema })
      ],
      inputSchema: { type: "object", additionalProperties: true }
    }),
    source: "local"
  }, async (invocation, ports) => {
    const method = requiredString(invocation.params.method, "method").toUpperCase();
    if (!["GET", "HEAD", "POST", "PUT", "PATCH", "DELETE"].includes(method)) {
      throw invalidArguments2(`Unsupported HTTP method "${method}".`, "method");
    }
    const path3 = requiredString(invocation.params.path, "path");
    if (!path3.startsWith("/api/") || path3.startsWith("//") || /^[a-z][a-z0-9+.-]*:/i.test(path3)) {
      throw invalidArguments2("path must be a relative Luna API path beginning with /api/.", "path");
    }
    const { method: _method, path: _path, allowDiagnostic: _allow, ...params } = invocation.params;
    const result = await ports.api.request({ method, path: path3, params, globals: invocation.globals });
    return asResult(result, "api.request/v1");
  });
}
function localMetadata(category, tool, details) {
  return {
    category,
    tool,
    source: "local",
    risk: "low",
    transport: "local",
    projectContext: "none",
    ...details
  };
}
function parameter(name, options = {}) {
  return { name, schema: stringSchema, valueSources: ["inline"], ...options };
}
function contextView(name, context, config, current) {
  const instance = config.instances[context.instance];
  const credential = context.credential ? config.credentials[context.credential] : void 0;
  return {
    name,
    current,
    instance: context.instance,
    server: instance?.server ?? null,
    credential: context.credential ?? null,
    authType: optionalString(credential?.type) ?? null,
    userId: optionalString(credential?.userId) ?? null,
    expiresAt: optionalString(credential?.expiresAt) ?? null,
    scopes: Array.isArray(credential?.scopes) ? credential.scopes : [],
    project: context.project ?? null,
    language: context.language ?? null,
    output: context.output || null
  };
}
function selectedContext(config, override) {
  const name = override ?? config.currentContext ?? void 0;
  const context = name ? config.contexts[name] : void 0;
  return {
    name,
    context,
    instance: context ? config.instances[context.instance] : void 0
  };
}
function findInstanceName(instances, server) {
  return Object.entries(instances).find(([, instance]) => instance.server === server)?.[0];
}
function uniqueInstanceName(preferred, instances) {
  if (!instances[preferred])
    return preferred;
  let suffix = 2;
  while (instances[`${preferred}-${suffix}`]) suffix += 1;
  return `${preferred}-${suffix}`;
}
function normalizeServer(value) {
  let url;
  try {
    url = new URL(value);
  } catch (cause) {
    throw new CliCommandError("invalid_arguments", "server must be an absolute HTTP(S) URL.", {
      status: 400,
      exitCode: 2,
      details: { key: "server" },
      cause
    });
  }
  if (!["http:", "https:"].includes(url.protocol) || url.username || url.password || url.hash || url.pathname !== "/" && url.pathname !== "") {
    throw invalidArguments2(
      "server must be an HTTP(S) origin without credentials, path, query, or fragment.",
      "server"
    );
  }
  if (url.search)
    throw invalidArguments2("server must not contain a query.", "server");
  return url.origin;
}
function redactConfig(config) {
  return {
    ...config,
    credentials: Object.fromEntries(
      Object.entries(config.credentials).map(([name, credential]) => [
        name,
        redactRecord(credential)
      ])
    )
  };
}
function redactRecord(value) {
  return Object.fromEntries(
    Object.entries(value).map(([key, entry]) => [
      key,
      /token|secret|password|cookie|authorization|recovery/i.test(key) ? entry === void 0 || entry === null || entry === "" ? entry : "******" : typeof entry === "object" && entry !== null && !Array.isArray(entry) ? redactRecord(entry) : entry
    ])
  );
}
function outputFormat(value) {
  if (value === "")
    return "";
  if (value === "table" || value === "json" || value === "raw-json" || value === "yaml" || value === "jsonl" || value === "name") {
    return value;
  }
  if (value === void 0)
    return void 0;
  throw invalidArguments2(`Unsupported output format "${String(value)}".`, "output");
}
function asResult(value, schemaVersion) {
  if (typeof value === "object" && value !== null && "data" in value) {
    return value;
  }
  return { data: value, schemaVersion };
}
function requiredString(value, key) {
  const result = optionalString(value);
  if (!result)
    throw invalidArguments2(`Missing required argument "${key}".`, key);
  return result;
}
function optionalString(value) {
  return typeof value === "string" && value.length > 0 ? value : void 0;
}
function stringList(value) {
  if (Array.isArray(value)) {
    return value.filter((entry) => typeof entry === "string");
  }
  return typeof value === "string" ? [value] : [];
}
function notFound(name) {
  return new CliCommandError("context_not_found", `Context "${name}" was not found.`, {
    status: 404,
    details: { name }
  });
}
function invalidArguments2(message, key) {
  return new CliCommandError("invalid_arguments", message, {
    status: 400,
    exitCode: 2,
    details: key ? { key } : {}
  });
}

// src/commands/registry.ts
var NAME_PATTERN = /^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$/;
var CommandRegistry = class {
  catalogMetadata;
  #commands = /* @__PURE__ */ new Map();
  #aliases = /* @__PURE__ */ new Map();
  constructor(metadata = {}) {
    this.catalogMetadata = {
      catalogVersion: metadata.catalogVersion ?? "cli.luna.devops/catalog/v1",
      openapiDigest: metadata.openapiDigest ?? "unavailable",
      schemaDigest: metadata.schemaDigest ?? "unavailable"
    };
  }
  register(metadata, handler) {
    const normalized = normalizeMetadata(metadata);
    const path3 = normalized.canonicalPath;
    if (this.#commands.has(path3)) {
      throw new CliCommandError(
        "duplicate_command",
        `Command "${path3}" is registered more than once.`,
        { status: 409, details: { path: path3 } }
      );
    }
    validateSource(normalized);
    const registered = { metadata: normalized, handler };
    this.#commands.set(path3, registered);
    for (const alias of normalized.aliases) {
      const aliasPath = `${normalized.category}.${alias}`;
      this.#registerAlias(aliasPath, path3);
    }
    for (const categoryAlias of normalized.categoryAliases) {
      this.#registerAlias(`${categoryAlias}.${normalized.tool}`, path3);
      for (const alias of normalized.aliases) {
        this.#registerAlias(`${categoryAlias}.${alias}`, path3);
      }
    }
    return registered;
  }
  get(path3, allowAlias = false) {
    const normalizedPath = normalizePath(path3);
    const canonicalPath = allowAlias ? this.#aliases.get(normalizedPath) ?? normalizedPath : normalizedPath;
    return this.#commands.get(canonicalPath);
  }
  require(path3, allowAlias = false) {
    const command = this.get(path3, allowAlias);
    if (!command) {
      throw new CliCommandError("unknown_command", `Unknown command "${path3}".`, {
        status: 400,
        exitCode: 2,
        details: { path: path3 }
      });
    }
    return command;
  }
  list(options = {}) {
    const query = options.query?.trim().toLocaleLowerCase();
    return [...this.#commands.values()].filter(({ metadata }) => options.includeHidden || !metadata.hidden).filter(({ metadata }) => !options.category || metadata.category === options.category).filter(({ metadata }) => !options.risk || metadata.risk === options.risk).filter(({ metadata }) => !options.scope || metadata.scopes.includes(options.scope)).filter(({ metadata }) => !options.transport || metadata.transport === options.transport).filter(({ metadata }) => {
      if (!query)
        return true;
      return [
        metadata.canonicalPath,
        metadata.summary,
        metadata.description,
        metadata.operationId,
        ...metadata.scopes
      ].some((value) => value?.toLocaleLowerCase().includes(query));
    }).sort(
      (left, right) => left.metadata.canonicalPath.localeCompare(right.metadata.canonicalPath)
    );
  }
  categories() {
    return [...new Set(this.list({ includeHidden: true }).map((item) => item.metadata.category))].sort();
  }
  categoryAliases(category) {
    return [
      ...new Set(
        this.list({ includeHidden: true }).filter((item) => item.metadata.category === category).flatMap((item) => item.metadata.categoryAliases)
      )
    ].sort();
  }
  #registerAlias(aliasPath, canonicalPath) {
    const existing = this.#aliases.get(aliasPath);
    if (existing && existing !== canonicalPath) {
      throw new CliCommandError(
        "duplicate_command_alias",
        `Command alias "${aliasPath}" is ambiguous.`,
        { status: 409, details: { aliasPath, commands: [existing, canonicalPath] } }
      );
    }
    if (this.#commands.has(aliasPath) && aliasPath !== canonicalPath) {
      throw new CliCommandError(
        "command_alias_conflict",
        `Command alias "${aliasPath}" conflicts with a canonical command.`,
        { status: 409, details: { aliasPath, canonicalPath } }
      );
    }
    this.#aliases.set(aliasPath, canonicalPath);
  }
};
function normalizeMetadata(metadata) {
  validateName(metadata.category, "category");
  validateName(metadata.tool, "tool");
  const canonicalPath = `${metadata.category}.${metadata.tool}`;
  if (metadata.canonicalPath && normalizePath(metadata.canonicalPath) !== canonicalPath) {
    throw new CliCommandError(
      "invalid_command_path",
      `Canonical path must be "${canonicalPath}".`,
      { status: 400, details: { canonicalPath: metadata.canonicalPath } }
    );
  }
  for (const alias of metadata.aliases ?? []) validateName(alias, "tool alias");
  for (const alias of metadata.categoryAliases ?? []) validateName(alias, "category alias");
  return Object.freeze({
    ...metadata,
    canonicalPath,
    aliases: Object.freeze([...metadata.aliases ?? []]),
    categoryAliases: Object.freeze([...metadata.categoryAliases ?? []]),
    parameters: Object.freeze([...metadata.parameters ?? []]),
    scopes: Object.freeze([...metadata.scopes ?? []]),
    risk: metadata.risk ?? "low",
    transport: metadata.transport ?? (metadata.source === "local" ? "local" : "http"),
    projectContext: metadata.projectContext ?? "none",
    agentAllowed: metadata.agentAllowed ?? true
  });
}
function normalizePath(path3) {
  const parts = path3.trim().split(".");
  if (parts.length !== 2) {
    throw new CliCommandError(
      "invalid_command_path",
      "Commands must use the fixed <category>.<tool> path.",
      { status: 400, exitCode: 2, details: { path: path3 } }
    );
  }
  validateName(parts[0], "category");
  validateName(parts[1], "tool");
  return `${parts[0]}.${parts[1]}`;
}
function validateName(value, label) {
  if (!NAME_PATTERN.test(value)) {
    throw new CliCommandError(
      "invalid_command_name",
      `Invalid ${label} "${value}".`,
      { status: 400, exitCode: 2, details: { label, value } }
    );
  }
}
function validateSource(metadata) {
  if (metadata.source === "openapi" && !metadata.operationId) {
    throw new CliCommandError(
      "missing_operation_id",
      `OpenAPI command "${metadata.canonicalPath}" must declare operationId.`,
      { status: 400, details: { path: metadata.canonicalPath } }
    );
  }
  if (metadata.source === "local" && metadata.operationId) {
    throw new CliCommandError(
      "invalid_local_operation",
      `Local command "${metadata.canonicalPath}" cannot declare operationId.`,
      { status: 400, details: { path: metadata.canonicalPath } }
    );
  }
  if (metadata.source === "protocol" && !metadata.consumedOperations?.length) {
    throw new CliCommandError(
      "missing_protocol_operations",
      `Protocol command "${metadata.canonicalPath}" must declare consumed operations.`,
      { status: 400, details: { path: metadata.canonicalPath } }
    );
  }
}

// src/commands/openapi.ts
function createRegistryFromContract(contractModule) {
  const catalog = extractCatalog(contractModule);
  const registry = new CommandRegistry(catalog.metadata);
  registerOpenApiCommands(registry, catalog.entries);
  return registry;
}
function extractCatalog(contractModule) {
  const moduleRecord = asRecord3(contractModule);
  const candidate = moduleRecord.OPERATION_CATALOG ?? moduleRecord.commandCatalog ?? moduleRecord.COMMAND_CATALOG ?? moduleRecord.cliCommandCatalog ?? moduleRecord.default;
  const value = typeof candidate === "function" ? candidate() : candidate;
  const catalog = asRecord3(value);
  const exportedMetadata = asRecord3(moduleRecord.OPERATION_CATALOG_METADATA);
  const entries = Array.isArray(value) ? value : Array.isArray(catalog.commands) ? catalog.commands : Array.isArray(catalog.entries) ? catalog.entries : [];
  return {
    metadata: {
      catalogVersion: stringValue2(exportedMetadata.catalogVersion) ?? stringValue2(catalog.metadata?.catalogVersion) ?? stringValue2(catalog.catalogVersion),
      openapiDigest: stringValue2(exportedMetadata.openapiDigest) ?? stringValue2(catalog.metadata?.openapiDigest) ?? stringValue2(catalog.openapiDigest),
      schemaDigest: stringValue2(exportedMetadata.catalogDigest) ?? stringValue2(exportedMetadata.schemaDigest) ?? stringValue2(catalog.metadata?.schemaDigest) ?? stringValue2(catalog.schemaDigest)
    },
    entries: entries.map(normalizeCatalogEntry)
  };
}
function registerOpenApiCommands(registry, entries) {
  for (const entry of entries) {
    if (entry.source !== "openapi")
      continue;
    registry.register(entry, async (invocation, ports) => {
      if (!entry.operationId) {
        throw new CliCommandError(
          "missing_operation_id",
          `Command "${invocation.metadata.canonicalPath}" has no operationId.`,
          { status: 500 }
        );
      }
      const result = await ports.api.execute({
        operationId: entry.operationId,
        params: invocation.params,
        globals: invocation.globals,
        metadata: invocation.metadata
      });
      return asCommandResult(result, invocation.metadata.schemaVersion);
    });
  }
}
function normalizeCatalogEntry(value) {
  const entry = asRecord3(value);
  const command = asRecord3(entry.command);
  const extension = asRecord3(entry["x-luna-cli"] ?? entry.cli);
  const category = requiredString2(entry.category ?? command.category ?? extension.category, "category");
  const tool = requiredString2(entry.tool ?? command.tool ?? extension.tool, "tool");
  const parameters = parameterArray(entry.parameters);
  const requestBody = asRecord3(entry.requestBody);
  if (Object.keys(requestBody).length > 0) {
    parameters.push({
      name: "body",
      location: "body",
      description: "OpenAPI request body.",
      required: booleanValue(requestBody.required),
      valueSources: ["file", "stdin"],
      schema: {
        type: ["object", "array", "string", "null"],
        contentTypes: stringArray(requestBody.contentTypes),
        schemaRefs: stringArray(requestBody.schemaRefs)
      }
    });
  }
  return {
    category,
    tool,
    canonicalPath: stringValue2(entry.canonicalPath ?? command.canonicalPath),
    categoryAliases: stringArray(entry.categoryAliases ?? extension.categoryAliases),
    aliases: stringArray(entry.aliases ?? extension.aliases),
    source: "openapi",
    operationId: stringValue2(entry.operationId),
    consumedOperations: stringArray(entry.consumedOperations),
    summary: stringValue2(entry.summary),
    summaryKey: stringValue2(entry.summaryKey),
    description: stringValue2(entry.description),
    descriptionKey: stringValue2(entry.descriptionKey),
    parameters,
    inputSchema: schemaValue(entry.inputSchema),
    outputSchema: schemaValue(entry.outputSchema),
    errorSchema: schemaValue(entry.errorSchema),
    schemaVersion: stringValue2(entry.schemaVersion),
    schemaDigest: stringValue2(entry.schemaDigest),
    scopes: stringArray(entry.scopes ?? command.requiredScopes ?? extension.scopes),
    mfaPurpose: stringValue2(entry.mfaPurpose ?? extension.mfaPurpose),
    risk: riskValue(entry.risk ?? command.risk ?? extension.risk),
    transport: transportValue(entry.transport ?? command.transport ?? extension.transport),
    projectContext: projectContextValue(
      entry.projectContext ?? asRecord3(extension.projectContext).mode
    ) ?? inferProjectContext(parameters),
    streaming: booleanValue(entry.streaming),
    hidden: booleanValue(entry.hidden ?? command.hidden),
    agentAllowed: typeof entry.agentAllowed === "boolean" ? entry.agentAllowed : void 0,
    examples: stringArray(entry.examples),
    method: stringValue2(entry.method),
    path: stringValue2(entry.path)
  };
}
function parameterArray(value) {
  if (!Array.isArray(value))
    return [];
  return value.map((item) => {
    const parameter2 = asRecord3(item);
    return {
      name: requiredString2(parameter2.name, "parameter name"),
      location: parameterLocation(parameter2.in ?? parameter2.location),
      description: stringValue2(parameter2.description),
      descriptionKey: stringValue2(parameter2.descriptionKey),
      required: booleanValue(parameter2.required),
      repeated: booleanValue(parameter2.repeated),
      sensitive: booleanValue(parameter2.sensitive),
      valueSources: valueSourceArray(parameter2.valueSources),
      schema: schemaValue(parameter2.schema)
    };
  });
}
function valueSourceArray(value) {
  if (!Array.isArray(value))
    return void 0;
  return value.filter(
    (item) => item === "inline" || item === "file" || item === "stdin"
  );
}
function asCommandResult(value, schemaVersion) {
  const record = asRecord3(value);
  if ("data" in record && ("schemaVersion" in record || "meta" in record)) {
    return value;
  }
  return { data: value, schemaVersion };
}
function riskValue(value) {
  return value === "low" || value === "medium" || value === "high" || value === "critical" ? value : void 0;
}
function transportValue(value) {
  return value === "local" || value === "http" || value === "sse" || value === "websocket" || value === "download" || value === "upload" ? value : void 0;
}
function inferProjectContext(parameters) {
  const projectParameter = parameters.find(
    (parameter2) => parameter2.name === "project" || parameter2.name === "projectId" || parameter2.name === "projectID"
  );
  if (!projectParameter)
    return "none";
  return projectParameter.required ? "required" : "optional";
}
function parameterLocation(value) {
  return value === "query" || value === "header" || value === "path" || value === "cookie" || value === "body" ? value : void 0;
}
function projectContextValue(value) {
  return value === "required" || value === "optional" || value === "none" ? value : void 0;
}
function schemaValue(value) {
  return typeof value === "object" && value !== null ? value : void 0;
}
function requiredString2(value, label) {
  const result = stringValue2(value);
  if (!result) {
    throw new CliCommandError("invalid_command_catalog", `Missing ${label}.`, {
      status: 500
    });
  }
  return result;
}
function stringValue2(value) {
  return typeof value === "string" && value.length > 0 ? value : void 0;
}
function booleanValue(value) {
  return typeof value === "boolean" ? value : void 0;
}
function stringArray(value) {
  return Array.isArray(value) ? value.filter((item) => typeof item === "string") : [];
}
function asRecord3(value) {
  return typeof value === "object" && value !== null ? value : {};
}

// src/commands/output.ts
import { Buffer as Buffer5 } from "buffer";
import process11 from "process";

// src/output/json.ts
function stringifyJson(value, options = {}) {
  const safeValue = options.redact === false ? value : redactValue(value);
  const serialized = JSON.stringify(safeValue, null, options.pretty ? 2 : void 0);
  return escapeUnsafeJsonCharacters(serialized ?? "null");
}
function stringifyJsonLine(value) {
  return `${stringifyJson(value)}
`;
}

// src/output/render.ts
import { stringify as stringifyYaml } from "yaml";
function renderTable(rows, options = {}) {
  if (rows.length === 0) return sanitizeTerminalText(options.emptyText ?? "");
  const columns = options.columns ?? inferColumns(rows);
  if (columns.length === 0) return "";
  const matrix = [
    columns.map((column) => sanitizeTerminalText(column.header ?? column.key)),
    ...rows.map((row) => columns.map((column) => formatCell(column.render ? column.render(row[column.key], row) : row[column.key])))
  ];
  const widths = columns.map((_, columnIndex) => Math.max(...matrix.map((row) => displayWidth(row[columnIndex] ?? ""))));
  return matrix.map((row) => {
    const cells = row.map((cell, columnIndex) => padDisplay(cell, widths[columnIndex]));
    return cells.join("  ").trimEnd();
  }).join("\n");
}
function renderFieldView(value, labels = {}) {
  const safe = redactValue(value);
  const entries = Object.entries(safe);
  if (entries.length === 0) return "";
  const width = Math.max(...entries.map(([key]) => displayWidth(labels[key] ?? key)));
  return entries.map(([key, fieldValue]) => `${padDisplay(sanitizeTerminalText(labels[key] ?? key), width)}  ${formatCell(fieldValue)}`).join("\n");
}
function renderHuman(value, options = {}) {
  if (Array.isArray(value)) {
    const rows = value.filter(isRecord7);
    return rows.length === value.length ? renderTable(rows, options) : value.map(formatCell).join("\n");
  }
  if (isRecord7(value)) return renderFieldView(value);
  return formatCell(value);
}
function renderYaml(value) {
  return stringifyYaml(redactValue(value), {
    lineWidth: 0,
    sortMapEntries: false
  }).trimEnd();
}
function renderNames(value) {
  if (Array.isArray(value)) return value.map(extractName).filter(Boolean).join("\n");
  return extractName(value);
}
function inferColumns(rows) {
  const keys = /* @__PURE__ */ new Set();
  for (const row of rows) {
    for (const key of Object.keys(row)) keys.add(key);
  }
  return [...keys].map((key) => ({ key }));
}
function extractName(value) {
  if (isRecord7(value)) {
    for (const key of ["name", "id", "identifier"]) {
      if (typeof value[key] === "string") return sanitizeTerminalText(value[key]);
    }
    return "";
  }
  return typeof value === "string" ? sanitizeTerminalText(value) : "";
}
function formatCell(value) {
  const safe = redactValue(value);
  if (safe === null || safe === void 0) return "";
  if (typeof safe === "string") return sanitizeTerminalText(safe).replace(/[\r\n]+/gu, " ");
  if (typeof safe === "number" || typeof safe === "boolean" || typeof safe === "bigint") {
    return String(safe);
  }
  return stringifyJson(safe);
}
function displayWidth(value) {
  let width = 0;
  for (const character of value) {
    const codePoint = character.codePointAt(0);
    width += isWideCodePoint(codePoint) ? 2 : 1;
  }
  return width;
}
function padDisplay(value, width) {
  return `${value}${" ".repeat(Math.max(0, width - displayWidth(value)))}`;
}
function isWideCodePoint(codePoint) {
  return codePoint >= 4352 && (codePoint <= 4447 || codePoint === 9001 || codePoint === 9002 || codePoint >= 11904 && codePoint <= 42191 && codePoint !== 12351 || codePoint >= 44032 && codePoint <= 55203 || codePoint >= 63744 && codePoint <= 64255 || codePoint >= 65040 && codePoint <= 65049 || codePoint >= 65072 && codePoint <= 65135 || codePoint >= 65280 && codePoint <= 65376 || codePoint >= 65504 && codePoint <= 65510 || codePoint >= 127744 && codePoint <= 129791 || codePoint >= 131072 && codePoint <= 262141);
}
function isRecord7(value) {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

// src/output/channels.ts
var OutputChannels = class {
  #streams;
  #quiet;
  constructor(streams = { stdout: process.stdout, stderr: process.stderr }, options = {}) {
    this.#streams = streams;
    this.#quiet = options.quiet ?? false;
  }
  writeResult(value, options) {
    let rendered;
    switch (options.format) {
      case "json":
        rendered = stringifyJson(value, { pretty: false });
        break;
      case "raw-json":
        rendered = stringifyJson(options.rawData ?? value, { pretty: false });
        break;
      case "yaml":
        rendered = renderYaml(value);
        break;
      case "name":
        rendered = renderNames(value);
        break;
      case "jsonl":
        if (!Array.isArray(value)) {
          rendered = stringifyJsonLine(value).trimEnd();
          break;
        }
        rendered = value.map((item) => stringifyJson(item)).join("\n");
        break;
      default:
        rendered = renderHuman(value, options.human);
    }
    this.#writeLine(this.#streams.stdout, rendered);
  }
  writeJsonLine(value) {
    this.#streams.stdout.write(stringifyJsonLine(value));
  }
  writeInfo(message) {
    if (!this.#quiet) this.#writeLine(this.#streams.stderr, sanitizeTerminalText(message));
  }
  writeWarning(message) {
    if (!this.#quiet) this.#writeLine(this.#streams.stderr, sanitizeTerminalText(message));
  }
  writeDebug(message, enabled) {
    if (enabled) this.#writeLine(this.#streams.stderr, sanitizeTerminalText(message));
  }
  writeError(error, machine = false) {
    const normalized = normalizeLunaError(error);
    const rendered = machine ? stringifyJson(toErrorDocument(normalized)) : `${normalized.code}: ${sanitizeTerminalText(normalized.message)}`;
    this.#writeLine(this.#streams.stderr, rendered);
    return normalized.exitCode;
  }
  #writeLine(stream, value) {
    if (value.length === 0) return;
    stream.write(value.endsWith("\n") ? value : `${value}
`);
  }
};

// src/output/envelope.ts
import { randomUUID as randomUUID2 } from "crypto";
var CLI_API_VERSION = "cli.luna.devops/v1";
function createSuccessEnvelope(schemaVersion, operationId, command, data, meta = {}) {
  return {
    apiVersion: CLI_API_VERSION,
    schemaVersion,
    operationId,
    command,
    data: redactValue(data),
    meta: redactValue(meta)
  };
}

// src/commands/output.ts
var CommandOutput = class {
  #streams;
  #version;
  #translate;
  constructor(options = {}) {
    this.#streams = options.streams ?? {
      stdout: process11.stdout,
      stderr: process11.stderr
    };
    this.#version = options.version ?? "0.1.0";
    this.#translate = options.translate;
  }
  writeSuccess(metadata, result, globals) {
    const channels = new OutputChannels(this.#streams, { quiet: globals.quiet });
    const envelope = createSuccessEnvelope(
      result.schemaVersion ?? metadata.schemaVersion ?? "unversioned",
      metadata.operationId ?? metadata.canonicalPath,
      metadata.canonicalPath,
      result.data,
      {
        requestId: stringMeta(result.meta, "requestId") ?? globals.requestId,
        server: globals.server,
        context: globals.context,
        projectId: stringMeta(result.meta, "projectId") ?? globals.project,
        cliVersion: this.#version,
        openapiDigest: metadata.schemaDigest
      }
    );
    channels.writeResult(
      globals.output === "table" || globals.output === "name" ? result.data : envelope,
      {
        format: globals.output,
        rawData: result.data
      }
    );
  }
  writeError(error, globals) {
    const channels = new OutputChannels(this.#streams, { quiet: globals?.quiet });
    const machine = Boolean(globals?.agent || globals?.output !== "table");
    if (machine || !this.#translate) {
      channels.writeError(error, machine);
      return;
    }
    const normalized = normalizeLunaError(error);
    channels.writeError(new LunaError(
      normalized.code,
      this.#translate(`errors.${normalized.code}`, normalized.message, globals?.lang),
      {
        status: normalized.status,
        exitCode: normalized.exitCode,
        retryable: normalized.retryable,
        requestId: normalized.requestId,
        retryAfter: normalized.retryAfter,
        purpose: normalized.purpose,
        fields: normalized.fields,
        details: normalized.details
      }
    ));
  }
};
function stringMeta(meta, key) {
  const value = meta?.[key];
  return typeof value === "string" ? value : void 0;
}

// src/i18n/index.ts
import { createInstance } from "i18next";

// src/i18n/locale.ts
import process12 from "process";
function normalizeLocale(locale) {
  if (!locale)
    return void 0;
  const normalized = locale.trim().replace(/_/gu, "-").replace(/[.@].*$/u, "");
  if (!normalized)
    return void 0;
  const lower = normalized.toLocaleLowerCase();
  if (lower === "zh" || lower.startsWith("zh-cn") || lower.startsWith("zh-hans"))
    return "zh-CN";
  if (lower === "en" || lower.startsWith("en-"))
    return "en-US";
  return void 0;
}
function detectLocale(options = {}) {
  const env = options.env ?? process12.env;
  const candidates = [
    options.explicit,
    options.context,
    env.LC_ALL,
    env.LC_MESSAGES,
    env.LANG,
    options.runtimeLocale ?? runtimeLocale()
  ];
  for (const candidate of candidates) {
    if (candidate?.trim())
      return normalizeLocale(candidate) ?? "en-US";
  }
  return "en-US";
}
function runtimeLocale() {
  try {
    return new Intl.DateTimeFormat().resolvedOptions().locale;
  } catch {
    return void 0;
  }
}

// src/i18n/resources.ts
var resources = {
  "en-US": {
    translation: {
      common: {
        empty: "No items found.",
        error: "Command failed.",
        warning: "Warning"
      },
      cli: {
        description: "Luna DevOps command-line client for people and agents"
      },
      confirm: {
        execute: "Run this command?"
      },
      errors: {
        invalid_arguments: "Input validation failed.",
        unauthenticated: "Authentication is required.",
        forbidden: "You do not have permission to perform this operation.",
        not_found: "The requested resource was not found.",
        conflict: "The resource changed or is not in the required state.",
        retry_later: "The operation cannot be completed yet. Try again later.",
        service_failure: "The service is temporarily unavailable.",
        confirmation_required: "This operation requires confirmation.",
        operation_cancelled: "Operation cancelled.",
        server_plan_required: "This high-risk operation requires a server-issued execution plan."
      },
      table: {
        name: "Name",
        status: "Status",
        type: "Type",
        createdAt: "Created",
        updatedAt: "Updated"
      }
    }
  },
  "zh-CN": {
    translation: {
      common: {
        empty: "\u6CA1\u6709\u627E\u5230\u6570\u636E\u3002",
        error: "\u547D\u4EE4\u6267\u884C\u5931\u8D25\u3002",
        warning: "\u8B66\u544A"
      },
      cli: {
        description: "\u9762\u5411\u7528\u6237\u548C\u667A\u80FD\u4F53\u7684 Luna DevOps \u547D\u4EE4\u884C\u5BA2\u6237\u7AEF"
      },
      confirm: {
        execute: "\u786E\u8BA4\u6267\u884C\u6B64\u547D\u4EE4\u5417\uFF1F"
      },
      errors: {
        invalid_arguments: "\u8F93\u5165\u53C2\u6570\u6821\u9A8C\u5931\u8D25\u3002",
        unauthenticated: "\u9700\u8981\u5148\u5B8C\u6210\u8EAB\u4EFD\u9A8C\u8BC1\u3002",
        forbidden: "\u5F53\u524D\u8D26\u53F7\u6CA1\u6709\u6267\u884C\u6B64\u64CD\u4F5C\u7684\u6743\u9650\u3002",
        not_found: "\u672A\u627E\u5230\u8BF7\u6C42\u7684\u8D44\u6E90\u3002",
        conflict: "\u8D44\u6E90\u5DF2\u53D1\u751F\u53D8\u5316\u6216\u5F53\u524D\u72B6\u6001\u4E0D\u5141\u8BB8\u6B64\u64CD\u4F5C\u3002",
        retry_later: "\u5F53\u524D\u6682\u65F6\u65E0\u6CD5\u5B8C\u6210\u64CD\u4F5C\uFF0C\u8BF7\u7A0D\u540E\u91CD\u8BD5\u3002",
        service_failure: "\u670D\u52A1\u6682\u65F6\u4E0D\u53EF\u7528\u3002",
        confirmation_required: "\u6B64\u64CD\u4F5C\u9700\u8981\u5148\u786E\u8BA4\u3002",
        operation_cancelled: "\u64CD\u4F5C\u5DF2\u53D6\u6D88\u3002",
        server_plan_required: "\u6B64\u9AD8\u98CE\u9669\u64CD\u4F5C\u9700\u8981\u670D\u52A1\u7AEF\u7B7E\u53D1\u6267\u884C\u8BA1\u5212\u3002"
      },
      table: {
        name: "\u540D\u79F0",
        status: "\u72B6\u6001",
        type: "\u7C7B\u578B",
        createdAt: "\u521B\u5EFA\u65F6\u95F4",
        updatedAt: "\u66F4\u65B0\u65F6\u95F4"
      }
    }
  }
};
var SUPPORTED_LOCALES = Object.freeze(Object.keys(resources));

// src/i18n/index.ts
async function createCliI18n(options = {}) {
  const instance = createInstance();
  await instance.init({
    lng: detectLocale(options),
    fallbackLng: options.fallbackLocale ?? "en-US",
    supportedLngs: Object.keys(resources),
    resources,
    interpolation: { escapeValue: false },
    returnNull: false
  });
  return instance;
}

// src/entry.ts
function createLunaCli(options = {}) {
  const version = options.version ?? process13.env.LUNA_CLI_VERSION ?? "0.1.0";
  const env = options.ports?.env ?? process13.env;
  const config = options.ports?.config ?? new FileConfigStore();
  const translate = options.ports?.translate;
  const output = options.ports?.output ?? new CommandOutput({ version, translate });
  const ports = {
    config,
    input: options.ports?.input ?? new DefaultInputPort(),
    output,
    api: options.ports?.api ?? new LunaApiAdapter({ config, env }),
    env,
    isTTY: options.ports?.isTTY ?? Boolean(process13.stdout.isTTY),
    version,
    distribution: options.distribution ?? options.ports?.distribution ?? runtimeDistribution(),
    translate
  };
  const registry = createRegistryFromContract(src_exports);
  registerLocalCommands(registry);
  const programOptions = {
    registry,
    ports,
    name: "luna",
    description: translate?.(
      "cli.description",
      "Luna DevOps command-line client for people and agents"
    ) ?? "Luna DevOps command-line client for people and agents"
  };
  return {
    program: createCliProgram(programOptions),
    registry,
    ports
  };
}
async function main(argv = process13.argv) {
  const i18n = await createCliI18n({ env: process13.env });
  const cli = createLunaCli({
    ports: {
      translate(key, fallback, locale) {
        return i18n.getFixedT(normalizeLocale(locale) ?? i18n.language)(key, {
          defaultValue: fallback
        });
      }
    }
  });
  const result = await runCli(cli.program, argv, cli.ports.output);
  process13.exitCode = result.exitCode;
  return result.exitCode;
}
function runtimeDistribution() {
  if (typeof process13.versions.bun === "string")
    return "binary";
  return process13.env.npm_package_name ? "npm" : "source";
}
if (isDirectExecution()) {
  void main();
}
function isDirectExecution() {
  const executable = process13.argv[1];
  return Boolean(executable && import.meta.url === pathToFileURL(executable).href);
}
export {
  createLunaCli,
  main
};
