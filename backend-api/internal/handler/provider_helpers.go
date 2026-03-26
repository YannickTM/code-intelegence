package handler

import (
	"encoding/json"

	"myjungle/backend-api/internal/providers"
	"myjungle/backend-api/internal/validate"
)

type credentialsUpdatePayload struct {
	set   bool
	clear bool
	data  map[string]any
}

func validateProviderEndpointURL(value, field string, errs validate.Errors) string {
	if value == "" {
		return value
	}
	normalized, err := providers.ValidateEndpointURL(value)
	if err != nil {
		errs.Add(field, err.Error())
		return value
	}
	return normalized
}

func parseCredentialsUpdate(raw json.RawMessage) (credentialsUpdatePayload, error) {
	if len(raw) == 0 {
		return credentialsUpdatePayload{}, nil
	}
	if string(raw) == "null" {
		return credentialsUpdatePayload{set: true, clear: true}, nil
	}

	var object map[string]any
	if err := json.Unmarshal(raw, &object); err != nil {
		return credentialsUpdatePayload{}, err
	}
	if len(object) == 0 {
		return credentialsUpdatePayload{set: true, clear: true}, nil
	}
	return credentialsUpdatePayload{set: true, data: object}, nil
}
