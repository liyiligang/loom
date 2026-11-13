package credentials

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
)

// Text preserves raw input and gives older records a lossless editable form.
func (v Values) Text() string {
	if v.Content != "" {
		return v.Content
	}
	if v.Token != "" {
		return v.Token
	}
	legacy := map[string]string{}
	if v.AccessKeyID != "" {
		legacy["accessKeyId"] = v.AccessKeyID
	}
	if v.SecretAccessKey != "" {
		legacy["secretAccessKey"] = v.SecretAccessKey
	}
	if len(legacy) == 0 {
		return ""
	}
	raw, _ := json.MarshalIndent(legacy, "", "  ")
	return string(raw)
}

var fieldName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.-]{0,127}$`)

// Fields interprets data only. Unknown formats remain available through content.
// Parsed names and values stay inside the request broker, never in metadata.
func (v Values) Fields() map[string]string {
	fields := map[string]string{"content": v.Text()}
	put := func(name, value string) {
		if name != "content" && fieldName.MatchString(name) && value != "" && len(fields) <= 128 {
			fields[name] = value
		}
	}
	put("token", v.Token)
	put("accessKeyId", v.AccessKeyID)
	put("secretAccessKey", v.SecretAccessKey)
	if v.Content == "" {
		return fields
	}
	var object map[string]any
	decoder := json.NewDecoder(strings.NewReader(v.Content))
	decoder.UseNumber()
	if json.Valid([]byte(v.Content)) && decoder.Decode(&object) == nil && object != nil {
		var walk func(map[string]any, string, int)
		walk = func(values map[string]any, prefix string, depth int) {
			if depth > 8 || len(fields) > 128 {
				return
			}
			for name, value := range values {
				path := prefix + name
				switch typed := value.(type) {
				case string:
					put(path, typed)
				case json.Number:
					put(path, typed.String())
				case map[string]any:
					walk(typed, path+".", depth+1)
				}
			}
		}
		walk(object, "", 0)
		return fields
	}
	for _, line := range strings.Split(v.Content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		index := strings.IndexAny(line, "=:")
		if index < 1 {
			continue
		}
		name, value := strings.TrimSpace(line[:index]), strings.TrimSpace(line[index+1:])
		if strings.HasPrefix(value, "\"") {
			if unquoted, err := strconv.Unquote(value); err == nil {
				value = unquoted
			}
		} else if len(value) >= 2 && value[0] == '\'' && value[len(value)-1] == '\'' {
			value = value[1 : len(value)-1]
		}
		if value != "|" && value != ">" && !strings.HasPrefix(value, "=") {
			put(name, value)
		}
	}
	return fields
}

func (v Values) Bearer() string {
	fields := v.Fields()
	for _, name := range []string{"token", "TOKEN", "api_key", "apiKey", "API_KEY", "API_TOKEN", "access_token"} {
		if fields[name] != "" {
			return fields[name]
		}
	}
	text := v.Text()
	if len(fields) == 1 && strings.TrimSpace(text) == text && !strings.ContainsAny(text, "\r\n\t ") && !strings.HasPrefix(text, "{") && !strings.HasPrefix(text, "[") {
		return text
	}
	return ""
}

func (v Values) AWSKeys() (access, secret, session string) {
	fields := v.Fields()
	pick := func(names ...string) string {
		for _, name := range names {
			if fields[name] != "" {
				return fields[name]
			}
		}
		return ""
	}
	return pick("accessKeyId", "aws_access_key_id", "AWS_ACCESS_KEY_ID", "AK"), pick("secretAccessKey", "aws_secret_access_key", "AWS_SECRET_ACCESS_KEY", "SK"), pick("sessionToken", "aws_session_token", "AWS_SESSION_TOKEN")
}
