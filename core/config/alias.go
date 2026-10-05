package config

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"

	"go.yaml.in/yaml/v3"
)

func NormalizeAliasKey(key string) (string, error) {
	k := strings.ToLower(strings.TrimSpace(key))
	switch {
	case k == "":
		return "", errors.New("alias must not be empty")
	case strings.Contains(k, "."):
		return "", fmt.Errorf("alias %q must not contain '.'", key)
	}
	return k, nil
}

// SetAlias writes aliases.<key> = value into the YAML file at path, keeping
// comments and all other keys. The file is created if it does not exist.
func SetAlias(path, key, value string) error {
	k, err := NormalizeAliasKey(key)
	if err != nil {
		return err
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return errors.New("official name must not be empty")
	}

	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	out, err := setAliasYAML(data, k, value)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return WriteFileAtomic(path, out)
}

func setAliasYAML(data []byte, key, value string) ([]byte, error) {
	var doc yaml.Node
	if len(bytes.TrimSpace(data)) > 0 {
		if err := yaml.Unmarshal(data, &doc); err != nil {
			return nil, err
		}
	}
	if doc.Kind == 0 || len(doc.Content) == 0 {
		doc = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode, Tag: "!!map"}}}
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, errors.New("top level is not a mapping")
	}

	aliases := mappingValue(root, "aliases")
	if aliases == nil {
		aliases = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		root.Content = append(root.Content, scalar("aliases"), aliases)
	}
	if aliases.Kind == yaml.ScalarNode && (aliases.Tag == "!!null" || aliases.Value == "") {
		aliases.Kind, aliases.Tag, aliases.Value = yaml.MappingNode, "!!map", ""
	}
	if aliases.Kind != yaml.MappingNode {
		return nil, errors.New("aliases is not a mapping")
	}

	if v := mappingValue(aliases, key); v != nil {
		*v = *scalar(value)
	} else {
		aliases.Content = append(aliases.Content, scalar(key), scalar(value))
	}

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func mappingValue(m *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if strings.EqualFold(m.Content[i].Value, key) {
			return m.Content[i+1]
		}
	}
	return nil
}

func scalar(s string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: s}
}
