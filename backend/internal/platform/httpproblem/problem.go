// Package httpproblem writes the shared RFC 9457-compatible API error shape.
package httpproblem

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/lupppig/credar/backend/internal/platform/requestcontext"
)

const ContentType = "application/problem+json"

type Problem struct {
	Type          string `json:"type"`
	Title         string `json:"title"`
	Status        int    `json:"status"`
	Detail        string `json:"detail,omitempty"`
	Instance      string `json:"instance,omitempty"`
	Code          string `json:"code"`
	RequestID     string `json:"request_id"`
	CorrelationID string `json:"correlation_id"`
}

func Write(context *gin.Context, status int, code string, title string, detail string) {
	requestContext := context.Request.Context()
	context.Header("Content-Type", ContentType)
	context.AbortWithStatusJSON(status, Problem{
		Type:          "/problems/" + code,
		Title:         title,
		Status:        status,
		Detail:        detail,
		Instance:      context.Request.URL.Path,
		Code:          code,
		RequestID:     requestcontext.RequestID(requestContext),
		CorrelationID: requestcontext.CorrelationID(requestContext),
	})
}

func InternalServerError(context *gin.Context) {
	Write(
		context,
		http.StatusInternalServerError,
		"internal_server_error",
		"Internal server error",
		"The request could not be completed.",
	)
}
