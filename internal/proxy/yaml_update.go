package proxy

import (
	"bytes"
	"fmt"
	"strings"

	yamlv3 "go.yaml.in/yaml/v3"
)

func parseYAMLDocumentNode(content string) (*yamlv3.Node, error) {
	var doc yamlv3.Node
	s := strings.ReplaceAll(content, "\r\n", "\n")
	if strings.TrimSpace(s) == "" {
		// Create empty document with mapping root.
		doc.Kind = yamlv3.DocumentNode
		doc.Content = []*yamlv3.Node{{Kind: yamlv3.MappingNode, Tag: "!!map"}}
		return &doc, nil
	}
	if err := yamlv3.Unmarshal([]byte(s), &doc); err != nil {
		return nil, err
	}
	if doc.Kind == 0 {
		doc.Kind = yamlv3.DocumentNode
	}
	if doc.Kind != yamlv3.DocumentNode {
		// Should not happen for Unmarshal into Node, but be defensive.
		return nil, fmt.Errorf("unexpected YAML root kind=%d", doc.Kind)
	}
	if len(doc.Content) == 0 {
		doc.Content = []*yamlv3.Node{{Kind: yamlv3.MappingNode, Tag: "!!map"}}
	}
	if doc.Content[0].Kind != yamlv3.MappingNode {
		// Normalize to a mapping; we only support root maps for config.yaml.
		doc.Content[0] = &yamlv3.Node{Kind: yamlv3.MappingNode, Tag: "!!map"}
	}
	return &doc, nil
}

func marshalYAMLDocumentNode(doc *yamlv3.Node) ([]byte, error) {
	var buf bytes.Buffer
	enc := yamlv3.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		_ = enc.Close()
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	out := buf.Bytes()
	if len(out) == 0 || out[len(out)-1] != '\n' {
		out = append(out, '\n')
	}
	return out, nil
}

func nodeFromValue(v any) (*yamlv3.Node, error) {
	b, err := yamlv3.Marshal(v)
	if err != nil {
		return nil, err
	}
	var doc yamlv3.Node
	if err := yamlv3.Unmarshal(b, &doc); err != nil {
		return nil, err
	}
	if doc.Kind != yamlv3.DocumentNode || len(doc.Content) == 0 {
		return nil, fmt.Errorf("failed to build YAML node from value")
	}
	return doc.Content[0], nil
}

// upsertRootKey updates/creates key in a document-root mapping.
func upsertRootKey(doc *yamlv3.Node, key string, valNode *yamlv3.Node) {
	if doc == nil || len(doc.Content) == 0 {
		return
	}
	root := doc.Content[0]
	if root.Kind != yamlv3.MappingNode {
		root.Kind = yamlv3.MappingNode
		root.Tag = "!!map"
		root.Content = nil
	}
	k := strings.TrimSpace(key)
	if k == "" {
		return
	}
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Kind == yamlv3.ScalarNode && root.Content[i].Value == k {
			root.Content[i+1] = valNode
			return
		}
	}
	root.Content = append(root.Content,
		&yamlv3.Node{Kind: yamlv3.ScalarNode, Tag: "!!str", Value: k},
		valNode,
	)
}

func removeRootKey(doc *yamlv3.Node, key string) {
	if doc == nil || len(doc.Content) == 0 || doc.Content[0].Kind != yamlv3.MappingNode {
		return
	}
	root := doc.Content[0]
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Kind == yamlv3.ScalarNode && root.Content[i].Value == key {
			root.Content = append(root.Content[:i], root.Content[i+2:]...)
			return
		}
	}
}
