package config

// MetricsReceiverLayer creates the final configuration layer that selects the
// receivers attached to the Collector metrics pipeline.
func MetricsReceiverLayer(receivers []string) InlineLayer {
	values := make([]any, len(receivers))

	for index, receiver := range receivers {
		values[index] = receiver
	}

	return InlineLayer{
		Name: "managed dependency receivers",
		Document: map[string]any{
			"service": map[string]any{
				"pipelines": map[string]any{
					"metrics": map[string]any{
						"receivers": values,
					},
				},
			},
		},
	}
}
