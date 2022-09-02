package middleware

import (
	"net/http"

	sentryecho "github.com/getsentry/sentry-go/echo"
	"github.com/labstack/echo/v4"
)

// CORS allows certain CORS headers.
func CORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Authorization2, Origin, X-Requested-With, Content-Type, Accept, Platform, Version, X-Request-ID")
		if r.Method == "OPTIONS" {
			return
		}

		next.ServeHTTP(w, r)
	})
}

// XRequestID add X-Request-ID header if not exists
func XRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestId := r.Header.Get(echo.HeaderXRequestID)
		if !isValidXRequestID(requestId) {
			requestId = generateXRequestID()
			r.Header.Add(echo.HeaderXRequestID, requestId)
		}
		w.Header().Set(echo.HeaderXRequestID, requestId)

		next.ServeHTTP(w, r)
	})
}

// EchoHandler is wrapper for Echo.
func EchoHandler(next http.Handler) echo.HandlerFunc {
	return func(ctx echo.Context) error {
		req := ctx.Request()
		if hub := sentryecho.GetHubFromContext(ctx); hub != nil {
			req = ctx.Request().WithContext(NewSentryHubContext(ctx.Request().Context(), hub))
		}
		req = req.WithContext(NewIPContext(req.Context(), ctx.RealIP()))
		CORS(XRequestID(next)).ServeHTTP(ctx.Response(), req)
		return nil
	}
}
