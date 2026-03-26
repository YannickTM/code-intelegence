package providers

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// ValidateEndpointURL normalizes and validates a provider endpoint URL.
func ValidateEndpointURL(endpointURL string) (string, error) {
	endpointURL = strings.TrimSpace(endpointURL)
	if endpointURL == "" {
		return "", fmt.Errorf("endpoint_url is required")
	}

	u, err := url.Parse(endpointURL)
	if err != nil || u.Scheme == "" {
		return "", fmt.Errorf("endpoint_url must be a valid URL")
	}
	if u.Hostname() == "" {
		return "", fmt.Errorf("endpoint_url must include a host")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("endpoint_url must use http or https scheme")
	}
	if u.User != nil {
		return "", fmt.Errorf("endpoint_url must not contain user credentials")
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("endpoint_url must not contain query or fragment")
	}
	if port := u.Port(); port != "" {
		portNumber, err := strconv.Atoi(port)
		if err != nil || portNumber < 1 || portNumber > 65535 {
			return "", fmt.Errorf("endpoint_url must include a valid port (1-65535)")
		}
	} else if strings.HasSuffix(u.Host, ":") {
		return "", fmt.Errorf("endpoint_url must include a valid port (1-65535)")
	}
	return endpointURL, nil
}
