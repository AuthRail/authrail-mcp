package gateway

import (
	"bytes"
	"encoding/json"
	"github.com/google/jsonschema-go/jsonschema"
	"io"
	"math/big"
	"net/url"
	"strconv"
)

// ValidateReviewedSchema applies the saved human-reviewed contract without network resolution.
// Keep original numeric literals: decoding via float64 can change approved business values.
func ValidateReviewedSchema(rawSchema, rawValue json.RawMessage) error {
	if len(rawSchema) == 0 || len(rawSchema) > maxBody/2 || len(rawValue) == 0 || len(rawValue) > maxBody/2 {
		return errBoundary
	}
	// The shared review parser refuses integer schema constants outside JS-safe range.
	// Match that boundary before this library could decode a schema constant via float64.
	schemaDecoder := json.NewDecoder(bytes.NewReader(rawSchema))
	schemaDecoder.UseNumber()
	var schemaValue any
	if schemaDecoder.Decode(&schemaValue) != nil || !reviewableSchemaNumbers(schemaValue, 0) {
		return errBoundary
	}
	var schema jsonschema.Schema
	if json.Unmarshal(rawSchema, &schema) != nil {
		return errBoundary
	}
	resolved, err := schema.Resolve(&jsonschema.ResolveOptions{Loader: func(*url.URL) (*jsonschema.Schema, error) { return nil, errBoundary }})
	if err != nil {
		return errBoundary
	}
	var value any
	decoder := json.NewDecoder(bytes.NewReader(rawValue))
	decoder.UseNumber()
	if decoder.Decode(&value) != nil || decoder.Decode(&struct{}{}) != io.EOF {
		return errBoundary
	}
	value, err = reviewedNumericValues(value, 0)
	if err != nil || resolved.Validate(value) != nil {
		return errBoundary
	}
	return nil
}

func reviewableSchemaNumbers(value any, depth int) bool {
	if depth > 32 {
		return false
	}
	switch v := value.(type) {
	case json.Number:
		number, ok := new(big.Rat).SetString(v.String())
		if !ok {
			return false
		}
		if number.IsInt() {
			bound := big.NewInt(9007199254740991)
			return new(big.Int).Abs(number.Num()).Cmp(bound) <= 0
		}
	case map[string]any:
		for _, child := range v {
			if !reviewableSchemaNumbers(child, depth+1) {
				return false
			}
		}
	case []any:
		for _, child := range v {
			if !reviewableSchemaNumbers(child, depth+1) {
				return false
			}
		}
	}
	return true
}

// The pinned validator classifies json.Number as string. Convert only when the
// original numeric value survives its supported Go representation exactly.
func reviewedNumericValues(value any, depth int) (any, error) {
	if depth > 64 {
		return nil, errBoundary
	}
	switch v := value.(type) {
	case json.Number:
		rat, ok := new(big.Rat).SetString(v.String())
		if !ok {
			return nil, errBoundary
		}
		if rat.IsInt() {
			if rat.Num().IsInt64() {
				return rat.Num().Int64(), nil
			}
			if rat.Num().IsUint64() {
				return rat.Num().Uint64(), nil
			}
			return nil, errBoundary
		}
		number, err := strconv.ParseFloat(v.String(), 64)
		if err != nil {
			return nil, errBoundary
		}
		encoded, err := json.Marshal(number)
		if err != nil {
			return nil, errBoundary
		}
		roundtrip, ok := new(big.Rat).SetString(string(encoded))
		if !ok || rat.Cmp(roundtrip) != 0 {
			return nil, errBoundary
		}
		return number, nil
	case map[string]any:
		for key, item := range v {
			converted, err := reviewedNumericValues(item, depth+1)
			if err != nil {
				return nil, err
			}
			v[key] = converted
		}
		return v, nil
	case []any:
		for index, item := range v {
			converted, err := reviewedNumericValues(item, depth+1)
			if err != nil {
				return nil, err
			}
			v[index] = converted
		}
		return v, nil
	default:
		return value, nil
	}
}
