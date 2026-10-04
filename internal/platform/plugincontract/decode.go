package plugincontract

import (
	"bytes"
	"encoding/json"
	"io"
)

// DecodeManifest rejects unknown fields and ambiguous duplicate JSON object keys.
// Extensions are typed references; arbitrary commercial code/config is never decoded.
func DecodeManifest(raw []byte, coreVersion string) (Manifest, error) {
	var m Manifest
	if len(raw) > 65536 || !uniqueJSON(raw) {
		return m, ErrManifest
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&m) != nil {
		return m, ErrManifest
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return m, ErrManifest
	}
	if err := m.Validate(coreVersion); err != nil {
		return m, err
	}
	return m, nil
}

func uniqueJSON(raw []byte) bool {
	d := json.NewDecoder(bytes.NewReader(raw))
	var value func() bool
	value = func() bool {
		t, err := d.Token()
		if err != nil {
			return false
		}
		if delim, ok := t.(json.Delim); ok {
			switch delim {
			case '{':
				seen := map[string]bool{}
				for d.More() {
					key, err := d.Token()
					k, ok := key.(string)
					if err != nil || !ok || seen[k] {
						return false
					}
					seen[k] = true
					if !value() {
						return false
					}
				}
				end, err := d.Token()
				return err == nil && end == json.Delim('}')
			case '[':
				for d.More() {
					if !value() {
						return false
					}
				}
				end, err := d.Token()
				return err == nil && end == json.Delim(']')
			default:
				return false
			}
		}
		return true
	}
	if !value() {
		return false
	}
	_, err := d.Token()
	return err == io.EOF
}
