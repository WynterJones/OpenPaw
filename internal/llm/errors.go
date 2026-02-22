package llm

import (
	"strings"
)

func IsAuthError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "401") || strings.Contains(msg, "authentication") || strings.Contains(msg, "invalid x-api-key")
}

