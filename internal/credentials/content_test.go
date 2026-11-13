package credentials

import (
	"strings"
	"testing"
)

func TestCredentialTextAndDataFields(t *testing.T) {
	for _, test := range []struct {
		text, field, expected string
	}{
		{"opaque-dummy-token==", "content", "opaque-dummy-token=="},
		{"  unrelated text\nwith arbitrary format\n", "content", "  unrelated text\nwith arbitrary format\n"},
		{`{"auth":{"private_key":"dummy\nkey","client_id":12345}}`, "auth.private_key", "dummy\nkey"},
		{"export API_TOKEN='dummy-token'\nCUSTOM_KEY=\"dummy\\nkey\"", "CUSTOM_KEY", "dummy\nkey"},
		{"Authorization: Bearer dummy-value\nX-Custom-Key: dummy-custom", "X-Custom-Key", "dummy-custom"},
		{"custom_key: 'dummy-custom'\nprivate_key: |\n  dummy multiline text", "custom_key", "dummy-custom"},
		{"-----BEGIN PRIVATE KEY-----\ndummykeymaterial\n-----END PRIVATE KEY-----\n", "content", "-----BEGIN PRIVATE KEY-----\ndummykeymaterial\n-----END PRIVATE KEY-----\n"},
	} {
		t.Run(test.field+test.text[:min(len(test.text), 16)], func(t *testing.T) {
			values := Values{Content: test.text}
			if values.Text() != test.text || values.Fields()[test.field] != test.expected {
				t.Fatal("credential text changed or requested data field was not parsed")
			}
		})
	}
	if (Values{Content: "opaque-dummy-token=="}).Bearer() != "opaque-dummy-token==" {
		t.Fatal("a Base64 token was mistaken for an environment assignment")
	}
	if (Values{Content: "API_TOKEN='dummy-token'"}).Bearer() != "dummy-token" {
		t.Fatal("a pasted token assignment could not authenticate")
	}
	if (Values{Content: "arbitrary\nmultiline"}).Bearer() != "" {
		t.Fatal("multiline text was treated as a Bearer token")
	}
	legacy := Values{AccessKeyID: "dummy-ak", SecretAccessKey: "dummy-sk"}
	if !strings.Contains(legacy.Text(), "dummy-sk") || legacy.Fields()["accessKeyId"] != "dummy-ak" {
		t.Fatal("legacy AK/SK values were lost")
	}
	access, secret, session := (Values{Content: "AWS_ACCESS_KEY_ID=dummy-ak\nAWS_SECRET_ACCESS_KEY=dummy-sk\nAWS_SESSION_TOKEN=dummy-session"}).AWSKeys()
	if access != "dummy-ak" || secret != "dummy-sk" || session != "dummy-session" {
		t.Fatal("pasted temporary AWS credentials were not recognized")
	}
}

func TestCredentialParsingNeverEvaluatesPastedTextAndBoundsFields(t *testing.T) {
	values := Values{Content: "API_TOKEN=$(touch /tmp/should-not-exist)\nCUSTOM_KEY='${HOME}'\ncontent=do-not-shadow\ninvalid field=value"}
	fields := values.Fields()
	if fields["API_TOKEN"] != "$(touch /tmp/should-not-exist)" || fields["CUSTOM_KEY"] != "${HOME}" || fields["content"] != values.Content || fields["invalid field"] != "" {
		t.Fatal("pasted text was expanded or reserved fields were overwritten")
	}
	var text strings.Builder
	for i := range 1000 {
		text.WriteString("KEY" + strings.Repeat("x", i+1) + "=dummy\n")
	}
	if len((Values{Content: text.String()}).Fields()) > 129 {
		t.Fatal("parsed field limit was exceeded")
	}
}
