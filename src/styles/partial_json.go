package styles

import (
	"encoding/json"
	"maps"
)

type PartialJSON map[string]json.RawMessage

func ParsePartialJSON(data []byte) (PartialJSON, error) {
	var pj PartialJSON
	err := json.Unmarshal(data, &pj)
	return pj, err
}

func PartiallyMarshalJSON(obj any) (PartialJSON, error) {
	// todo find a way to avoid double marshal
	data, err := json.Marshal(obj)
	if err != nil {
		return nil, err
	}
	return ParsePartialJSON(data)
}

func GetFromPartialJSON[T any](pj PartialJSON, key string) (T, error) {
	var zero T
	raw, ok := pj[key]
	if !ok {
		return zero, nil
	}
	var result T
	err := json.Unmarshal(raw, &result)
	if err != nil {
		return zero, err
	}
	return result, nil
}

func TryGetFromPartialJSON[T any](pj PartialJSON, key string) T {
	var zero T
	raw, ok := pj[key]
	if !ok {
		return zero
	}
	var result T
	err := json.Unmarshal(raw, &result)
	if err != nil {
		return zero
	}
	return result
}

func (pj PartialJSON) Set(key string, value any) error {
	b, err := json.Marshal(value)
	if err != nil {
		return err
	}
	pj[key] = b
	return nil
}

func (pj PartialJSON) Clone() PartialJSON {
	clone := make(PartialJSON)
	maps.Copy(clone, pj)
	return clone
}

func (pj PartialJSON) CloneWith(key string, value any) (PartialJSON, error) {
	clone := pj.Clone()
	err := clone.Set(key, value)
	if err != nil {
		return nil, err
	}
	return clone, nil
}

func (pj PartialJSON) Marshal() ([]byte, error) {
	return json.Marshal(pj)
}
