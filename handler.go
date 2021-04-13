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
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Authorization2, Origin, X-Requested-With, Content-Type, Accept, Platform, Version")
		if r.Method == "OPTIONS" {
			return
		}

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
		CORS(next).ServeHTTP(ctx.Response(), req)
		return nil
	}
}
