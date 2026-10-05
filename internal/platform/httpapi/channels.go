package httpapi

import (
	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/domains/channels"
	tg "github.com/naseri-coder/TELMIVO/internal/integrations/telegram"
	"net/http"
)

func (a *API) ChannelService() *channels.Service {
	if a.Channels != nil {
		return a.Channels
	}
	client := tg.New(a.Config.BotToken, !a.Config.Synthetic)
	return &channels.Service{DB: a.DB, Authority: client, Publisher: client}
}
func (a *API) channelList(w http.ResponseWriter, r *http.Request) {
	a.tenant(w, r, false, func(tx pgx.Tx) (any, error) {
		return a.ChannelService().List(r.Context(), tx, user(r), workspace(r))
	})
}
func (a *API) channelConnect(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ChatID int64 `json:"chat_id"`
	}
	if decode(w, r, &in) != nil {
		fail(w, 400, "invalid")
		return
	}
	a.tenant(w, r, false, func(tx pgx.Tx) (any, error) {
		return a.ChannelService().Connect(r.Context(), tx, user(r), workspace(r), correlation(r), in.ChatID)
	})
}
func (a *API) channelVerify(w http.ResponseWriter, r *http.Request) {
	var in struct{}
	if decode(w, r, &in) != nil {
		fail(w, 400, "invalid")
		return
	}
	a.tenant(w, r, false, func(tx pgx.Tx) (any, error) {
		return a.ChannelService().Verify(r.Context(), tx, user(r), workspace(r), r.PathValue("channel"), correlation(r))
	})
}
func (a *API) channelDisconnect(w http.ResponseWriter, r *http.Request) {
	var in struct{}
	if decode(w, r, &in) != nil {
		fail(w, 400, "invalid")
		return
	}
	a.tenant(w, r, false, func(tx pgx.Tx) (any, error) {
		return a.ChannelService().Disconnect(r.Context(), tx, user(r), workspace(r), r.PathValue("channel"), correlation(r))
	})
}
func (a *API) channelPosts(w http.ResponseWriter, r *http.Request) {
	a.tenant(w, r, false, func(tx pgx.Tx) (any, error) {
		return a.ChannelService().ListPosts(r.Context(), tx, user(r), workspace(r), r.PathValue("channel"))
	})
}
func (a *API) channelPostCreate(w http.ResponseWriter, r *http.Request) {
	var in channels.CreatePost
	if decode(w, r, &in) != nil {
		fail(w, 400, "invalid")
		return
	}
	a.tenant(w, r, false, func(tx pgx.Tx) (any, error) {
		return a.ChannelService().CreatePost(r.Context(), tx, user(r), workspace(r), r.PathValue("channel"), correlation(r), in)
	})
}
func (a *API) channelPostEdit(w http.ResponseWriter, r *http.Request) {
	var in channels.EditPost
	if decode(w, r, &in) != nil {
		fail(w, 400, "invalid")
		return
	}
	a.tenant(w, r, false, func(tx pgx.Tx) (any, error) {
		return a.ChannelService().EditPost(r.Context(), tx, user(r), workspace(r), r.PathValue("channel"), r.PathValue("post"), correlation(r), in)
	})
}
func (a *API) channelPostAction(w http.ResponseWriter, r *http.Request) {
	var in channels.Action
	if decode(w, r, &in) != nil {
		fail(w, 400, "invalid")
		return
	}
	a.tenant(w, r, false, func(tx pgx.Tx) (any, error) {
		return a.ChannelService().Act(r.Context(), tx, user(r), workspace(r), r.PathValue("channel"), r.PathValue("post"), correlation(r), in)
	})
}
func (a *API) channelPostHistory(w http.ResponseWriter, r *http.Request) {
	a.tenant(w, r, false, func(tx pgx.Tx) (any, error) {
		return a.ChannelService().History(r.Context(), tx, user(r), workspace(r), r.PathValue("channel"), r.PathValue("post"))
	})
}
