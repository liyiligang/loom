package controller

import (
	"encoding/json"
	"github.com/liyiligang/loom/internal/credentials"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/liyiligang/loom/internal/model"
)

type SecretValues = credentials.Values

type SecretInput struct {
	Content         *string `json:"content"`
	Name            *string `json:"name"`
	Kind            *string `json:"kind"`
	Token           *string `json:"token"`
	AccessKeyID     *string `json:"accessKeyId"`
	SecretAccessKey *string `json:"secretAccessKey"`
}

func (s *Service) secret(id string) *model.Secret {
	for _, secret := range s.data.Secrets {
		if secret.ID == id {
			return secret
		}
	}
	return nil
}

func (s *Service) ListSecrets() []model.Secret {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]model.Secret, 0, len(s.data.Secrets))
	for _, secret := range s.data.Secrets {
		result = append(result, *secret)
	}
	sort.SliceStable(result, func(i, j int) bool { return result[i].UpdatedAt > result[j].UpdatedAt })
	return result
}

func (s *Service) secretValuesLocked(id string) (SecretValues, error) {
	sealed, err := s.store.SecretCiphertext(id)
	if err != nil {
		return SecretValues{}, problem("secret_unavailable", "Could not read the saved credential.", 500)
	}
	plain, err := s.vault.Open(id, sealed)
	if err != nil {
		return SecretValues{}, problem("secret_unavailable", "Could not decrypt the saved credential.", 500)
	}
	defer clear(plain)
	var values SecretValues
	if json.Unmarshal(plain, &values) != nil {
		return values, problem("secret_unavailable", "Could not read the saved credential.", 500)
	}
	return values, nil
}

func (s *Service) RevealSecret(id string) (SecretValues, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.secret(id) == nil {
		return SecretValues{}, problem("secret_not_found", "Credential not found.", 404)
	}
	values, err := s.secretValuesLocked(id)
	if err == nil {
		values.Content = values.Text()
	}
	return values, err
}

func validSecretValue(value string) bool {
	return strings.TrimSpace(value) != "" && len(value) <= 8192 && !strings.ContainsAny(value, "\x00\r\n")
}

func validSecretContent(value string) bool {
	return strings.TrimSpace(value) != "" && len(value) <= 64<<10 && utf8.ValidString(value) && !strings.ContainsRune(value, '\x00')
}

func (s *Service) SaveSecret(id string, input SecretInput) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	current := s.secret(id)
	if id != "" && current == nil {
		return "", problem("secret_not_found", "Credential not found.", 404)
	}
	metadata := model.Secret{ID: model.ID(), CreatedAt: model.Now()}
	if current != nil {
		metadata = *current
	}
	if input.Name != nil {
		metadata.Name = strings.TrimSpace(*input.Name)
	}
	if input.Kind != nil {
		metadata.Kind = *input.Kind
	}
	raw := input.Content != nil || metadata.Kind == "text" || (input.Kind == nil && input.Token == nil && input.AccessKeyID == nil && input.SecretAccessKey == nil)
	if raw && (current == nil || (input.Content != nil && *input.Content != "")) {
		metadata.Kind = "text"
	}
	if raw && metadata.Name == "" {
		if current != nil {
			metadata.Name = current.Name
		} else {
			metadata.Name = "Credential " + metadata.ID[:8]
		}
	}
	if !validName(metadata.Name) || (metadata.Kind != "text" && metadata.Kind != "token" && metadata.Kind != "accessKey") {
		return "", problem("invalid_secret", "Enter a valid credential name.", 400)
	}
	for _, other := range s.data.Secrets {
		if other.ID != metadata.ID && strings.EqualFold(other.Name, metadata.Name) {
			return "", problem("secret_duplicate", "A credential with this name already exists.", 409)
		}
	}
	values := SecretValues{}
	if current != nil && (current.Kind == metadata.Kind || (raw && (input.Content == nil || *input.Content == ""))) {
		var err error
		values, err = s.secretValuesLocked(current.ID)
		if err != nil {
			return "", err
		}
	}
	if raw {
		if input.Token != nil || input.AccessKeyID != nil || input.SecretAccessKey != nil {
			return "", problem("invalid_secret_value", "Use credential text without separate value fields.", 400)
		}
		if input.Content != nil && *input.Content != "" {
			values = SecretValues{Content: *input.Content}
		}
		if !validSecretContent(values.Text()) {
			return "", problem("secret_values_required", "Paste credential text of at most 64 KiB. Leave it blank when editing to keep the saved value.", 400)
		}
	} else {
		fields := []struct {
			input *string
			value *string
		}{{input.Token, &values.Token}, {input.AccessKeyID, &values.AccessKeyID}, {input.SecretAccessKey, &values.SecretAccessKey}}
		for _, field := range fields {
			if field.input != nil && *field.input != "" {
				if !validSecretValue(*field.input) {
					return "", problem("invalid_secret_value", "Credential values must be single-line text of at most 8 KiB.", 400)
				}
				*field.value = *field.input
			}
		}
		if (metadata.Kind == "token" && (!validSecretValue(values.Token) || values.AccessKeyID != "" || values.SecretAccessKey != "")) || (metadata.Kind == "accessKey" && (!validSecretValue(values.AccessKeyID) || !validSecretValue(values.SecretAccessKey) || values.Token != "")) {
			return "", problem("secret_values_required", "Enter the token or both access keys. Leave existing values blank to keep them.", 400)
		}
	}
	plain, err := json.Marshal(values)
	if err != nil {
		return "", err
	}
	defer clear(plain)
	sealed, err := s.vault.Seal(metadata.ID, plain)
	if err != nil {
		return "", problem("secret_unavailable", "Could not encrypt the credential.", 500)
	}
	metadata.UpdatedAt = model.Now()
	if err := s.commitLocked("secrets.updated", metadata.ID, &model.SecretRecord{Secret: metadata, Ciphertext: sealed}); err != nil {
		return "", err
	}
	if current == nil {
		s.data.Secrets = append(s.data.Secrets, &metadata)
	} else {
		s.cancelCredentialUsesLocked("", metadata.ID, "")
		*current = metadata
	}
	return metadata.ID, nil
}

func (s *Service) DeleteSecret(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.secret(id) == nil {
		return problem("secret_not_found", "Credential not found.", 404)
	}
	projects := s.projectsWithoutSecretLocked(id)
	values := []any{&model.SecretRemoval{ID: id}}
	for _, project := range projects {
		values = append(values, project)
	}
	if err := s.commitLocked("secrets.updated", id, values...); err != nil {
		return err
	}
	s.cancelCredentialUsesLocked("", id, "")
	for _, project := range projects {
		*s.project(project.ID) = *project
	}
	for index, secret := range s.data.Secrets {
		if secret.ID == id {
			s.data.Secrets = append(s.data.Secrets[:index], s.data.Secrets[index+1:]...)
			break
		}
	}
	return nil
}
