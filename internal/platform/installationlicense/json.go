package installationlicense

import (
	"bytes"
	"encoding/json"
	"io"
)

func StrictJSON(b []byte, v any) error {
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	var walk func() error
	walk = func() error {
		t, e := d.Token()
		if e != nil {
			return e
		}
		del, ok := t.(json.Delim)
		if !ok {
			return nil
		}
		switch del {
		case '{':
			seen := map[string]bool{}
			for d.More() {
				k, e := d.Token()
				s, ok := k.(string)
				if e != nil || !ok || seen[s] {
					return ErrState
				}
				seen[s] = true
				if e = walk(); e != nil {
					return e
				}
			}
		case '[':
			for d.More() {
				if e := walk(); e != nil {
					return e
				}
			}
		default:
			return ErrState
		}
		_, e = d.Token()
		return e
	}
	if e := walk(); e != nil {
		return ErrState
	}
	if _, e := d.Token(); e != io.EOF {
		return ErrState
	}
	d = json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if d.Decode(v) != nil {
		return ErrState
	}
	return nil
}
