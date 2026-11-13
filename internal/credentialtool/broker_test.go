package credentialtool

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"html"
	"io"
	"net/http"
	"net/netip"
	"strings"
	"testing"

	"github.com/liyiligang/loom/internal/credentials"
	"github.com/liyiligang/loom/internal/model"
)

type fixtureAccess struct {
	values   credentials.Values
	inactive bool
}

func (a *fixtureAccess) ListSessionCredentials(id string) ([]model.Secret, error) {
	if id != "session-one" {
		return nil, errors.New("wrong session")
	}
	return []model.Secret{{ID: "enabled", Name: "Platform", Kind: "token"}}, nil
}
func (a *fixtureAccess) LeaseSessionCredential(ctx context.Context, id, name string) (Lease, error) {
	if id != "session-one" || name != "enabled" || a.inactive {
		return Lease{}, errors.New("Credential not activated.")
	}
	return Lease{Context: ctx, Values: a.values, Release: func() {}}, nil
}

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func reply(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}
}
func fixtureBroker(t *testing.T, access *fixtureAccess, transport roundTrip) *Broker {
	t.Helper()
	b, err := New(access, &http.Client{Transport: transport})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(b.Close)
	return b
}

func TestMCPAuthenticationAndCredentialedRequestRedactValues(t *testing.T) {
	access := &fixtureAccess{values: credentials.Values{Token: "dummy\"token-value"}}
	calls := 0
	b := fixtureBroker(t, access, func(request *http.Request) (*http.Response, error) {
		calls++
		if request.Header.Get("Authorization") != "Bearer "+access.values.Token {
			t.Fatal("request did not receive the selected token")
		}
		if request.URL.String() != "https://platform.example/api" {
			t.Fatal("request target changed")
		}
		body, _ := json.Marshal(map[string]string{"echo": access.values.Token, "base64": base64.StdEncoding.EncodeToString([]byte(access.values.Token)), "hex": hex.EncodeToString([]byte(access.values.Token)), "html": html.EscapeString(access.values.Token), "result": "ok"})
		return reply(200, string(body)), nil
	})
	server := b.Session("session-one")
	call := func(token, origin string, packet any) (int, string) {
		raw, _ := json.Marshal(packet)
		request, _ := http.NewRequest("POST", server.URL, strings.NewReader(string(raw)))
		request.Header.Set("Authorization", "Bearer "+token)
		request.Header.Set("Content-Type", "application/json")
		if origin != "" {
			request.Header.Set("Origin", origin)
		}
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		if response.Header.Get("Cache-Control") != "no-store" {
			t.Fatal("MCP response could be cached")
		}
		data, _ := io.ReadAll(response.Body)
		return response.StatusCode, string(data)
	}
	packet := map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]any{"protocolVersion": "2025-06-18"}}
	if status, _ := call("forged", "", packet); status != 401 {
		t.Fatal("forged capability was accepted")
	}
	if status, _ := call(server.Token, "http://localhost", packet); status != 401 {
		t.Fatal("browser origin was accepted")
	}
	if status, raw := call(server.Token, "", packet); status != 200 || !strings.Contains(raw, "2025-06-18") {
		t.Fatal("MCP initialization failed")
	}
	packet["method"] = "tools/list"
	if _, raw := call(server.Token, "", packet); !strings.Contains(raw, "list_credentials") || strings.Contains(raw, "dummy") {
		t.Fatal("tool discovery failed or leaked a value")
	}
	packet["method"] = "tools/call"
	packet["params"] = map[string]any{"name": "list_credentials", "arguments": map[string]any{}}
	if _, raw := call(server.Token, "", packet); !strings.Contains(raw, "Platform") || strings.Contains(raw, "dummy") {
		t.Fatal("metadata listing leaked values")
	}
	packet["params"] = map[string]any{"name": "request", "arguments": map[string]any{"credential": "enabled", "url": "https://platform.example/api"}}
	_, raw := call(server.Token, "", packet)
	if calls != 1 || strings.Contains(raw, "dummy") || !strings.Contains(raw, "[redacted]") || !strings.Contains(raw, "ok") {
		t.Fatal("credentialed response was not redacted")
	}
	if strings.Contains(raw, base64.StdEncoding.EncodeToString([]byte(access.values.Token))) || strings.Contains(raw, hex.EncodeToString([]byte(access.values.Token))) {
		t.Fatal("encoded credential was exposed")
	}
	access.inactive = true
	_, raw = call(server.Token, "", packet)
	if calls != 1 || !strings.Contains(raw, `"isError":true`) {
		t.Fatal("deactivated credential still reached the platform")
	}
	b.RevokeSession("session-one")
	if status, _ := call(server.Token, "", packet); status != 401 {
		t.Fatal("revoked session capability was accepted")
	}
}

func TestCustomHeadersJSONAndAWSSigningUseOnlySelectedValues(t *testing.T) {
	access := &fixtureAccess{values: credentials.Values{AccessKeyID: "dummy-ak", SecretAccessKey: "dummy\"sk"}}
	phase := 0
	b := fixtureBroker(t, access, func(request *http.Request) (*http.Response, error) {
		if phase == 0 {
			if request.Header.Get("X-Access-Key") != "dummy-ak" || request.Header.Get("X-Secret-Key") != "dummy\"sk" {
				t.Fatal("custom header substitution failed")
			}
			var body map[string]string
			if json.NewDecoder(request.Body).Decode(&body) != nil || body["secret"] != "dummy\"sk" {
				t.Fatal("JSON substitution did not escape the value")
			}
		} else {
			authorization := request.Header.Get("Authorization")
			if !strings.HasPrefix(authorization, "AWS4-HMAC-SHA256 Credential=dummy-ak/") || !strings.Contains(authorization, "/us-east-1/s3/aws4_request") || request.Header.Get("X-Amz-Date") == "" || strings.Contains(authorization, "dummy\"sk") {
				t.Fatal("AWS request was not signed with the selected AK/SK")
			}
		}
		return reply(200, `{"status":"ok"}`), nil
	})
	input := Request{Credential: "enabled", URL: "https://platform.example/api", Method: "POST", Headers: map[string]string{"X-Access-Key": "{{accessKeyId}}", "X-Secret-Key": "{{secretAccessKey}}", "Content-Type": "application/json"}, Body: `{"secret":"{{secretAccessKey}}"}`}
	if result, err := b.request(context.Background(), "session-one", input); err != nil || result.Status != 200 {
		t.Fatalf("custom authentication failed: %v", err)
	}
	phase = 1
	input = Request{Credential: "enabled", URL: "https://platform.example/api"}
	json.Unmarshal([]byte(`{"region":"us-east-1","service":"s3"}`), &input.AWS)
	if _, err := b.request(context.Background(), "session-one", input); err != nil {
		t.Fatal(err)
	}
}

func TestRequestRejectsUnsafeTargetsHeadersAndOversizedResponses(t *testing.T) {
	calls := 0
	access := &fixtureAccess{values: credentials.Values{Token: "dummy-token"}}
	b := fixtureBroker(t, access, func(request *http.Request) (*http.Response, error) {
		calls++
		return reply(200, strings.Repeat("x", (1<<20)+1)), nil
	})
	for _, target := range []string{"http://platform.example", "https://user:pass@platform.example", "https://127.0.0.1", "https://[::1]", "https://10.0.0.1", "https://169.254.169.254", "https://100.100.100.200", "https://platform.example?token={{token}}"} {
		if _, err := b.request(context.Background(), "session-one", Request{Credential: "enabled", URL: target}); err == nil {
			t.Fatalf("unsafe target accepted: %s", target)
		}
	}
	for name, value := range map[string]string{"Host": "other.example", "Connection": "keep-alive", "X-Test": "line\r\nInjected: header"} {
		if _, err := b.request(context.Background(), "session-one", Request{Credential: "enabled", URL: "https://platform.example", Headers: map[string]string{name: value}}); err == nil {
			t.Fatal("unsafe header was accepted")
		}
	}
	if calls != 0 {
		t.Fatal("invalid requests reached a platform")
	}
	result, err := b.request(context.Background(), "session-one", Request{Credential: "enabled", URL: "https://platform.example"})
	if err != nil || !result.Omitted || result.Body != "" {
		t.Fatal("oversized response was exposed")
	}
	for _, address := range []string{"::ffff:127.0.0.1", "fc00::1", "fe80::1", "100.64.0.1"} {
		if publicAddress(netip.MustParseAddr(address)) {
			t.Fatal("private address accepted")
		}
	}
}

func TestRawCredentialsUseGenericFieldsAndRedactMultilineText(t *testing.T) {
	access := &fixtureAccess{values: credentials.Values{Content: `{"auth":{"custom_key":"dummy\"custom-key","private_key":"dummy-first-line\ndummy-second-line"}}`}}
	calls := 0
	b := fixtureBroker(t, access, func(request *http.Request) (*http.Response, error) {
		calls++
		if request.Header.Get("X-Custom-Key") != "dummy\"custom-key" {
			t.Fatal("generic authentication field was not substituted")
		}
		var body map[string]string
		if json.NewDecoder(request.Body).Decode(&body) != nil || body["key"] != "dummy-first-line\ndummy-second-line" || body["complete"] != access.values.Content {
			t.Fatal("multiline JSON substitution changed the pasted text")
		}
		body["fragment"] = "dummy-first-line"
		raw, _ := json.Marshal(body)
		return reply(200, string(raw)), nil
	})
	input := Request{Credential: "enabled", URL: "https://platform.example/api", Method: "POST", Headers: map[string]string{"X-Custom-Key": "{{auth.custom_key}}", "Content-Type": "application/json"}, Body: `{"key":"{{auth.private_key}}","complete":"{{content}}"}`}
	result, err := b.request(context.Background(), "session-one", input)
	if err != nil || strings.Contains(result.Body, "dummy-") || !strings.Contains(result.Body, "[redacted]") {
		t.Fatalf("generic response exposed credential text: %v", err)
	}
	input.Headers["X-Custom-Key"] = "{{missing_field}}"
	if _, err := b.request(context.Background(), "session-one", input); err == nil || calls != 1 {
		t.Fatal("missing credential field reached a third-party platform")
	}
	input.Headers["X-Custom-Key"] = "{{auth.private_key}}"
	if _, err := b.request(context.Background(), "session-one", input); err == nil || calls != 1 {
		t.Fatal("multiline credential was injected into HTTP headers")
	}
	access.values = credentials.Values{Content: strings.Repeat("x", 64<<10)}
	input.Headers = map[string]string{}
	input.Body = "{{content}}{{content}}{{content}}"
	if _, err := b.request(context.Background(), "session-one", input); err == nil || calls != 1 {
		t.Fatal("expanded request exceeded the body bound")
	}
}

func TestPastedAWSKeysIncludeSessionTokenInSignature(t *testing.T) {
	access := &fixtureAccess{values: credentials.Values{Content: "AWS_ACCESS_KEY_ID=dummy-ak\nAWS_SECRET_ACCESS_KEY=dummy-sk\nAWS_SESSION_TOKEN=dummy-session-token"}}
	b := fixtureBroker(t, access, func(request *http.Request) (*http.Response, error) {
		if !strings.HasPrefix(request.Header.Get("Authorization"), "AWS4-HMAC-SHA256 Credential=dummy-ak/") || request.Header.Get("X-Amz-Security-Token") != "dummy-session-token" {
			t.Fatal("pasted temporary AWS credentials were not used for signing")
		}
		return reply(200, `{"status":"ok"}`), nil
	})
	input := Request{Credential: "enabled", URL: "https://platform.example/api"}
	json.Unmarshal([]byte(`{"region":"us-east-1","service":"s3"}`), &input.AWS)
	if _, err := b.request(context.Background(), "session-one", input); err != nil {
		t.Fatal(err)
	}
}
