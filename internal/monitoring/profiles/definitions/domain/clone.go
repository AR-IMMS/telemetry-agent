package domain

import "encoding/json"

// Clone returns a defensive copy of all mutable definition metadata.
func (d Definition) Clone() Definition {
	if d.TemplateSettingRules != nil {
		rules := d.TemplateSettingRules
		d.TemplateSettingRules = make(map[string]TemplateSettingRule, len(rules))
		for key, rule := range rules {
			if rule.Default != nil {
				rule.Default, _ = cloneJSONValue(rule.Default)
			}
			rule.Constraints = append([]string(nil), rule.Constraints...)
			d.TemplateSettingRules[key] = rule
		}
	}
	if d.Matching == nil {
		return d
	}
	matching := *d.Matching
	matching.RequiredCapabilities = append([]string(nil), matching.RequiredCapabilities...)
	if d.Matching.Selector != nil {
		selector := *d.Matching.Selector
		selector.ResourceKinds = append([]string(nil), selector.ResourceKinds...)
		selector.LifecycleStates = append([]string(nil), selector.LifecycleStates...)
		selector.ExactAttributes = make(map[string]string, len(d.Matching.Selector.ExactAttributes))
		for key, value := range d.Matching.Selector.ExactAttributes {
			selector.ExactAttributes[key] = value
		}
		matching.Selector = &selector
	}
	d.Matching = &matching
	return d
}

func cloneJSONValue(value any) (any, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var copied any
	if err := json.Unmarshal(encoded, &copied); err != nil {
		return nil, err
	}
	return copied, nil
}
