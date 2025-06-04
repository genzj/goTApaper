package util

import "github.com/go-viper/mapstructure/v2"

// MapToStruct converts a interface{} (usually unmarshalled from JSON) to a concrete type using mapstructure
// Example usage:
//
//	type Person struct {
//	  Name string
//	  Age  int
//	}
//
//	data := map[string]interface{}{"name": "John", "age": 30}
//	var person Person
//	err := MapToStruct(data, &person)
func MapToStruct(input interface{}, result interface{}) error {
	config := &mapstructure.DecoderConfig{
		WeaklyTypedInput: true,
		Result:           result,
	}

	decoder, err := mapstructure.NewDecoder(config)
	if err != nil {
		return err
	}

	return decoder.Decode(input)
}
