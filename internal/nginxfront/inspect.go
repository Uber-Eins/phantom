package nginxfront

import (
	"errors"
	"os"
)

// ReadConfig returns the active generated Nginx configuration. An empty result
// means no guided inbound has caused Nginx to be configured yet.
func ReadConfig() (string, error) {
	content, err := os.ReadFile(ConfigPath())
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return string(content), nil
}
