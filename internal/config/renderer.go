package config

import (
	"fmt"
	"io"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

const maxLayerBytes = 1 << 20

func loadLayer(layer Layer) (map[string]any, error) {
	file, err := os.Open(layer.Path)
	if err != nil {
		return nil, fmt.Errorf("open %s layer %q: %w", layer.Name, layer.Path, err)
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, maxLayerBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read %s layer %q: %w", layer.Name, layer.Path, err)
	}
	if len(content) > maxLayerBytes {
		return nil, fmt.Errorf("read %s layer %q: exceeds %d bytes", layer.Name, layer.Path, maxLayerBytes)
	}
	var document map[string]any
	if err := yaml.Unmarshal(content, &document); err != nil {
		return nil, fmt.Errorf("parse %s layer %q: %w", layer.Name, layer.Path, err)
	}
	if document == nil {
		document = map[string]any{}
	}
	return document, nil
}

func Render(input RenderInput) ([]byte, error) {
	if len(input.Layers) == 0 {
		return nil, fmt.Errorf("render configuration: no layers supplied")
	}
	merged := map[string]any{}
	for _, layer := range input.Layers {
		document, err := loadLayer(layer)
		if err != nil {
			return nil, err
		}
		merged = mergeDocument(merged, document)
	}
	if err := ValidateDocument(merged); err != nil {
		return nil, fmt.Errorf("validate rendered configuration: %w", err)
	}
	rendered, err := yaml.Marshal(merged)
	if err != nil {
		return nil, fmt.Errorf("encode rendered configuration: %w", err)
	}
	if len(input.Values) > 0 {
		rendered = substituteValues(rendered, input.Values)
	}
	return rendered, nil
}

func substituteValues(document []byte, values map[string]string) []byte {
	result := string(document)
	for key, value := range values {
		result = strings.ReplaceAll(result, "${value:"+key+"}", value)
	}
	return []byte(result)
}
