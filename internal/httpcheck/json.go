package httpcheck

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"unicode/utf8"
)

// Token decoding catches duplicate object keys, which ordinary Unmarshal drops.
func parseJSON(body []byte) (any, error) {
	// encoding/json replaces invalid UTF-8 in strings instead of rejecting it.
	if !utf8.Valid(body) {
		return nil, errors.New("invalid JSON UTF-8")
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	value, err := readJSON(decoder, 0)
	if err != nil {
		return nil, err
	}
	if _, err = decoder.Token(); err != io.EOF {
		return nil, errors.New("trailing JSON")
	}
	return value, nil
}

func readJSON(decoder *json.Decoder, depth int) (any, error) {
	if depth > 128 {
		return nil, errors.New("JSON nesting limit")
	}
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	switch token {
	case json.Delim('{'):
		object := map[string]any{}
		for decoder.More() {
			key, err := decoder.Token()
			if err != nil {
				return nil, err
			}
			name, ok := key.(string)
			if !ok {
				return nil, errors.New("object key")
			}
			if _, exists := object[name]; exists {
				return nil, errors.New("duplicate key")
			}
			value, err := readJSON(decoder, depth+1)
			if err != nil {
				return nil, err
			}
			object[name] = value
		}
		_, err = decoder.Token()
		return object, err
	case json.Delim('['):
		list := []any{}
		for decoder.More() {
			value, err := readJSON(decoder, depth+1)
			if err != nil {
				return nil, err
			}
			list = append(list, value)
		}
		_, err = decoder.Token()
		return list, err
	default:
		return token, nil
	}
}
