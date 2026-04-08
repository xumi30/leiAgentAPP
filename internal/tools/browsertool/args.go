package browsertool

import (
	"encoding/json"
	"fmt"
	"strings"
)

// parseBrowserAutomationArgs unmarshals tool arguments and normalizes alternate key names / wrappers.
func parseBrowserAutomationArgs(args string) (map[string]interface{}, error) {
	s := strings.TrimSpace(args)
	s = strings.TrimPrefix(s, "\ufeff")
	if s == "" {
		return nil, fmt.Errorf("empty arguments")
	}
	var params map[string]interface{}
	if err := json.Unmarshal([]byte(s), &params); err != nil {
		if i := strings.Index(s, "{"); i >= 0 {
			j := strings.LastIndex(s, "}")
			if j > i {
				fragment := strings.TrimSpace(s[i : j+1])
				if err2 := json.Unmarshal([]byte(fragment), &params); err2 == nil {
					return mergeParams(params), nil
				}
			}
		}
		return nil, fmt.Errorf("parse arguments: %w", err)
	}
	return mergeParams(params), nil
}

func mergeParams(params map[string]interface{}) map[string]interface{} {
	if props, ok := params["properties"].(string); ok && strings.TrimSpace(props) != "" {
		var inner map[string]interface{}
		if json.Unmarshal([]byte(props), &inner) == nil {
			for k, v := range inner {
				if _, exists := params[k]; !exists {
					params[k] = v
				}
			}
		}
	}
	if inner, ok := params["parameters"].(map[string]interface{}); ok {
		for k, v := range inner {
			if _, exists := params[k]; !exists {
				params[k] = v
			}
		}
	}
	aliases := map[string][]string{
		"url":      {"URL", "Url", "link", "href", "page_url"},
		"selector": {"Selector", "css", "css_selector", "elementSelector"},
		"action":   {"Action", "ACTION"},
	}
	for canonical, alts := range aliases {
		if getStringParam(params, canonical) != "" {
			continue
		}
		for _, alt := range alts {
			if v, ok := params[alt]; ok && v != nil {
				if s := strings.TrimSpace(coerceString(v)); s != "" {
					params[canonical] = v
					break
				}
			}
		}
	}
	return params
}

// inferActionIfMissing sets params["action"] when the model omits it but passes url/selector.
func inferActionIfMissing(params map[string]interface{}) string {
	if a := strings.TrimSpace(strings.ToLower(getStringParam(params, "action", "Action", "ACTION"))); a != "" {
		return a
	}
	hasURL := getStringParam(params, "url", "URL", "Url", "link", "href", "page_url") != ""
	hasSel := getStringParam(params, "selector", "Selector", "css", "css_selector", "elementSelector") != ""
	hasLinkFilter := strings.TrimSpace(getStringParam(params, "href_contains", "hrefContains")) != "" ||
		strings.TrimSpace(getStringParam(params, "text_contains", "textContains")) != ""
	if hasURL && hasLinkFilter && !hasSel {
		params["action"] = "list_links"
		return "list_links"
	}
	switch {
	case hasURL && hasSel:
		params["action"] = "click"
		return "click"
	case hasURL && !hasSel:
		params["action"] = "navigate"
		return "navigate"
	case hasSel && !hasURL:
		params["action"] = "click"
		return "click"
	default:
		return ""
	}
}

func getStringParam(params map[string]interface{}, keys ...string) string {
	for _, k := range keys {
		if v, ok := params[k]; ok && v != nil {
			s := strings.TrimSpace(coerceString(v))
			if s != "" {
				return s
			}
		}
	}
	return ""
}

func coerceString(v interface{}) string {
	switch t := v.(type) {
	case string:
		return t
	case json.Number:
		return t.String()
	case float64:
		return fmt.Sprint(t)
	case bool:
		if t {
			return "true"
		}
		return "false"
	default:
		return strings.TrimSpace(fmt.Sprint(t))
	}
}
