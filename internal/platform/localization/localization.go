package localization

import (
	"embed"
	"encoding/json"
)

//go:embed fa.json
var files embed.FS

func Text(key string) string {
	b, _ := files.ReadFile("fa.json")
	var m map[string]string
	_ = json.Unmarshal(b, &m)
	if v := m[key]; v != "" {
		return v
	}
	return m["internal"]
}
