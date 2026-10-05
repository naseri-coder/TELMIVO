package telegram

import (
	"context"
	"github.com/naseri-coder/TELMIVO/internal/domains/accessgates"
)

type Message struct {
	ChatID int64
	Text   string
	Markup any
}
type Messenger interface {
	Send(context.Context, Message) (int64, error)
	Copy(context.Context, int64, int64, int64) (int64, error)
	Forward(context.Context, int64, int64, int64) (int64, error)
}
type MembershipService interface {
	Membership(context.Context, int64, int64) (accessgates.Membership, error)
}
type ChannelService interface {
	CanPublish(context.Context, int64) (bool, error)
}
type GroupService interface {
	Restrict(context.Context, int64, int64) error
}
type MediaDelivery interface {
	Deliver(context.Context, int64, string) (int64, error)
}
