package wsconnector

import (
	"encoding/json"
	"io"
)

func ReadAndDecodeJson(r io.Reader, v interface{}) error {
	return json.NewDecoder(r).Decode(v)
}

func EncodeJson(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}
