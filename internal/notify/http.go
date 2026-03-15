package notify

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

type Server struct {
	service *Service
	mux     *http.ServeMux
}

func NewServer(service *Service) *Server {
	server := &Server{
		service: service,
		mux:     http.NewServeMux(),
	}
	server.routes()
	return server
}

func (s *Server) Handler() http.Handler {
	return s.mux
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /", s.handleRoot)
	s.mux.HandleFunc("GET /swagger", s.handleSwaggerUI)
	s.mux.HandleFunc("GET /openapi.json", s.handleOpenAPI)
	s.mux.HandleFunc("GET /v1/health", s.handleHealth)
	s.mux.HandleFunc("GET /v1/readiness", s.handleReadiness)
	s.mux.HandleFunc("POST /v1/tenants", s.handleCreateTenant)
	s.mux.HandleFunc("GET /v1/tenants/", s.handleTenantRoutes)
	s.mux.HandleFunc("POST /v1/templates", s.handleCreateTemplate)
	s.mux.HandleFunc("GET /v1/templates/", s.handleGetTemplate)
	s.mux.HandleFunc("PUT /v1/templates/", s.handleUpdateTemplate)
	s.mux.HandleFunc("POST /v1/notifications", s.handleCreateNotification)
	s.mux.HandleFunc("GET /v1/notifications/", s.handleNotificationRoutes)
	s.mux.HandleFunc("POST /v1/notifications/", s.handleNotificationRoutes)
	s.mux.HandleFunc("GET /v1/dead-letters", s.handleDeadLetters)
}

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/swagger", http.StatusTemporaryRedirect)
}

func (s *Server) handleSwaggerUI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(swaggerUIHTML))
}

func (s *Server) handleOpenAPI(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, openAPISpec())
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleReadiness(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (s *Server) handleCreateTenant(w http.ResponseWriter, r *http.Request) {
	var input CreateTenantInput
	if err := decodeJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if input.ID == "" || input.Name == "" || input.DailyQuota <= 0 {
		writeError(w, http.StatusBadRequest, "id, name, and positive daily_quota are required")
		return
	}

	tenant, err := s.service.CreateTenant(input)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, errTenantExists) {
			status = http.StatusConflict
		}
		writeError(w, status, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, tenant)
}

func (s *Server) handleTenantRoutes(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/v1/tenants/")
	if path == "" {
		http.NotFound(w, r)
		return
	}
	if strings.HasSuffix(path, "/usage") {
		tenantID := strings.TrimSuffix(path, "/usage")
		s.handleUsage(w, r, strings.TrimSuffix(tenantID, "/"))
		return
	}
	s.handleGetTenant(w, r, path)
}

func (s *Server) handleGetTenant(w http.ResponseWriter, r *http.Request, tenantID string) {
	tenant, err := s.service.GetTenant(tenantID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, tenant)
}

func (s *Server) handleUsage(w http.ResponseWriter, _ *http.Request, tenantID string) {
	usage, err := s.service.Usage(tenantID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, usage)
}

func (s *Server) handleCreateTemplate(w http.ResponseWriter, r *http.Request) {
	var input CreateTemplateInput
	if err := decodeJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if input.ID == "" || input.TenantID == "" || input.Name == "" || input.Channel == "" || input.Body == "" {
		writeError(w, http.StatusBadRequest, "id, tenant_id, name, channel, and body are required")
		return
	}

	template, err := s.service.CreateTemplate(input)
	if err != nil {
		switch {
		case errors.Is(err, errTenantNotFound):
			writeError(w, http.StatusNotFound, err.Error())
		case errors.Is(err, errTemplateExists):
			writeError(w, http.StatusConflict, err.Error())
		default:
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	writeJSON(w, http.StatusCreated, template)
}

func (s *Server) handleGetTemplate(w http.ResponseWriter, r *http.Request) {
	templateID := strings.TrimPrefix(r.URL.Path, "/v1/templates/")
	template, err := s.service.GetTemplate(templateID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, template)
}

func (s *Server) handleUpdateTemplate(w http.ResponseWriter, r *http.Request) {
	templateID := strings.TrimPrefix(r.URL.Path, "/v1/templates/")
	var input UpdateTemplateInput
	if err := decodeJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if input.Name == "" || input.Body == "" {
		writeError(w, http.StatusBadRequest, "name and body are required")
		return
	}

	template, err := s.service.UpdateTemplate(templateID, input)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, template)
}

func (s *Server) handleCreateNotification(w http.ResponseWriter, r *http.Request) {
	var input CreateNotificationInput
	if err := decodeJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if input.TenantID == "" || input.TemplateID == "" || input.IdempotencyKey == "" || len(input.Channels) == 0 || len(input.Recipient) == 0 {
		writeError(w, http.StatusBadRequest, "tenant_id, template_id, channels, recipient, and idempotency_key are required")
		return
	}

	notification, duplicate, err := s.service.CreateNotification(input)
	if err != nil {
		switch {
		case errors.Is(err, errTenantNotFound), errors.Is(err, errTemplateNotFound):
			writeError(w, http.StatusNotFound, err.Error())
		case errors.Is(err, errDailyQuotaExceeded):
			writeError(w, http.StatusTooManyRequests, err.Error())
		default:
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	status := http.StatusAccepted
	if duplicate {
		status = http.StatusOK
	}
	writeJSON(w, status, notification)
}

func (s *Server) handleNotificationRoutes(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/v1/notifications/")
	if path == "" {
		http.NotFound(w, r)
		return
	}
	if strings.HasSuffix(path, "/replay") {
		id := strings.TrimSuffix(path, "/replay")
		s.handleReplayNotification(w, r, strings.TrimSuffix(id, "/"))
		return
	}
	s.handleGetNotification(w, r, path)
}

func (s *Server) handleGetNotification(w http.ResponseWriter, _ *http.Request, notificationID string) {
	notification, err := s.service.GetNotification(notificationID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, notification)
}

func (s *Server) handleReplayNotification(w http.ResponseWriter, _ *http.Request, notificationID string) {
	notification, err := s.service.ReplayNotification(notificationID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, notification)
}

func (s *Server) handleDeadLetters(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.service.DeadLetters())
}

func decodeJSON(r *http.Request, dst any) error {
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	return decoder.Decode(dst)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func newID() string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(buf[:])
}

const swaggerUIHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Notification Service API Docs</title>
  <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css">
  <style>
    body { margin: 0; background: #f5f7f4; }
    .topbar { display: none; }
  </style>
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
  <script>
    window.ui = SwaggerUIBundle({
      url: '/openapi.json',
      dom_id: '#swagger-ui',
      deepLinking: true,
      displayRequestDuration: true,
      defaultModelsExpandDepth: 1,
      docExpansion: 'list'
    });
  </script>
</body>
</html>`
