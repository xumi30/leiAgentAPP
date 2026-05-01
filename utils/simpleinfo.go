package utils

// SimpleInfoMap builds {"topic": topic, "simpledescription": simpledescription}.
// This lives in utils (not tools) to avoid cross-package coupling.
func SimpleInfoMap(topic, simpledescription string) map[string]string {
	return map[string]string{
		"topic":             topic,
		"simpledescription": simpledescription,
	}
}
