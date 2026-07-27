// Generated from openapi/openapi.yaml. Do not edit manually.
import type { OpenApiOperationSnapshot, OpenApiSnapshotMetadata } from "../types.js";

export const OPENAPI_SNAPSHOT_METADATA = {
  "source": "openapi/openapi.yaml",
  "openapiVersion": "3.1.0",
  "apiVersion": "0.1.0",
  "sourceDigest": "sha256:54cb6ef4eb6fd0721c90ecdb109e9f76d424db01aa7b426b81392f75ae4bf09e",
  "operationCount": 116
} as const satisfies OpenApiSnapshotMetadata;

export const OPENAPI_OPERATION_SNAPSHOTS = [
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
    "method": "get",
    "path": "/.well-known/oauth-authorization-server",
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
        "schemaRefs": [],
        "description": "OAuth 2.0 authorization server metadata."
      }
    ],
    "summary": "Get OAuth authorization server metadata",
    "operationId": "getOAuthAuthorizationServerMetadata",
    "xLunaCli": {
      "classification": "protocol-adapter",
      "hidden": true,
      "exclusionReason": "OAuth discovery is consumed by the CLI authentication adapter."
    }
  },
  {
    "method": "post",
    "path": "/api/v1/oauth/device/authorization",
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
        "schemaRefs": [],
        "description": "Device and user codes for the browser verification flow."
      },
      {
        "status": "400",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/OAuthProtocolError"
        ],
        "description": "OAuth protocol error."
      }
    ],
    "summary": "Start an OAuth Device Authorization Grant",
    "operationId": "startOAuthDeviceAuthorization",
    "requestBody": {
      "required": true,
      "contentTypes": [
        "application/x-www-form-urlencoded"
      ],
      "schemaRefs": []
    },
    "inputSchema": {
      "type": "object",
      "properties": {
        "body": {
          "type": "object",
          "required": [
            "client_id"
          ],
          "properties": {
            "client_id": {
              "type": "string"
            },
            "scope": {
              "type": "string"
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
      "classification": "protocol-adapter",
      "hidden": true,
      "exclusionReason": "Device authorization is exposed through `luna login`."
    }
  },
  {
    "method": "get",
    "path": "/api/v1/oauth/device/verification",
    "tags": [
      "Auth"
    ],
    "deprecated": false,
    "security": [],
    "parameters": [
      {
        "name": "user_code",
        "in": "query",
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
          "#/components/schemas/OAuthApplication"
        ],
        "description": "Pending device authorization visible to the signed-in user."
      },
      {
        "status": "400",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/ErrorResponse"
        ],
        "description": "Invalid, expired, or consumed user code."
      }
    ],
    "summary": "Inspect a pending OAuth device authorization",
    "operationId": "getOAuthDeviceVerification",
    "inputSchema": {
      "type": "object",
      "properties": {
        "user_code": {
          "type": "string"
        }
      },
      "required": [
        "user_code"
      ],
      "additionalProperties": false
    },
    "xLunaCli": {
      "classification": "protocol-adapter",
      "hidden": true,
      "exclusionReason": "Browser-only Device Code verification endpoint."
    }
  },
  {
    "method": "post",
    "path": "/api/v1/oauth/device/verification",
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
        "schemaRefs": [],
        "description": "Device authorization decision accepted."
      },
      {
        "status": "400",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/ErrorResponse"
        ],
        "description": "Invalid, expired, or consumed user code."
      }
    ],
    "summary": "Approve or deny an OAuth device authorization",
    "operationId": "decideOAuthDeviceVerification",
    "requestBody": {
      "required": true,
      "contentTypes": [
        "application/json"
      ],
      "schemaRefs": []
    },
    "inputSchema": {
      "type": "object",
      "properties": {
        "body": {
          "type": "object",
          "required": [
            "decision",
            "userCode"
          ],
          "properties": {
            "decision": {
              "type": "string",
              "enum": [
                "approve",
                "deny"
              ]
            },
            "userCode": {
              "type": "string"
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
      "classification": "protocol-adapter",
      "hidden": true,
      "exclusionReason": "Browser-only Device Code verification endpoint."
    }
  },
  {
    "method": "post",
    "path": "/api/v1/oauth/token",
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
          "#/components/schemas/OAuthTokenResponse"
        ],
        "description": "OAuth access and refresh tokens."
      },
      {
        "status": "400",
        "contentTypes": [
          "application/json"
        ],
        "schemaRefs": [
          "#/components/schemas/OAuthProtocolError"
        ],
        "description": "OAuth protocol error."
      }
    ],
    "summary": "Exchange an OAuth authorization grant for tokens",
    "operationId": "exchangeOAuthToken",
    "requestBody": {
      "required": true,
      "contentTypes": [
        "application/x-www-form-urlencoded"
      ],
      "schemaRefs": []
    },
    "inputSchema": {
      "type": "object",
      "properties": {
        "body": {
          "type": "object",
          "required": [
            "grant_type"
          ],
          "properties": {
            "client_id": {
              "type": "string"
            },
            "client_secret": {
              "type": "string",
              "writeOnly": true
            },
            "code": {
              "type": "string"
            },
            "code_verifier": {
              "type": "string"
            },
            "device_code": {
              "type": "string",
              "writeOnly": true
            },
            "grant_type": {
              "type": "string",
              "enum": [
                "authorization_code",
                "refresh_token",
                "urn:ietf:params:oauth:grant-type:device_code"
              ]
            },
            "redirect_uri": {
              "type": "string",
              "format": "uri"
            },
            "refresh_token": {
              "type": "string",
              "writeOnly": true
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
      "classification": "protocol-adapter",
      "hidden": true,
      "exclusionReason": "OAuth token exchange is consumed by the CLI authentication adapter."
    }
  },
  {
    "method": "post",
    "path": "/api/v1/oauth/revoke",
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
        "description": "Token is revoked or was already invalid."
      }
    ],
    "summary": "Revoke an OAuth access or refresh token",
    "operationId": "revokeOAuthToken",
    "requestBody": {
      "required": true,
      "contentTypes": [
        "application/x-www-form-urlencoded"
      ],
      "schemaRefs": []
    },
    "inputSchema": {
      "type": "object",
      "properties": {
        "body": {
          "type": "object",
          "required": [
            "token"
          ],
          "properties": {
            "client_id": {
              "type": "string"
            },
            "client_secret": {
              "type": "string",
              "writeOnly": true
            },
            "token": {
              "type": "string",
              "writeOnly": true
            },
            "token_type_hint": {
              "type": "string",
              "enum": [
                "access_token",
                "refresh_token"
              ]
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
      "classification": "protocol-adapter",
      "hidden": true,
      "exclusionReason": "OAuth revocation is exposed through `luna logout`."
    }
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
] as const satisfies readonly OpenApiOperationSnapshot[];
