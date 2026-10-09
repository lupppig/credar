package httpserver

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lupppig/credar/backend/internal/platform/dependency"
	"github.com/lupppig/credar/backend/internal/platform/identifier"
	"github.com/lupppig/credar/backend/internal/platform/requestcontext"
)

const (
	readHeaderTimeout = 5 * time.Second
	readTimeout       = 15 * time.Second
	writeTimeout      = 15 * time.Second
	idleTimeout       = 60 * time.Second
)

// Readiness reports whether every required dependency can accept work.
type Readiness interface {
	Readiness(context.Context) dependency.ReadinessReport
}

// New returns the minimal HTTP server used to prove the backend build and
// runtime. Domain routes are added by their owning modules in later phases.
func New(address string, logger *slog.Logger, readiness Readiness) *http.Server {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.RedirectTrailingSlash = false
	router.RedirectFixedPath = false
	router.HandleMethodNotAllowed = true
	if err := router.SetTrustedProxies(nil); err != nil {
		panic("configure trusted proxies: " + err.Error())
	}
	router.Use(requestIdentifiers(), logRequests(logger), gin.Recovery())
	router.GET("/healthz", liveness)
	router.GET("/livez", liveness)
	router.GET("/readyz", readinessHandler(readiness))

	return &http.Server{
		Addr:              address,
		Handler:           router,
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
	}
}

func requestIdentifiers() gin.HandlerFunc {
	return func(context *gin.Context) {
		requestID := context.GetHeader("X-Request-ID")
		if !identifier.IsUUIDv7(requestID) {
			var err error
			requestID, err = identifier.NewUUIDv7()
			if err != nil {
				context.AbortWithStatusJSON(
					http.StatusInternalServerError,
					gin.H{"error": "could not create request identifier"},
				)
				return
			}
		}

		correlationID := context.GetHeader("X-Correlation-ID")
		if !identifier.IsUUIDv7(correlationID) {
			correlationID = requestID
		}

		context.Header("X-Request-ID", requestID)
		context.Header("X-Correlation-ID", correlationID)
		requestContext := requestcontext.WithIdentifiers(
			context.Request.Context(),
			requestID,
			correlationID,
		)
		context.Request = context.Request.WithContext(requestContext)
		context.Next()
	}
}

func liveness(context *gin.Context) {
	context.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func readinessHandler(readiness Readiness) gin.HandlerFunc {
	return func(context *gin.Context) {
		report := readiness.Readiness(context.Request.Context())
		checks := make([]gin.H, 0, len(report.Checks))
		for _, check := range report.Checks {
			status := "unavailable"
			if check.Ready {
				status = "ok"
			}
			checks = append(checks, gin.H{"name": check.Name, "status": status})
		}

		if !report.Ready {
			context.JSON(http.StatusServiceUnavailable, gin.H{
				"status": "unavailable",
				"checks": checks,
			})
			return
		}
		context.JSON(http.StatusOK, gin.H{"status": "ok", "checks": checks})
	}
}

func logRequests(logger *slog.Logger) gin.HandlerFunc {
	return func(context *gin.Context) {
		startedAt := time.Now()
		context.Next()

		logger.InfoContext(
			context.Request.Context(),
			"http request completed",
			"method", context.Request.Method,
			"path", context.Request.URL.Path,
			"status", context.Writer.Status(),
			"request_id", context.Writer.Header().Get("X-Request-ID"),
			"correlation_id", context.Writer.Header().Get("X-Correlation-ID"),
			"duration_ms", time.Since(startedAt).Milliseconds(),
		)
	}
}
