package v1alpha1

import "encoding/json"

// Values contains a map type data
// +kubebuilder:validation:Type=object
type Values struct {
	Data map[string]interface{} `json:"-" yaml:",inline"`
}

// MarshalJSON implements the Marshaler interface.
func (in *Values) MarshalJSON() ([]byte, error) {
	return json.Marshal(in.Data)
}

// UnmarshalJSON implements the Unmarshaler interface.
func (in *Values) UnmarshalJSON(data []byte) error {
	var out map[string]interface{}
	err := json.Unmarshal(data, &out)
	if err != nil {
		return err
	}
	in.Data = out
	return nil
}

// DeepCopyInto implements the DeepCopyInto interface.
func (in *Values) DeepCopyInto(out *Values) {
	bytes, err := json.Marshal(*in)
	if err != nil {
		panic(err)
	}
	var clone map[string]interface{}
	err = json.Unmarshal(bytes, &clone)
	if err != nil {
		panic(err)
	}
	out.Data = clone
}

// DeepCopy implements the DeepCopy interface.
func (in *Values) DeepCopy() *Values {
	if in == nil {
		return nil
	}
	out := new(Values)
	in.DeepCopyInto(out)
	return out
}
