package telegram

import (
	"context"
	"errors"
	"github.com/naseri-coder/TELMIVO/internal/domains/groupbot"
)

func (c *Client) Inspect(ctx context.Context, chat, user int64) (groupbot.Member, error) {
	var out groupbot.Member
	e := c.call(ctx, "getChatMember", map[string]int64{"chat_id": chat, "user_id": user}, &out)
	if e == nil {
		switch out.Status {
		case "creator", "administrator", "member", "restricted", "left", "kicked":
		default:
			return out, groupbot.ErrAuthority
		}
	}
	return out, e
}
func (c *Client) Bot(ctx context.Context, chat int64) (int64, groupbot.Member, error) {
	var me struct {
		ID int64 `json:"id"`
	}
	if e := c.call(ctx, "getMe", map[string]any{}, &me); e != nil {
		return 0, groupbot.Member{}, e
	}
	if me.ID <= 0 {
		return 0, groupbot.Member{}, groupbot.ErrAuthority
	}
	m, e := c.Inspect(ctx, chat, me.ID)
	return me.ID, m, e
}
func (c *Client) Execute(ctx context.Context, chat, target, msg int64, a groupbot.Action, until int64) error {
	p := map[string]any{"chat_id": chat, "user_id": target}
	method := ""
	switch a {
	case groupbot.Delete:
		method = "deleteMessage"
		delete(p, "user_id")
		p["message_id"] = msg
	case groupbot.Ban:
		method = "banChatMember"
	case groupbot.Unban:
		method = "unbanChatMember"
		p["only_if_banned"] = true
	case groupbot.Restrict, groupbot.Unmute:
		method = "restrictChatMember"
		permissions := map[string]bool{}
		allow := a == groupbot.Unmute
		for _, k := range []string{"can_send_messages", "can_send_audios", "can_send_documents", "can_send_photos", "can_send_videos", "can_send_video_notes", "can_send_voice_notes", "can_send_polls", "can_send_other_messages", "can_add_web_page_previews"} {
			permissions[k] = allow
		}
		p["permissions"] = permissions
		p["use_independent_chat_permissions"] = true
		if !allow {
			p["until_date"] = until
		}
	default:
		return groupbot.ErrRights
	}
	var ok bool
	e := c.call(ctx, method, p, &ok)
	if e == nil && !ok {
		return errors.New("Telegram action not acknowledged")
	}
	return e
}
