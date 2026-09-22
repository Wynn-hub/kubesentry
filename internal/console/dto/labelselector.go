package dto

import (
	"bytes"
	"encoding/json"
	"fmt"
)

type LabelSelectorRequirementView struct {
	Key      string   `json:"key" binding:"required"`
	Operator string   `json:"operator" binding:"required" enums:"In,NotIn,Exists,DoesNotExist"`
	Values   []string `json:"values,omitempty"`
}

func (r *LabelSelectorRequirementView) UnmarshalJSON(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if token, err := decoder.Token(); err != nil || token != json.Delim('{') {
		return fmt.Errorf("decode label selector requirement: expected object")
	}
	var keySet, operatorSet bool
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return err
		}
		switch key {
		case "key":
			keySet = true
			var raw json.RawMessage
			if err = decoder.Decode(&raw); err == nil {
				if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
					return fmt.Errorf("key: must be a string")
				}
				err = json.Unmarshal(raw, &r.Key)
			}
		case "operator":
			operatorSet = true
			var raw json.RawMessage
			if err = decoder.Decode(&raw); err == nil {
				if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
					return fmt.Errorf("operator: must be a string")
				}
				err = json.Unmarshal(raw, &r.Operator)
				if err == nil {
					switch r.Operator {
					case "In", "NotIn", "Exists", "DoesNotExist":
					default:
						return fmt.Errorf("operator: invalid value %q", r.Operator)
					}
				}
			}
		case "values":
			r.Values = nil
			var raw json.RawMessage
			if err = decoder.Decode(&raw); err == nil {
				err = decodeNonNullJSON(raw, "values", &r.Values)
			}
		default:
			var ignored json.RawMessage
			err = decoder.Decode(&ignored)
		}
		if err != nil {
			return fmt.Errorf("decode field %q: %w", key, err)
		}
	}
	if _, err := decoder.Token(); err != nil {
		return err
	}
	if !keySet {
		return fmt.Errorf("key: field is required")
	}
	if !operatorSet {
		return fmt.Errorf("operator: field is required")
	}
	return nil
}

type LabelSelectorView struct {
	MatchLabels      map[string]string              `json:"matchLabels,omitempty"`
	MatchExpressions []LabelSelectorRequirementView `json:"matchExpressions,omitempty"`
}

func (s *LabelSelectorView) UnmarshalJSON(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if token, err := decoder.Token(); err != nil || token != json.Delim('{') {
		return fmt.Errorf("decode label selector: expected object")
	}
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return err
		}
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			return err
		}
		switch key {
		case "matchLabels":
			s.MatchLabels = nil
			err = decodeNonNullJSON(raw, "matchLabels", &s.MatchLabels)
		case "matchExpressions":
			s.MatchExpressions = nil
			err = decodeNonNullJSON(raw, "matchExpressions", &s.MatchExpressions)
		}
		if err != nil {
			return err
		}
	}
	_, err := decoder.Token()
	return err
}

func decodeNonNullJSON(data json.RawMessage, field string, dst any) error {
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return fmt.Errorf("%s: null is not allowed", field)
	}
	if err := json.Unmarshal(data, dst); err != nil {
		return fmt.Errorf("%s: %w", field, err)
	}
	return nil
}
