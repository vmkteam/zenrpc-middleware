//nolint:unparam,noctx,goconst // tests
package middleware_test

import (
	"bytes"
	"context"
	"io"
	"log"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"os"
	"testing"

	"github.com/go-pg/pg/v10"
	"github.com/labstack/echo/v4"
	"github.com/vmkteam/zenrpc-middleware"
	"github.com/vmkteam/zenrpc/v2"
	"github.com/vmkteam/zenrpc/v2/testdata"
)

func newArithServer(isDevel bool, dbc *pg.DB, appName string) zenrpc.Server {
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
		namespace, method := zenrpc.NamespaceFromContext(ctx), middleware.MethodFromContext(ctx)
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

	ts := httptest.NewServer(middleware.XRequestID(http.HandlerFunc(rpc.ServeHTTP)))
	defer ts.Close()

	in := `{"jsonrpc": "2.0", "method": "arith.divide", "params": { "a": 1, "b": 24 }, "id": 1 }`

	res, err := http.Post(ts.URL, "application/json", bytes.NewBufferString(in))
	if err != nil {
		t.Error(err)
	}
	_ = res.Body.Close()

	xRequestID := res.Header.Get(echo.HeaderXRequestID)
	if xRequestID == "" {
		t.Error("Empty X-Request-ID header response")
	}

	bb, _ := httputil.DumpResponse(res, true)
	log.Println(string(bb))
}
