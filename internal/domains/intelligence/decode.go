package intelligence

import (
	"bytes"
	"encoding/json"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	"io"
)

func decodeStrict(b []byte, v any) error {
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if d.Decode(v) != nil {
		return database.ErrInvalid
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return database.ErrInvalid
	}
	return nil
}
