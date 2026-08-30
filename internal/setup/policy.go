package setup

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/TKlerx/github-runner-dispatcher/internal/config"
	"go.yaml.in/yaml/v4"
)

func MutatePolicy(configPath, policyPath, action string) error {
	if action != "add" && action != "reconcile" && action != "remove" {
		return errors.New("policy action must be add, reconcile, or remove")
	}
	policyData, err := os.ReadFile(policyPath)
	if err != nil {
		return fmt.Errorf("read policy file: %w", err)
	}
	policy, policyNode, err := parsePolicyDocument(policyData)
	if err != nil {
		return err
	}
	configData, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}
	var document yaml.Node
	if err := yaml.Unmarshal(configData, &document); err != nil {
		return fmt.Errorf("decode config: %w", err)
	}
	repositories, err := mappingValue(document.Content[0], "repositories")
	if err != nil || repositories.Kind != yaml.SequenceNode {
		return errors.New("configuration has no repositories sequence")
	}
	index := repositoryNodeIndex(repositories, policy.Owner, policy.Name)
	switch action {
	case "add":
		if index >= 0 {
			return fmt.Errorf("repository %s/%s already exists", policy.Owner, policy.Name)
		}
		repositories.Content = append(repositories.Content, policyNode)
	case "reconcile":
		if index >= 0 {
			repositories.Content[index] = policyNode
		} else {
			repositories.Content = append(repositories.Content, policyNode)
		}
	case "remove":
		if index < 0 {
			return fmt.Errorf("repository %s/%s is not configured", policy.Owner, policy.Name)
		}
		repositories.Content = append(repositories.Content[:index], repositories.Content[index+1:]...)
	}
	if err := validateConfigDocument(&document); err != nil {
		return err
	}
	return atomicWriteYAML(configPath, &document)
}

func parsePolicyDocument(data []byte) (config.Repository, *yaml.Node, error) {
	var decoded struct {
		Repository config.Repository `yaml:"repository"`
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&decoded); err != nil {
		return config.Repository{}, nil, fmt.Errorf("decode policy file: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return config.Repository{}, nil, errors.New("policy file must contain exactly one YAML document")
	}
	decoded.Repository.Owner = strings.TrimSpace(decoded.Repository.Owner)
	decoded.Repository.Name = strings.TrimSpace(decoded.Repository.Name)
	if decoded.Repository.Owner == "" || decoded.Repository.Name == "" {
		return config.Repository{}, nil, errors.New("policy repository owner and name are required")
	}
	var document yaml.Node
	if err := yaml.Unmarshal(data, &document); err != nil {
		return config.Repository{}, nil, fmt.Errorf("decode policy node: %w", err)
	}
	node, err := mappingValue(document.Content[0], "repository")
	if err != nil || node.Kind != yaml.MappingNode {
		return config.Repository{}, nil, errors.New("policy file must contain one repository mapping")
	}
	return decoded.Repository, node, nil
}

func repositoryNodeIndex(repositories *yaml.Node, owner, name string) int {
	for i, node := range repositories.Content {
		var repository config.Repository
		if node.Decode(&repository) == nil && strings.EqualFold(repository.Owner, owner) && strings.EqualFold(repository.Name, name) {
			return i
		}
	}
	return -1
}

func mappingValue(mapping *yaml.Node, key string) (*yaml.Node, error) {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return nil, errors.New("expected YAML mapping")
	}
	for i := 0; i < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i+1], nil
		}
	}
	return nil, fmt.Errorf("missing YAML field %s", key)
}

func validateConfigDocument(document *yaml.Node) error {
	var encoded bytes.Buffer
	encoder := yaml.NewEncoder(&encoded)
	encoder.SetIndent(2)
	if err := encoder.Encode(document); err != nil {
		return err
	}
	_, err := config.Parse(encoded.Bytes())
	return err
}
