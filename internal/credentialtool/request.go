package credentialtool

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"html"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/liyiligang/loom/internal/credentials"
	"golang.org/x/net/http/httpguts"
)

type Request struct {
	Credential string            `json:"credential"`
	URL        string            `json:"url"`
	Method     string            `json:"method"`
	Headers    map[string]string `json:"headers"`
	Body       string            `json:"body"`
	AWS        *struct {
		Region  string `json:"region"`
		Service string `json:"service"`
	} `json:"aws"`
}
type Response struct {
	Status      int    `json:"status"`
	ContentType string `json:"contentType"`
	Body        string `json:"body"`
	Omitted     bool   `json:"omitted,omitempty"`
}

func publicAddress(ip netip.Addr) bool {
	ip = ip.Unmap()
	return ip.IsGlobalUnicast() && !ip.IsPrivate() && !ip.IsLoopback() && !ip.IsLinkLocalUnicast() && !netip.MustParsePrefix("100.64.0.0/10").Contains(ip)
}
func publicClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, errors.New("Invalid destination.")
		}
		ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
		if err != nil || len(ips) == 0 {
			return nil, errors.New("Could not resolve the destination.")
		}
		for _, ip := range ips {
			if !publicAddress(ip) {
				return nil, errors.New("Private network destinations are not allowed.")
			}
		}
		var last error
		for _, ip := range ips {
			conn, err := (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
			if err == nil {
				return conn, nil
			}
			last = err
		}
		return nil, last
	}
	return &http.Client{Transport: transport, Timeout: 45 * time.Second}
}

var placeholder = regexp.MustCompile(`\{\{([A-Za-z_][A-Za-z0-9_.-]{0,127})\}\}`)

func substitute(text string, fields map[string]string) (string, bool, error) {
	used := false
	pairs := []string{}
	for _, match := range placeholder.FindAllStringSubmatch(text, -1) {
		marker, value := match[0], fields[match[1]]
		if value == "" {
			return "", false, errors.New("The selected credential does not contain the requested field.")
		}
		used = true
		pairs = append(pairs, marker, value)
	}
	return strings.NewReplacer(pairs...).Replace(text), used, nil
}
func substituteJSON(value any, fields map[string]string) (any, bool, error) {
	used := false
	switch typed := value.(type) {
	case string:
		return substitute(typed, fields)
	case []any:
		for i, child := range typed {
			next, found, err := substituteJSON(child, fields)
			if err != nil {
				return nil, false, err
			}
			typed[i] = next
			used = used || found
		}
	case map[string]any:
		for key, child := range typed {
			next, found, err := substituteJSON(child, fields)
			if err != nil {
				return nil, false, err
			}
			typed[key] = next
			used = used || found
		}
	}
	return value, used, nil
}

func (b *Broker) request(ctx context.Context, session string, input Request) (Response, error) {
	target, err := url.Parse(input.URL)
	if err != nil || target.Scheme != "https" || target.Hostname() == "" || target.User != nil || target.Fragment != "" || strings.Contains(input.URL, "{{") {
		return Response{}, errors.New("Use a public HTTPS URL without credentials or placeholders.")
	}
	if ip, err := netip.ParseAddr(target.Hostname()); err == nil && !publicAddress(ip) {
		return Response{}, errors.New("Private network destinations are not allowed.")
	}
	method := strings.ToUpper(input.Method)
	if method == "" {
		method = "GET"
	}
	if !slices.Contains([]string{"GET", "HEAD", "POST", "PUT", "PATCH", "DELETE"}, method) || len(input.Body) > 128<<10 || len(input.Headers) > 32 {
		return Response{}, errors.New("Invalid or oversized API request.")
	}
	lease, err := b.access.LeaseSessionCredential(ctx, session, input.Credential)
	if err != nil {
		return Response{}, err
	}
	defer lease.Release()
	requestCtx, cancel := context.WithTimeout(lease.Context, 45*time.Second)
	defer cancel()
	fields := lease.Values.Fields()
	headers := http.Header{}
	used := false
	for name, value := range input.Headers {
		if !httpguts.ValidHeaderFieldName(name) || slices.Contains([]string{"Host", "Content-Length", "Connection", "Transfer-Encoding", "Trailer", "Proxy-Authorization"}, http.CanonicalHeaderKey(name)) {
			return Response{}, errors.New("Invalid request header.")
		}
		value, found, err := substitute(value, fields)
		if err != nil {
			return Response{}, err
		}
		if !httpguts.ValidHeaderFieldValue(value) || len(value) > 16384 {
			return Response{}, errors.New("Invalid request header value.")
		}
		headers.Set(name, value)
		used = used || found
	}
	body := input.Body
	if strings.Contains(strings.ToLower(headers.Get("Content-Type")), "json") && body != "" {
		var value any
		decoder := json.NewDecoder(strings.NewReader(body))
		decoder.UseNumber()
		if decoder.Decode(&value) != nil || decoder.Decode(new(any)) != io.EOF {
			return Response{}, errors.New("The JSON request body is invalid.")
		}
		value, found, err := substituteJSON(value, fields)
		if err != nil {
			return Response{}, err
		}
		used = used || found
		raw, _ := json.Marshal(value)
		body = string(raw)
	} else {
		next, found, err := substitute(body, fields)
		if err != nil {
			return Response{}, err
		}
		body = next
		used = used || found
	}
	if len(body) > 128<<10 {
		return Response{}, errors.New("The expanded request body is too large.")
	}
	request, err := http.NewRequestWithContext(requestCtx, method, input.URL, strings.NewReader(body))
	if err != nil {
		return Response{}, errors.New("Invalid API request.")
	}
	request.Header = headers
	if input.AWS != nil {
		access, secret, sessionToken := lease.Values.AWSKeys()
		if access == "" || secret == "" || input.AWS.Region == "" || input.AWS.Service == "" || len(input.AWS.Region) > 128 || len(input.AWS.Service) > 128 {
			return Response{}, errors.New("AWS signing requires AK/SK, region and service.")
		}
		hash := sha256.Sum256([]byte(body))
		payload := hex.EncodeToString(hash[:])
		request.Header.Set("X-Amz-Content-Sha256", payload)
		if err := v4.NewSigner().SignHTTP(requestCtx, aws.Credentials{AccessKeyID: access, SecretAccessKey: secret, SessionToken: sessionToken}, request, payload, input.AWS.Service, input.AWS.Region, time.Now()); err != nil {
			return Response{}, errors.New("Could not sign the API request.")
		}
		used = true
	} else if !used && lease.Values.Bearer() != "" && request.Header.Get("Authorization") == "" {
		value := "Bearer " + lease.Values.Bearer()
		if !httpguts.ValidHeaderFieldValue(value) || len(value) > 16384 {
			return Response{}, errors.New("Invalid authentication header value.")
		}
		request.Header.Set("Authorization", value)
		used = true
	}
	if !used {
		return Response{}, errors.New("Use {{content}} or a saved field placeholder in authentication headers or the body, or provide AWS signing.")
	}
	response, err := b.client.Do(request)
	if err != nil {
		return Response{}, errors.New("The credentialed API request failed or was canceled.")
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil {
		return Response{}, errors.New("Could not read the API response.")
	}
	if requestCtx.Err() != nil {
		return Response{}, errors.New("The credentialed API request was canceled.")
	}
	result := Response{Status: response.StatusCode, ContentType: response.Header.Get("Content-Type")}
	result.ContentType = redact(result.ContentType, lease.Values)
	if len(raw) > 1<<20 || (!strings.HasPrefix(result.ContentType, "text/") && !strings.Contains(result.ContentType, "json") && !strings.Contains(result.ContentType, "xml") && result.ContentType != "") {
		result.Omitted = true
		return result, nil
	}
	result.Body = redact(string(raw), lease.Values)
	if len(result.Body) > 1<<20 {
		result.Body = ""
		result.Omitted = true
	}
	return result, nil
}

func redact(text string, values credentials.Values) string {
	patterns := []string{}
	redactions := []string{values.Text()}
	for _, value := range values.Fields() {
		redactions = append(redactions, value)
	}
	for _, value := range slices.Clone(redactions) {
		for _, line := range strings.Split(value, "\n") {
			if strings.TrimSpace(line) != "" {
				redactions = append(redactions, line)
			}
		}
	}
	for _, value := range redactions {
		if value == "" {
			continue
		}
		patterns = append(patterns, value, url.QueryEscape(value), url.PathEscape(value), base64.StdEncoding.EncodeToString([]byte(value)), base64.RawStdEncoding.EncodeToString([]byte(value)), base64.URLEncoding.EncodeToString([]byte(value)), base64.RawURLEncoding.EncodeToString([]byte(value)))
		patterns = append(patterns, html.EscapeString(value), hex.EncodeToString([]byte(value)), strings.ToUpper(hex.EncodeToString([]byte(value))))
	}
	for _, pattern := range slices.Clone(patterns) {
		escaped, _ := json.Marshal(pattern)
		patterns = append(patterns, string(escaped[1:len(escaped)-1]))
	}
	slices.SortFunc(patterns, func(a, b string) int { return len(b) - len(a) })
	pairs := []string{}
	for _, pattern := range slices.Compact(patterns) {
		pairs = append(pairs, pattern, "[redacted]")
	}
	return strings.NewReplacer(pairs...).Replace(text)
}
