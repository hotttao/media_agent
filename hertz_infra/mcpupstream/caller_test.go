package mcpupstream

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

type callerRoundTripper func(*http.Request) (*http.Response, error)

func (f callerRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestCallerRemovedUnlessProvidedByIdentityAuthority(t *testing.T) {
	origin, _ := url.Parse("https://business.example/mcp")
	want := ""
	calls := 0
	transport := mcpHTTPTransport{origin: origin, headers: map[string]string{"X-Platform-Caller": "forged-config"}, transport: callerRoundTripper(func(r *http.Request) (*http.Response, error) {
		calls++
		if got := r.Header.Get("X-Platform-Caller"); got != want {
			t.Fatalf("Caller %q expected %q", got, want)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("{}"))}, nil
	})}
	request := func(method string) {
		t.Helper()
		r, _ := http.NewRequest(method, origin.String(), nil)
		r.Header.Set("X-Platform-Caller", "forged-inbound")
		res, e := transport.RoundTrip(r)
		if e != nil {
			t.Fatal(e)
		}
		res.Body.Close()
	}
	request("GET")
	request("POST")
	verified := 0
	transport.identityHeaders = func(context.Context) (map[string]string, error) {
		verified++
		return map[string]string{"X-Platform-Caller": "signed"}, nil
	}
	want = "signed"
	request("GET")
	request("POST")
	request("DELETE")
	if verified != 3 {
		t.Fatal("identity was not checked for every upstream request")
	}
	transport.identityHeaders = func(context.Context) (map[string]string, error) { return nil, errors.New("authority unavailable") }
	before := calls
	r, _ := http.NewRequest("POST", origin.String(), nil)
	if _, e := transport.RoundTrip(r); e == nil || calls != before {
		t.Fatal("authority failure reached upstream")
	}
}
