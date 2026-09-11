package config

import "fmt"

// ValidateDocument checks the minimum Collector component and pipeline structure.
func ValidateDocument(document map[string]any) error {
	for _, key := range []string{"receivers", "processors", "exporters"} {
		value, ok := document[key].(map[string]any)
		if !ok || len(value) == 0 {
			return fmt.Errorf("%s must be a non-empty map", key)
		}
	}
	service, ok := document["service"].(map[string]any)
	if !ok {
		return fmt.Errorf("service must be a map")
	}
	pipelines, ok := service["pipelines"].(map[string]any)
	if !ok || len(pipelines) == 0 {
		return fmt.Errorf("service.pipelines must be a non-empty map")
	}
	for name, raw := range pipelines {
		pipeline, ok := raw.(map[string]any)
		if !ok {
			return fmt.Errorf("service.pipelines.%s must be a map", name)
		}
		for _, field := range []string{"receivers", "exporters"} {
			list, ok := pipeline[field].([]any)
			if !ok || len(list) == 0 {
				return fmt.Errorf("service.pipelines.%s.%s must be a non-empty list", name, field)
			}
		}
	}
	return nil
}
