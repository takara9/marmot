package cmd

import (
	"encoding/json"
	"fmt"
	"strings"
)

func extractResponseID(body []byte) (string, error) {
	var data map[string]any
	if err := json.Unmarshal(body, &data); err != nil {
		return "", err
	}
	return extractResponseIDFromMap(data), nil
}

func extractResponseIDFromMap(data map[string]any) string {
	if data == nil {
		return ""
	}

	if metadata, ok := data["metadata"].(map[string]any); ok {
		if id := normalizeResponseID(metadata["id"]); id != "" {
			return id
		}
	}

	return normalizeResponseID(data["id"])
}

func normalizeResponseID(v any) string {
	id := strings.TrimSpace(fmt.Sprint(v))
	if id == "" || id == "<nil>" || id == "<null>" {
		return ""
	}
	return id
}
