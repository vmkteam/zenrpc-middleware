package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/vmkteam/zenrpc/v2"
)

var ErrSkipLog = errors.New("skip log")

// WithAPILogger logs via Printf function (e.g. log.Printf) all requests.
func WithAPILogger(pf Printf, serverName string) zenrpc.MiddlewareFunc {
	return func(h zenrpc.InvokeFunc) zenrpc.InvokeFunc {
		return func(ctx context.Context, method string, params json.RawMessage) zenrpc.Response {
			start := time.Now()
			r := h(ctx, method, params)

			methodName := fullMethodName(serverName, zenrpc.NamespaceFromContext(ctx), method)
			pf("ip=%s platform=%q version=%q method=%s duration=%v params=%q err=%q userAgent=%q country=%q xRequestId=%q",
				IPFromContext(ctx),
				PlatformFromContext(ctx),
				VersionFromContext(ctx),
				methodName,
				time.Since(start),
				params,
				r.Error,
				UserAgentFromContext(ctx),
				CountryFromContext(ctx),
				XRequestIDFromContext(ctx),
			)

			return r
		}
	}
}

// WithSLog logs via slog function (e.g. slog.InfoContext) all requests.
func WithSLog(pf Print, serverName string, fn LogAttrs) zenrpc.MiddlewareFunc {
	return func(h zenrpc.InvokeFunc) zenrpc.InvokeFunc {
		return func(ctx context.Context, method string, params json.RawMessage) zenrpc.Response {
			start := time.Now()
			r := h(ctx, method, params)

			// get additional args, check for ErrSkipLog
			var args []any
			if fn != nil {
				args = fn(ctx, method, r)
				if len(args) == 1 && errors.Is(ErrSkipLog, args[0].(error)) {
					return r
				}
			}

			methodName := fullMethodName(serverName, zenrpc.NamespaceFromContext(ctx), method)
			pf(ctx, "rpc",
				append([]any{
					"ip", IPFromContext(ctx),
					"platform", PlatformFromContext(ctx),
					"version", VersionFromContext(ctx),
					"method", methodName,
					"duration", time.Since(start),
					"params", params,
					"err", r.Error,
					"userAgent", UserAgentFromContext(ctx),
					"country", CountryFromContext(ctx),
					"xRequestId", XRequestIDFromContext(ctx),
				}, args...)...,
			)

			return r
		}
	}
}
