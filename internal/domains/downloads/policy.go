package downloads

import (
	"errors"
	"net/url"
	"strings"
)

var transitions = map[string][]string{"QUEUED": {"ANALYZING", "CANCELLED", "FAILED"}, "ANALYZING": {"AWAITING_FORMAT", "DOWNLOADING", "FAILED", "CANCELLED"}, "AWAITING_FORMAT": {"DOWNLOADING", "CANCELLED", "FAILED"}, "DOWNLOADING": {"PROCESSING", "UPLOADING", "FAILED", "CANCELLED"}, "PROCESSING": {"UPLOADING", "FAILED", "CANCELLED"}, "UPLOADING": {"COMPLETED", "FAILED", "CANCELLED"}, "FAILED": {"QUEUED"}}

func Transition(from, to string) error {
	for _, v := range transitions[from] {
		if v == to {
			return nil
		}
	}
	return errors.New("invalid download transition")
}
func Provider(raw string) (string, error) {
	u, e := url.Parse(raw)
	if e != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" {
		return "", errors.New("unsupported URL")
	}
	h := strings.ToLower(u.Hostname())
	switch h {
	case "youtube.com", "www.youtube.com", "youtu.be":
		return "youtube", nil
	case "instagram.com", "www.instagram.com":
		return "instagram", nil
	case "facebook.com", "www.facebook.com":
		return "facebook", nil
	}
	return "", errors.New("unsupported provider")
}

type Format struct {
	ID        string `json:"id"`
	Quality   string `json:"quality"`
	Kind      string `json:"kind"`
	SizeBytes int64  `json:"size_bytes"`
}

func Select(formats []Format, id string) (Format, error) {
	for _, f := range formats {
		if f.ID == id {
			return f, nil
		}
	}
	return Format{}, errors.New("unavailable format")
}
