package supportplugins

import (
	"context"
	"fmt"
	"github.com/naseri-coder/TELMIVO/internal/domains/contentplugins"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	sdk "github.com/naseri-coder/TELMIVO/internal/platform/plugincontract"
	tickets "github.com/naseri-coder/TELMIVO/internal/plugins/ticketing"
	"strings"
)

func (s *Service) HandleSupport(ctx context.Context, scope sdk.Scope, corr string, u contentplugins.Update) error {
	m := u.Message
	if m == nil || m.Chat.Type != "private" || m.From.ID <= 0 || m.Chat.ID != m.From.ID || u.ID < 0 {
		return database.ErrInvalid
	}
	text := strings.TrimSpace(m.Text)
	if text == "" {
		text = strings.TrimSpace(m.Caption)
	}
	if strings.HasPrefix(text, "/support") {
		text = strings.TrimSpace(strings.TrimPrefix(text, "/support"))
	}
	a := []tickets.Attachment{}
	var media *contentplugins.Media
	kind := ""
	if m.Document != nil {
		media, kind = m.Document, "document"
	} else if m.Video != nil {
		media, kind = m.Video, "video"
	} else if m.Audio != nil {
		media, kind = m.Audio, "audio"
	} else if len(m.Photos) > 0 {
		media, kind = &m.Photos[len(m.Photos)-1], "photo"
	}
	if media != nil {
		title := media.Name
		if title == "" {
			title = "پیوست تلگرام"
		}
		a = append(a, tickets.Attachment{FileID: media.FileID, UniqueID: media.UniqueID, Kind: kind, Title: title})
		if text == "" {
			text = "پیوست پشتیبانی"
		}
	}
	if text == "" {
		text = "درخواست پشتیبانی"
	}
	name := m.From.Name
	if name == "" {
		name = "مشتری تلگرام"
	}
	_, e := s.Inbound(ctx, scope, tickets.Inbound{User: m.From.ID, Name: name, Username: m.From.Username, Locale: m.From.Locale, Body: text, Key: fmt.Sprintf("support-tg:%d", u.ID), Attachments: a})
	return e
}
