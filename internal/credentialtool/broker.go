package credentialtool

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/liyiligang/loom/internal/credentials"
	"github.com/liyiligang/loom/internal/model"
	"github.com/liyiligang/loom/internal/provider"
)

// Access checks current project grants before exposing values to a bounded request.
type Access interface {
	ListSessionCredentials(string) ([]model.Secret, error)
	LeaseSessionCredential(context.Context, string, string) (Lease, error)
}

type Lease struct {
	Context context.Context
	Values  credentials.Values
	Release func()
}

// Broker provides authenticated, session-scoped MCP tools without returning secrets.
type Broker struct {
	access   Access
	client   *http.Client
	server   *http.Server
	address  string
	mu       sync.Mutex
	sessions map[string]string
	tokens   map[string]string
}

func New(access Access, client *http.Client) (*Broker, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	if client == nil {
		client = publicClient()
	}
	copy := *client
	copy.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	b := &Broker{access: access, client: &copy, address: listener.Addr().String(), sessions: map[string]string{}, tokens: map[string]string{}}
	b.server = &http.Server{Handler: http.HandlerFunc(b.serve), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
	go b.server.Serve(listener)
	return b, nil
}

func (b *Broker) Session(id string) provider.ToolServer {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.tokens[id] == "" {
		token := make([]byte, 32)
		if _, err := rand.Read(token); err != nil {
			panic("could not generate a credential tool capability")
		}
		b.tokens[id] = hex.EncodeToString(token)
		b.sessions[b.tokens[id]] = id
	}
	return provider.ToolServer{URL: "http://" + b.address + "/mcp", Token: b.tokens[id]}
}

func (b *Broker) RevokeSession(id string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.sessions, b.tokens[id])
	delete(b.tokens, id)
}
func (b *Broker) Close() { b.server.Close() }

func (b *Broker) serve(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	b.mu.Lock()
	session := b.sessions[strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")]
	b.mu.Unlock()
	if r.Host != b.address || r.Header.Get("Origin") != "" || !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") || session == "" {
		http.Error(w, "Unauthorized credential tool request.", http.StatusUnauthorized)
		return
	}
	if r.URL.Path != "/mcp" {
		http.NotFound(w, r)
		return
	}
	if r.Method == "DELETE" {
		w.WriteHeader(200)
		return
	}
	if r.Method != "POST" {
		w.Header().Set("Allow", "POST, DELETE")
		w.WriteHeader(405)
		return
	}
	media, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if media != "application/json" {
		w.WriteHeader(415)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 256<<10)
	var packet struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Method  string          `json:"method"`
		Params  json.RawMessage `json:"params"`
	}
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&packet); err != nil || packet.JSONRPC != "2.0" || decoder.Decode(new(any)) != io.EOF {
		w.WriteHeader(400)
		return
	}
	if len(packet.ID) == 0 {
		if !strings.HasPrefix(packet.Method, "notifications/") {
			w.WriteHeader(400)
			return
		}
		w.WriteHeader(202)
		return
	}
	result, err := b.call(r.Context(), session, packet.Method, packet.Params)
	response := map[string]any{"jsonrpc": "2.0", "id": packet.ID}
	if err != nil {
		response["error"] = map[string]any{"code": -32601, "message": err.Error()}
	} else {
		response["result"] = result
	}
	json.NewEncoder(w).Encode(response)
}

func (b *Broker) call(ctx context.Context, session, method string, params json.RawMessage) (any, error) {
	switch method {
	case "initialize":
		var input struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		json.Unmarshal(params, &input)
		version := input.ProtocolVersion
		if version != "2025-11-25" && version != "2025-06-18" && version != "2025-03-26" {
			version = "2025-03-26"
		}
		return map[string]any{"protocolVersion": version, "capabilities": map[string]any{"tools": map[string]any{}}, "serverInfo": map[string]any{"name": "loom_credentials", "version": model.Version}, "instructions": "Use list_credentials to discover credentials activated for the current project. Use request to access third-party APIs without reading credential values."}, nil
	case "ping":
		return map[string]any{}, nil
	case "tools/list":
		return map[string]any{"tools": tools}, nil
	case "tools/call":
		var input struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if json.Unmarshal(params, &input) != nil {
			return toolResult(nil, errors.New("Invalid tool arguments.")), nil
		}
		switch input.Name {
		case "list_credentials":
			values, err := b.access.ListSessionCredentials(session)
			return toolResult(values, err), nil
		case "request":
			var request Request
			if json.Unmarshal(input.Arguments, &request) != nil {
				return toolResult(nil, errors.New("Invalid request arguments.")), nil
			}
			result, err := b.request(ctx, session, request)
			return toolResult(result, err), nil
		default:
			return toolResult(nil, errors.New("Unknown credential tool.")), nil
		}
	default:
		return nil, errors.New("Unsupported MCP method.")
	}
}

func toolResult(value any, err error) any {
	text := ""
	if err != nil {
		text = err.Error()
	} else {
		raw, _ := json.Marshal(value)
		text = string(raw)
	}
	return map[string]any{"content": []map[string]string{{"type": "text", "text": text}}, "isError": err != nil}
}

var tools = []map[string]any{
	{"name": "list_credentials", "description": "List only the credential metadata activated for this session's project. Values are never returned.", "inputSchema": map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false}, "annotations": map[string]any{"readOnlyHint": true}},
	{"name": "request", "description": "Make a bounded HTTPS API request using an activated project credential. Credentials store arbitrary pasted text. Use {{content}} for the complete text, or {{fieldName}} for a JSON string/number field, nested JSON field such as {{auth.api_key}}, environment assignment, or flat YAML/header field. Values are substituted in headers or body without disclosure to the model; unknown fields fail. A plain single-line token or recognized token field defaults to Authorization: Bearer. Legacy {{token}}, {{accessKeyId}} and {{secretAccessKey}} remain supported. For AWS SigV4, provide aws.region and aws.service; common AWS key and optional session-token fields are recognized automatically. Other signing schemes need an adapter. Redirects and private network destinations are refused. Responses redact credential values. Use normal agent permission policies for mutations.", "inputSchema": json.RawMessage(`{"type":"object","properties":{"credential":{"type":"string","description":"An activated credential ID or exact name."},"url":{"type":"string","description":"Public HTTPS URL. Do not include credential placeholders in the URL."},"method":{"type":"string","enum":["GET","HEAD","POST","PUT","PATCH","DELETE"]},"headers":{"type":"object","additionalProperties":{"type":"string"}},"body":{"type":"string","description":"Optional request body. JSON string placeholders are escaped correctly."},"aws":{"type":"object","properties":{"region":{"type":"string"},"service":{"type":"string"}},"required":["region","service"],"additionalProperties":false}},"required":["credential","url"],"additionalProperties":false}`), "annotations": map[string]any{"readOnlyHint": false, "destructiveHint": true, "openWorldHint": true}},
}
