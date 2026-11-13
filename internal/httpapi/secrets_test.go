package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/liyiligang/loom/internal/controller"
	"github.com/liyiligang/loom/internal/model"
	"github.com/liyiligang/loom/internal/storage"
)

func TestCredentialLifecycleKeepsValuesOutOfMetadataAndPersists(t *testing.T) {
	f := setup(t)
	var created struct{ ID string }
	f.call("POST", "/secrets", map[string]any{"name": "Platform", "kind": "token", "token": "dummy-token-value"}, 201, &created)
	f.call("PATCH", "/secrets/"+created.ID, map[string]any{"name": "Renamed", "token": ""}, 200, nil)
	var revealed controller.SecretValues
	f.call("POST", "/secrets/"+created.ID+"/reveal", nil, 200, &revealed)
	if revealed.Token != "dummy-token-value" {
		t.Fatal("renaming did not retain the value")
	}
	sealed, err := f.store.SecretCiphertext(created.ID)
	if err != nil || bytes.Contains(sealed, []byte(revealed.Token)) {
		t.Fatal("plaintext credential was stored")
	}
	for _, path := range []string{"/secrets", "/bootstrap"} {
		_, raw, err := f.request("GET", path, nil)
		if err != nil || bytes.Contains(raw, []byte(revealed.Token)) || bytes.Contains(raw, []byte("ciphertext")) {
			t.Fatal("metadata exposed credential values")
		}
	}
	data, err := f.store.Load()
	if err != nil || len(data.Secrets) != 1 {
		t.Fatal("credential metadata was not persisted")
	}
	events, _ := f.store.Events(0)
	raw, _ := json.Marshal(events)
	if bytes.Contains(raw, []byte(revealed.Token)) {
		t.Fatal("event replay exposed a credential")
	}
	f.stop()
	f.start()
	f.waitReady()
	f.call("POST", "/secrets/"+created.ID+"/reveal", nil, 200, &revealed)
	if revealed.Token != "dummy-token-value" {
		t.Fatal("restart lost the credential")
	}
	f.call("PATCH", "/secrets/"+created.ID, map[string]any{"kind": "accessKey", "accessKeyId": "dummy-ak", "secretAccessKey": "dummy-sk"}, 200, nil)
	f.call("PATCH", "/secrets/"+created.ID, map[string]any{"secretAccessKey": "rotated-sk"}, 200, nil)
	revealed = controller.SecretValues{}
	f.call("POST", "/secrets/"+created.ID+"/reveal", nil, 200, &revealed)
	if revealed.Token != "" || revealed.AccessKeyID != "dummy-ak" || revealed.SecretAccessKey != "rotated-sk" {
		t.Fatal("kind change or partial rotation failed")
	}
	f.call("DELETE", "/secrets/"+created.ID, nil, 200, nil)
	f.call("POST", "/secrets/"+created.ID+"/reveal", nil, 404, nil)
	if _, err := f.store.SecretCiphertext(created.ID); err == nil {
		t.Fatal("deletion retained ciphertext")
	}
	var records []model.Secret
	f.call("GET", "/secrets", nil, 200, &records)
	if len(records) != 0 {
		t.Fatal("deleted credential was listed")
	}
	f.native.mu.Lock()
	defer f.native.mu.Unlock()
	if f.native.starts != 0 {
		t.Fatal("credential management started a model turn")
	}
}

func TestCredentialValidationAndExplicitRevealGuards(t *testing.T) {
	f := setup(t)
	for _, body := range []map[string]any{
		{"name": "", "kind": "token", "token": "value"},
		{"name": "X", "kind": "other", "token": "value"},
		{"name": "X", "kind": "token"},
		{"name": "X", "kind": "accessKey", "accessKeyId": "ak"},
		{"name": "X", "kind": "token", "token": "value\nline"},
		{"name": "X", "kind": "token", "token": strings.Repeat("a", 8193)},
		{"name": "X", "kind": "token", "token": "value", "secretAccessKey": "sk"},
		{"name": "X", "kind": "token", "token": "value", "unknown": true},
	} {
		f.call("POST", "/secrets", body, 400, nil)
	}
	var saved struct{ ID string }
	f.call("POST", "/secrets", map[string]any{"name": "Unique", "kind": "token", "token": "dummy-value"}, 201, &saved)
	f.call("POST", "/secrets", map[string]any{"name": "unique", "kind": "token", "token": "other"}, 409, nil)
	f.call("PATCH", "/secrets/"+saved.ID, map[string]any{"kind": "accessKey", "accessKeyId": "ak"}, 400, nil)
	f.call("PATCH", "/secrets/missing", map[string]any{"name": "Other"}, 404, nil)
	f.call("DELETE", "/secrets/missing", nil, 404, nil)
	for _, test := range []struct {
		method, origin, content string
		want                    int
	}{
		{"POST", f.server.URL, "application/json", 200},
		{"POST", "https://untrusted.example", "application/json", 403},
		{"POST", f.server.URL, "text/plain", 415},
		{"GET", f.server.URL, "application/json", 404},
	} {
		request, _ := http.NewRequest(test.method, f.server.URL+"/api/v1/secrets/"+saved.ID+"/reveal", strings.NewReader("{}"))
		request.Header.Set("Origin", test.origin)
		request.Header.Set("Content-Type", test.content)
		response, err := f.server.Client().Do(request)
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, response.Body)
		response.Body.Close()
		if response.StatusCode != test.want {
			t.Fatalf("reveal returned %d, want %d", response.StatusCode, test.want)
		}
		if test.want == 200 && response.Header.Get("Cache-Control") != "no-store" {
			t.Fatal("reveal response could be cached")
		}
	}
}

func TestMissingVaultKeyDoesNotReplaceExistingCredentialKey(t *testing.T) {
	f := setup(t)
	f.call("POST", "/secrets", map[string]any{"name": "Existing", "kind": "token", "token": "dummy-value"}, 201, nil)
	f.stop()
	// The normal cleanup operates on these closed handles; closing again is safe.
	key := filepath.Join(f.dataPath, "credentials", "vault.key")
	if err := os.Remove(key); err != nil {
		t.Fatal(err)
	}
	store, err := storage.Open(filepath.Join(f.dataPath, "loom.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if service, err := controller.New(context.Background(), store, nil); err == nil {
		service.Close()
		t.Fatal("startup replaced a lost vault key")
	}
	if _, err := os.Stat(key); !os.IsNotExist(err) {
		t.Fatal("a replacement vault key was generated")
	}
}

func TestRawCredentialTextRoundTripWithoutTypeOrName(t *testing.T) {
	f := setup(t)
	for _, text := range []string{
		"  dummy opaque text\r\nwith arbitrary format\n",
		`{"custom":{"client":"dummy-client","private_key":"dummy\nkey"}}`,
		"AWS_ACCESS_KEY_ID=dummy-ak\nAWS_SECRET_ACCESS_KEY=dummy-sk\n",
		"custom_key: dummy-key\nprivate_key: |\n  dummy-multiline-key\n",
		"-----BEGIN PRIVATE KEY-----\ndummy-key-material\n-----END PRIVATE KEY-----\n",
		strings.Repeat("x", 64<<10),
		strings.Repeat("\n", (64<<10)-1) + "x",
	} {
		var created struct{ ID string }
		f.call("POST", "/secrets", map[string]any{"content": text}, 201, &created)
		var reveal controller.SecretValues
		f.call("POST", "/secrets/"+created.ID+"/reveal", nil, 200, &reveal)
		if reveal.Content != text {
			t.Fatal("pasted text did not round-trip exactly")
		}
		f.call("PATCH", "/secrets/"+created.ID, map[string]any{"content": "", "name": ""}, 200, nil)
		f.call("POST", "/secrets/"+created.ID+"/reveal", nil, 200, &reveal)
		if reveal.Content != text {
			t.Fatal("empty edit discarded the saved text")
		}
		sealed, err := f.store.SecretCiphertext(created.ID)
		if err != nil || bytes.Contains(sealed, []byte(text)) {
			t.Fatal("pasted text was stored in plaintext")
		}
	}
	var records []model.Secret
	f.call("GET", "/secrets", nil, 200, &records)
	for _, record := range records {
		if record.Kind != "text" || !strings.HasPrefix(record.Name, "Credential ") {
			t.Fatal("raw credentials required a user-supplied type or name")
		}
	}
	for _, text := range []string{"", " \n\t", "dummy\x00value", strings.Repeat("x", (64<<10)+1)} {
		f.call("POST", "/secrets", map[string]any{"content": text}, 400, nil)
	}
	var legacy struct{ ID string }
	f.call("POST", "/secrets", map[string]any{"name": "Legacy", "kind": "accessKey", "accessKeyId": "dummy-ak", "secretAccessKey": "dummy-sk"}, 201, &legacy)
	f.call("PATCH", "/secrets/"+legacy.ID, map[string]any{"name": ""}, 200, nil)
	var reveal controller.SecretValues
	f.call("POST", "/secrets/"+legacy.ID+"/reveal", nil, 200, &reveal)
	if !strings.Contains(reveal.Content, "dummy-ak") || reveal.AccessKeyID != "dummy-ak" {
		t.Fatal("legacy credentials did not provide an editable text form")
	}
	f.call("PATCH", "/secrets/"+legacy.ID, map[string]any{"content": "dummy-new\ntext"}, 200, nil)
	f.stop()
	f.start()
	f.waitReady()
	reveal = controller.SecretValues{}
	f.call("POST", "/secrets/"+legacy.ID+"/reveal", nil, 200, &reveal)
	if reveal.Content != "dummy-new\ntext" || reveal.AccessKeyID != "" || reveal.SecretAccessKey != "" {
		t.Fatal("replacement text did not survive restart or retained old fields")
	}
	for _, path := range []string{"/secrets", "/bootstrap"} {
		_, raw, err := f.request("GET", path, nil)
		if err != nil || bytes.Contains(raw, []byte("dummy-")) || bytes.Contains(raw, []byte(`"content"`)) {
			t.Fatal("metadata exposed pasted credential text")
		}
	}
	events, _ := f.store.Events(0)
	raw, _ := json.Marshal(events)
	if bytes.Contains(raw, []byte("dummy-")) {
		t.Fatal("event replay exposed pasted credential text")
	}
}
