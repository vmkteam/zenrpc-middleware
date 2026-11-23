//nolint:unparam // tests
package middleware_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"os"
	"sync"
	"testing"

	"github.com/vmkteam/zenrpc-middleware"

	"github.com/getsentry/sentry-go"
	"github.com/go-pg/pg/v10"
	"github.com/vmkteam/appkit"
	"github.com/vmkteam/zenrpc/v2"
	"github.com/vmkteam/zenrpc/v2/smd"
	"github.com/vmkteam/zenrpc/v2/testdata"
)

func newArithServer(isDevel bool, dbc *pg.DB, appName string) *zenrpc.Server {
	elog := log.New(os.Stderr, "E", log.LstdFlags|log.Lshortfile)
	dlog := log.New(os.Stdout, "D", log.LstdFlags|log.Lshortfile)
	sl := slog.New(slog.NewTextHandler(os.Stdout, nil))

	allowDebugFn := func(param string) middleware.AllowDebugFunc {
		return func(req *http.Request) bool {
			return req.FormValue(param) == "true"
		}
	}

	rpc := zenrpc.NewServer(zenrpc.Options{
		ExposeSMD: true,
		AllowCORS: true,
	})

	fnLogAttr := func(ctx context.Context, r zenrpc.Response) []any {
		namespace, method := zenrpc.NamespaceFromContext(ctx), appkit.MethodFromContext(ctx)
		if namespace == "arith" && method == "divide" {
			return []any{middleware.ErrSkipLog}
		}

		return []any{"apiKey", "fromCtx"}
	}

	rpc.Use(
		middleware.WithDevel(isDevel),
		middleware.WithHeaders(),
		middleware.WithTiming(isDevel, allowDebugFn("d")),
		middleware.WithAPILogger(dlog.Printf, appName),
		middleware.WithSentry(appName),
		middleware.WithNoCancelContext(),
		middleware.WithMetrics(appName),
		middleware.WithErrorSLog(sl.ErrorContext, appName, fnLogAttr),
		middleware.WithErrorLogger(elog.Printf, appName),
		middleware.WithSLog(sl.InfoContext, appName, fnLogAttr),
	)

	if dbc != nil {
		rpc.Use(middleware.WithSQLLogger(dbc, isDevel, allowDebugFn("d"), allowDebugFn("s")))
	}

	arith := testdata.ArithService{}
	rpc.Register("arith", arith)

	return rpc
}

func TestMiddlewareDevel(t *testing.T) {
	rpc := newArithServer(true, nil, middleware.DefaultServerName)

	ts := httptest.NewServer(http.HandlerFunc(rpc.ServeHTTP))
	defer ts.Close()

	in := `{"jsonrpc": "2.0", "method": "arith.divide", "params": { "a": 1, "b": 24 }, "id": 1 }`
	out := `{"jsonrpc":"2.0","id":1,"result":{"Quo":0,"rem":1},"extensions":{"DurationLocal":0}}`

	res, err := http.Post(ts.URL, "application/json", bytes.NewBufferString(in))
	if err != nil {
		t.Error(err)
	}

	resp, err := io.ReadAll(res.Body)
	_ = res.Body.Close()
	if err != nil {
		t.Error(err)
	}

	if string(resp) != out {
		t.Errorf("Input: %s\n got %s expected %s", in, resp, out)
	}
}

func TestMiddlewareNoDevel(t *testing.T) {
	rpc := newArithServer(false, nil, "nodevel")

	ts := httptest.NewServer(http.HandlerFunc(rpc.ServeHTTP))
	defer ts.Close()

	in := `{"jsonrpc": "2.0", "method": "arith.divide", "params": { "a": 1, "b": 24 }, "id": 1 }`
	out := `{"jsonrpc":"2.0","id":1,"result":{"Quo":0,"rem":1}}`

	res, err := http.Post(ts.URL, "application/json", bytes.NewBufferString(in))
	if err != nil {
		t.Error(err)
	}

	resp, err := io.ReadAll(res.Body)
	_ = res.Body.Close()
	if err != nil {
		t.Error(err)
	}

	if string(resp) != out {
		t.Errorf("Input: %s\n got %s expected %s", in, resp, out)
	}
}

func TestMiddlewareErrorLogger(t *testing.T) {
	rpc := newArithServer(false, nil, "errordevel")

	ts := httptest.NewServer(http.HandlerFunc(rpc.ServeHTTP))
	defer ts.Close()

	in := `{"jsonrpc": "2.0", "method": "arith.checkzenrpcerror", "id": 0, "params": [ true ] }`
	out := `{"jsonrpc":"2.0","id":0,"error":{"code":500,"message":"Internal error"}}`

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, ts.URL, bytes.NewBufferString(in))
	if err != nil {
		t.Errorf("create request failed: %v", err)
	}

	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Platform", "Test1")
	req.Header.Add("Version", "v1.0.0 alpha1")

	c := http.Client{}
	res, err := c.Do(req)
	if err != nil {
		t.Error(err)
	}

	resp, err := io.ReadAll(res.Body)
	_ = res.Body.Close()
	if err != nil {
		t.Error(err)
	}

	if string(resp) != out {
		t.Errorf("Input: %s\n got %s expected %s", in, resp, out)
	}
}

func TestMiddlewareXRequestID(t *testing.T) {
	rpc := newArithServer(true, nil, "xrequestid")

	ts := httptest.NewServer(appkit.XRequestID(http.HandlerFunc(rpc.ServeHTTP)))
	defer ts.Close()

	in := `{"jsonrpc": "2.0", "method": "arith.divide", "params": { "a": 1, "b": 24 }, "id": 1 }`

	res, err := http.Post(ts.URL, "application/json", bytes.NewBufferString(in))
	if err != nil {
		t.Error(err)
	}
	_ = res.Body.Close()

	xRequestID := res.Header.Get("X-Request-Id")
	if xRequestID == "" {
		t.Error("Empty X-Request-ID header response")
	}

	bb, _ := httputil.DumpResponse(res, true)
	log.Println(string(bb))
}

// panicService panics in every method: zenrpc's testdata has no method that does.
type panicService struct{}

func (panicService) SMD() smd.ServiceInfo {
	return smd.ServiceInfo{Methods: map[string]smd.Service{"boom": {}}}
}

func (panicService) Invoke(context.Context, string, json.RawMessage) zenrpc.Response {
	panic("boom")
}

// A panic in a handler is recovered by WithSentry and answered with a JSON-RPC
// error. It used to come back as {"jsonrpc":"","id":0}, which no client reads
// as an error.
func TestMiddlewareSentryPanic(t *testing.T) {
	rpc := newArithServer(false, nil, "panic")
	rpc.Register("panic", panicService{})

	ts := httptest.NewServer(http.HandlerFunc(rpc.ServeHTTP))
	defer ts.Close()

	in := `{"jsonrpc": "2.0", "method": "panic.boom", "id": 0 }`
	out := `{"jsonrpc":"2.0","id":0,"error":{"code":-32603,"message":"Internal error"}}`

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, ts.URL, bytes.NewBufferString(in))
	if err != nil {
		t.Fatalf("create request failed: %v", err)
	}
	req.Header.Add("Content-Type", "application/json")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := io.ReadAll(res.Body)
	_ = res.Body.Close()
	if err != nil {
		t.Fatal(err)
	}

	if string(resp) != out {
		t.Errorf("Input: %s\n got %s expected %s", in, resp, out)
	}
}

// An error is reported with the call's details whether or not the request
// context carries a Sentry hub. Without one the logger falls back to the current
// hub, and the details used to go into a scope nobody captured with.
func TestMiddlewareErrorLoggerSentryEvent(t *testing.T) {
	var (
		mu     sync.Mutex
		events []*sentry.Event
	)
	client, err := sentry.NewClient(sentry.ClientOptions{BeforeSend: func(e *sentry.Event, _ *sentry.EventHint) *sentry.Event {
		mu.Lock()
		defer mu.Unlock()
		events = append(events, e)
		return nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	current := sentry.CurrentHub()
	prev := current.Client()
	current.BindClient(client)
	t.Cleanup(func() { current.BindClient(prev) })

	// withHub puts a request hub into the context, as appkit's echo middleware does.
	withHub := func(h zenrpc.InvokeFunc) zenrpc.InvokeFunc {
		return func(ctx context.Context, method string, params json.RawMessage) zenrpc.Response {
			return h(sentry.SetHubOnContext(ctx, sentry.NewHub(client, sentry.NewScope())), method, params)
		}
	}

	for name, chain := range map[string][]zenrpc.MiddlewareFunc{
		"current hub": {middleware.WithErrorLogger(func(string, ...any) {}, "app")},
		"request hub": {withHub, middleware.WithErrorLogger(func(string, ...any) {}, "app")},
	} {
		t.Run(name, func(t *testing.T) {
			mu.Lock()
			events = nil
			mu.Unlock()

			rpc := zenrpc.NewServer(zenrpc.Options{})
			rpc.Use(chain...)
			rpc.Register("arith", testdata.ArithService{})
			ts := httptest.NewServer(http.HandlerFunc(rpc.ServeHTTP))
			defer ts.Close()

			in := `{"jsonrpc": "2.0", "method": "arith.checkzenrpcerror", "id": 0, "params": [ true ] }`
			req, reqErr := http.NewRequestWithContext(t.Context(), http.MethodPost, ts.URL, bytes.NewBufferString(in))
			if reqErr != nil {
				t.Fatal(reqErr)
			}
			req.Header.Add("Content-Type", "application/json")
			res, reqErr := http.DefaultClient.Do(req)
			if reqErr != nil {
				t.Fatal(reqErr)
			}
			_ = res.Body.Close()

			mu.Lock()
			defer mu.Unlock()
			if len(events) != 1 {
				t.Fatalf("got %d events, expected 1", len(events))
			}
			rpcCtx, ok := events[0].Contexts["rpc"]
			if !ok {
				t.Fatalf("the event has no rpc context: %v", events[0].Contexts)
			}
			if rpcCtx["error.code"] != 500 {
				t.Errorf("rpc context error.code = %v, expected 500", rpcCtx["error.code"])
			}
			if got := events[0].Tags["method"]; got != "app.arith.checkzenrpcerror" {
				t.Errorf("tag method = %q, expected app.arith.checkzenrpcerror", got)
			}
		})
	}
}
