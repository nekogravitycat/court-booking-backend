package request

import "encoding/json"

// Nullable distinguishes the three states of a PATCH field: absent from the
// JSON body (Set == false), explicitly null (Set && Value == nil), or a value.
// Declare the field as a non-pointer so encoding/json calls UnmarshalJSON for
// an explicit null as well.
type Nullable[T any] struct {
	Set   bool
	Value *T
}

func (n *Nullable[T]) UnmarshalJSON(data []byte) error {
	n.Set = true
	if string(data) == "null" {
		n.Value = nil
		return nil
	}
	var v T
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	n.Value = &v
	return nil
}

// MarshalJSON encodes the value, or null for an explicit null. Declare the field
// with the omitzero tag option so an absent field is left out.
func (n Nullable[T]) MarshalJSON() ([]byte, error) {
	if n.Value == nil {
		return []byte("null"), nil
	}
	return json.Marshal(n.Value)
}
