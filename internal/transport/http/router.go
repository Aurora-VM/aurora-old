package http

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
)

// ResponseEnvelope is the standard JSON envelope for all Aurora API responses.
type ResponseEnvelope struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   interface{} `json:"error,omitempty"`
	Meta    MetaInfo    `json:"meta"`
}

// MetaInfo encapsulates request tracing metadata.
type MetaInfo struct {
	RequestID string    `json:"requestId"`
	Timestamp time.Time `json:"timestamp"`
}

// NewRouter constructs a configured Chi router with production middlewares.
func NewRouter() *chi.Mux {
	r := chi.NewRouter()

	// Middlewares
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))

	// CORS configuration
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Request-ID", "X-API-Key", "Idempotency-Key"},
		ExposedHeaders:   []string{"Link", "X-Request-ID"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	return r
}

// RespondJSON sends a formatted JSON response envelope.
func RespondJSON(w http.ResponseWriter, r *http.Request, status int, data interface{}) {
	reqID := middleware.GetReqID(r.Context())
	if reqID == "" {
		reqID = "unknown"
	}

	envelope := ResponseEnvelope{
		Success: status >= 200 && status < 300,
		Data:    data,
		Meta: MetaInfo{
			RequestID: reqID,
			Timestamp: time.Now().UTC(),
		},
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(envelope)
}

// RespondError sends an error JSON response envelope.
func RespondError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	reqID := middleware.GetReqID(r.Context())
	if reqID == "" {
		reqID = "unknown"
	}

	envelope := ResponseEnvelope{
		Success: false,
		Error: map[string]string{
			"code":    code,
			"message": message,
		},
		Meta: MetaInfo{
			RequestID: reqID,
			Timestamp: time.Now().UTC(),
		},
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(envelope)
}
