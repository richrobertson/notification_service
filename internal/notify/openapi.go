package notify

func openAPISpec() map[string]any {
	return map[string]any{
		"openapi": "3.0.3",
		"info": map[string]any{
			"title":       "Notification Service API",
			"version":     "0.1.0",
			"description": "MVP REST API for the multi-tenant notification platform.",
		},
		"servers": []map[string]any{
			{"url": "/"},
		},
		"paths": map[string]any{
			"/v1/health": map[string]any{
				"get": map[string]any{
					"summary":     "Health check",
					"operationId": "getHealth",
					"responses": map[string]any{
						"200": responseRef("Service health", "#/components/schemas/StatusResponse"),
					},
				},
			},
			"/v1/readiness": map[string]any{
				"get": map[string]any{
					"summary":     "Readiness check",
					"operationId": "getReadiness",
					"responses": map[string]any{
						"200": responseRef("Service readiness", "#/components/schemas/StatusResponse"),
					},
				},
			},
			"/v1/tenants": map[string]any{
				"post": map[string]any{
					"summary":     "Create tenant",
					"operationId": "createTenant",
					"requestBody": requestJSONBody("#/components/schemas/CreateTenantInput"),
					"responses": map[string]any{
						"201": responseRef("Tenant created", "#/components/schemas/Tenant"),
						"409": responseRef("Tenant already exists", "#/components/schemas/ErrorResponse"),
					},
				},
			},
			"/v1/tenants/{tenantId}": map[string]any{
				"get": map[string]any{
					"summary":     "Get tenant",
					"operationId": "getTenant",
					"parameters": []map[string]any{
						pathParameter("tenantId", "Tenant identifier"),
					},
					"responses": map[string]any{
						"200": responseRef("Tenant record", "#/components/schemas/Tenant"),
						"404": responseRef("Tenant not found", "#/components/schemas/ErrorResponse"),
					},
				},
			},
			"/v1/tenants/{tenantId}/usage": map[string]any{
				"get": map[string]any{
					"summary":     "Get tenant usage",
					"operationId": "getTenantUsage",
					"parameters": []map[string]any{
						pathParameter("tenantId", "Tenant identifier"),
					},
					"responses": map[string]any{
						"200": responseRef("Tenant usage snapshot", "#/components/schemas/Usage"),
						"404": responseRef("Tenant not found", "#/components/schemas/ErrorResponse"),
					},
				},
			},
			"/v1/templates": map[string]any{
				"post": map[string]any{
					"summary":     "Create template",
					"operationId": "createTemplate",
					"requestBody": requestJSONBody("#/components/schemas/CreateTemplateInput"),
					"responses": map[string]any{
						"201": responseRef("Template created", "#/components/schemas/Template"),
						"404": responseRef("Tenant not found", "#/components/schemas/ErrorResponse"),
						"409": responseRef("Template already exists", "#/components/schemas/ErrorResponse"),
					},
				},
			},
			"/v1/templates/{templateId}": map[string]any{
				"get": map[string]any{
					"summary":     "Get template",
					"operationId": "getTemplate",
					"parameters": []map[string]any{
						pathParameter("templateId", "Template identifier"),
					},
					"responses": map[string]any{
						"200": responseRef("Template record", "#/components/schemas/Template"),
						"404": responseRef("Template not found", "#/components/schemas/ErrorResponse"),
					},
				},
				"put": map[string]any{
					"summary":     "Update template",
					"operationId": "updateTemplate",
					"parameters": []map[string]any{
						pathParameter("templateId", "Template identifier"),
					},
					"requestBody": requestJSONBody("#/components/schemas/UpdateTemplateInput"),
					"responses": map[string]any{
						"200": responseRef("Updated template", "#/components/schemas/Template"),
						"404": responseRef("Template not found", "#/components/schemas/ErrorResponse"),
					},
				},
			},
			"/v1/notifications": map[string]any{
				"post": map[string]any{
					"summary":     "Submit notification",
					"operationId": "createNotification",
					"requestBody": requestJSONBody("#/components/schemas/CreateNotificationInput"),
					"responses": map[string]any{
						"200": responseRef("Existing notification returned for duplicate idempotency key", "#/components/schemas/Notification"),
						"202": responseRef("Notification accepted", "#/components/schemas/Notification"),
						"404": responseRef("Tenant or template not found", "#/components/schemas/ErrorResponse"),
						"429": responseRef("Tenant quota exceeded", "#/components/schemas/ErrorResponse"),
					},
				},
			},
			"/v1/notifications/{notificationId}": map[string]any{
				"get": map[string]any{
					"summary":     "Get notification",
					"operationId": "getNotification",
					"parameters": []map[string]any{
						pathParameter("notificationId", "Notification identifier"),
					},
					"responses": map[string]any{
						"200": responseRef("Notification record", "#/components/schemas/Notification"),
						"404": responseRef("Notification not found", "#/components/schemas/ErrorResponse"),
					},
				},
			},
			"/v1/notifications/{notificationId}/replay": map[string]any{
				"post": map[string]any{
					"summary":     "Replay notification",
					"operationId": "replayNotification",
					"parameters": []map[string]any{
						pathParameter("notificationId", "Notification identifier"),
					},
					"responses": map[string]any{
						"200": responseRef("Notification replayed", "#/components/schemas/Notification"),
						"404": responseRef("Notification not found", "#/components/schemas/ErrorResponse"),
					},
				},
			},
			"/v1/dead-letters": map[string]any{
				"get": map[string]any{
					"summary":     "List dead letters",
					"operationId": "listDeadLetters",
					"responses": map[string]any{
						"200": map[string]any{
							"description": "Dead-letter records",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": map[string]any{
										"type": "array",
										"items": map[string]any{
											"$ref": "#/components/schemas/DeadLetter",
										},
									},
								},
							},
						},
					},
				},
			},
		},
		"components": map[string]any{
			"schemas": map[string]any{
				"StatusResponse": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"status": map[string]any{"type": "string"},
					},
					"required": []string{"status"},
				},
				"ErrorResponse": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"error": map[string]any{"type": "string"},
					},
					"required": []string{"error"},
				},
				"Tenant": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"id":          map[string]any{"type": "string"},
						"name":        map[string]any{"type": "string"},
						"status":      map[string]any{"type": "string"},
						"daily_quota": map[string]any{"type": "integer"},
						"created_at":  map[string]any{"type": "string", "format": "date-time"},
					},
					"required": []string{"id", "name", "status", "daily_quota", "created_at"},
				},
				"CreateTenantInput": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"id":          map[string]any{"type": "string"},
						"name":        map[string]any{"type": "string"},
						"daily_quota": map[string]any{"type": "integer", "minimum": 1},
					},
					"required": []string{"id", "name", "daily_quota"},
				},
				"Template": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"id":         map[string]any{"type": "string"},
						"tenant_id":  map[string]any{"type": "string"},
						"name":       map[string]any{"type": "string"},
						"channel":    map[string]any{"type": "string", "enum": []string{"email", "webhook"}},
						"version":    map[string]any{"type": "integer"},
						"body":       map[string]any{"type": "string"},
						"created_at": map[string]any{"type": "string", "format": "date-time"},
					},
					"required": []string{"id", "tenant_id", "name", "channel", "version", "body", "created_at"},
				},
				"CreateTemplateInput": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"id":        map[string]any{"type": "string"},
						"tenant_id": map[string]any{"type": "string"},
						"name":      map[string]any{"type": "string"},
						"channel":   map[string]any{"type": "string", "enum": []string{"email", "webhook"}},
						"body":      map[string]any{"type": "string"},
					},
					"required": []string{"id", "tenant_id", "name", "channel", "body"},
				},
				"UpdateTemplateInput": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"name": map[string]any{"type": "string"},
						"body": map[string]any{"type": "string"},
					},
					"required": []string{"name", "body"},
				},
				"DeliveryAttempt": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"id":              map[string]any{"type": "string"},
						"notification_id": map[string]any{"type": "string"},
						"channel":         map[string]any{"type": "string", "enum": []string{"email", "webhook"}},
						"attempt_number":  map[string]any{"type": "integer"},
						"status":          map[string]any{"type": "string"},
						"error_code":      map[string]any{"type": "string", "nullable": true},
						"next_retry_at":   map[string]any{"type": "string", "format": "date-time", "nullable": true},
						"completed_at":    map[string]any{"type": "string", "format": "date-time", "nullable": true},
					},
					"required": []string{"id", "notification_id", "channel", "attempt_number", "status"},
				},
				"Notification": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"id":          map[string]any{"type": "string"},
						"tenant_id":   map[string]any{"type": "string"},
						"template_id": map[string]any{"type": "string"},
						"channels":    map[string]any{"type": "array", "items": map[string]any{"type": "string", "enum": []string{"email", "webhook"}}},
						"recipient": map[string]any{
							"type":                 "object",
							"additionalProperties": map[string]any{},
						},
						"variables": map[string]any{
							"type":                 "object",
							"additionalProperties": map[string]any{},
						},
						"idempotency_key": map[string]any{"type": "string"},
						"status":          map[string]any{"type": "string"},
						"submitted_at":    map[string]any{"type": "string", "format": "date-time"},
						"attempts": map[string]any{
							"type": "array",
							"items": map[string]any{
								"$ref": "#/components/schemas/DeliveryAttempt",
							},
						},
					},
					"required": []string{"id", "tenant_id", "template_id", "channels", "recipient", "variables", "idempotency_key", "status", "submitted_at", "attempts"},
				},
				"CreateNotificationInput": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"tenant_id":   map[string]any{"type": "string"},
						"template_id": map[string]any{"type": "string"},
						"channels":    map[string]any{"type": "array", "items": map[string]any{"type": "string", "enum": []string{"email", "webhook"}}, "minItems": 1},
						"recipient": map[string]any{
							"type":                 "object",
							"additionalProperties": map[string]any{},
						},
						"variables": map[string]any{
							"type":                 "object",
							"additionalProperties": map[string]any{},
						},
						"idempotency_key": map[string]any{"type": "string"},
					},
					"required": []string{"tenant_id", "template_id", "channels", "recipient", "idempotency_key"},
				},
				"Usage": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"tenant_id":              map[string]any{"type": "string"},
						"date":                   map[string]any{"type": "string", "format": "date"},
						"accepted_notifications": map[string]any{"type": "integer"},
						"daily_quota":            map[string]any{"type": "integer"},
						"remaining_quota":        map[string]any{"type": "integer"},
					},
					"required": []string{"tenant_id", "date", "accepted_notifications", "daily_quota", "remaining_quota"},
				},
				"DeadLetter": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"id":               map[string]any{"type": "string"},
						"notification_id":  map[string]any{"type": "string"},
						"channel":          map[string]any{"type": "string", "enum": []string{"email", "webhook"}},
						"final_error":      map[string]any{"type": "string"},
						"dead_lettered_at": map[string]any{"type": "string", "format": "date-time"},
					},
					"required": []string{"id", "notification_id", "channel", "final_error", "dead_lettered_at"},
				},
			},
		},
	}
}

func requestJSONBody(schemaRef string) map[string]any {
	return map[string]any{
		"required": true,
		"content": map[string]any{
			"application/json": map[string]any{
				"schema": map[string]any{
					"$ref": schemaRef,
				},
			},
		},
	}
}

func responseRef(description, schemaRef string) map[string]any {
	return map[string]any{
		"description": description,
		"content": map[string]any{
			"application/json": map[string]any{
				"schema": map[string]any{
					"$ref": schemaRef,
				},
			},
		},
	}
}

func pathParameter(name, description string) map[string]any {
	return map[string]any{
		"name":        name,
		"in":          "path",
		"required":    true,
		"description": description,
		"schema": map[string]any{
			"type": "string",
		},
	}
}
