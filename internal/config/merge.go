package config

func mergeDocument(base, overlay map[string]any) map[string]any {
	// Maps merge recursively while scalars and lists are replaced by the later layer.
	result := make(map[string]any, len(base)+len(overlay))
	for key, value := range base {
		result[key] = value
	}
	for key, value := range overlay {
		if baseMap, ok := result[key].(map[string]any); ok {
			if overlayMap, ok := value.(map[string]any); ok {
				result[key] = mergeDocument(baseMap, overlayMap)
				continue
			}
		}
		result[key] = value
	}
	return result
}
