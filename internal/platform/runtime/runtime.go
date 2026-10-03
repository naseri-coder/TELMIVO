package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/domains/downloads"
	"github.com/naseri-coder/TELMIVO/internal/domains/groupbot"
	"github.com/naseri-coder/TELMIVO/internal/domains/identity"
	"github.com/naseri-coder/TELMIVO/internal/domains/mainbot"
	telegramclient "github.com/naseri-coder/TELMIVO/internal/integrations/telegram"
	"github.com/naseri-coder/TELMIVO/internal/platform/cache"
	"github.com/naseri-coder/TELMIVO/internal/platform/config"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	"github.com/naseri-coder/TELMIVO/internal/platform/httpapi"
	"github.com/naseri-coder/TELMIVO/internal/platform/queue"
	telegramtransport "github.com/naseri-coder/TELMIVO/internal/platform/telegram"
	"github.com/naseri-coder/TELMIVO/internal/platform/testfixtures"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
)

func Main(role string) {
	if e := Run(role); e != nil && !errors.Is(e, context.Canceled) {
		slog.Error("process stopped", "process", role, "error_code", "STARTUP_OR_RUNTIME_FAILURE")
		os.Exit(1)
	}
}
func Run(role string) error {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	c, e := config.FromEnv()
	if e != nil {
		return e
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	db, e := database.Open(ctx, c.DatabaseURL, true)
	if e != nil {
		return e
	}
	defer db.Pool.Close()
	redis, e := cache.Open(c.RedisURL)
	if e != nil {
		return e
	}
	defer redis.Redis.Close()
	q, e := queue.Open(c.NATSURL)
	if e != nil {
		return e
	}
	defer q.Conn.Close()
	if e = q.Ensure(); e != nil {
		return e
	}
	ident := &identity.Service{DB: db, Admins: c.AdminIDs, SessionTTL: c.SessionTTL}
	groupClient := telegramclient.New(c.GroupBotToken, !c.Synthetic)
	groups := &groupbot.Service{DB: db, Identity: ident, Authority: groupClient, Transport: groupClient, MiniAppURL: c.MiniAppURL}
	if c.GroupTestAuthority {
		groups.Authority = testfixtures.GroupAuthority{}
	}
	api := &httpapi.API{DB: db, Cache: redis, Queue: q, Config: c, Identity: ident, Groups: groups}
	if role == "worker" || role == "media-worker" {
		return worker(ctx, role, db, q, c)
	}
	handler := api.Handler()
	if role != "api" {
		mux := http.NewServeMux()
		mux.HandleFunc("GET /health/live", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
		mux.HandleFunc("GET /health/ready", func(w http.ResponseWriter, r *http.Request) {
			if c.Synthetic {
				w.Header().Set("X-Telmivo-Mode", "synthetic-disabled")
				w.WriteHeader(200)
				return
			}
			if (role != "main-bot" && role != "group-bot") || c.WebhookSecret == "" || (role == "main-bot" && c.BotToken == "") || (role == "group-bot" && c.GroupBotToken == "") {
				w.WriteHeader(503)
				return
			}
			w.WriteHeader(200)
		})
		if role == "main-bot" && !c.Synthetic {
			client := telegramclient.New(c.BotToken, true)
			mux.Handle("POST /webhook", telegramclient.MainBotWebhook(c.WebhookSecret, &mainbot.Service{Identity: ident, MiniAppURL: c.MiniAppURL, Membership: client}))
		} else if role == "group-bot" && !c.Synthetic {
			mux.Handle("POST /webhook", telegramclient.GroupBotWebhook(c.WebhookSecret, groups))
		} else {
			mux.HandleFunc("POST /webhook", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(501) })
		}
		handler = mux
	}
	server := &http.Server{Addr: c.HTTPAddr, Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
	done := make(chan error, 1)
	go func() { done <- server.ListenAndServe() }()
	select {
	case e = <-done:
		return e
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return server.Shutdown(shutdown)
	}
}
func worker(ctx context.Context, role string, db *database.Store, q *queue.Client, c config.Config) error {
	ctx, cancel := context.WithCancel(ctx)
	var wg sync.WaitGroup
	defer func() { cancel(); wg.Wait() }()
	errorsCh := make(chan error, 2)
	if role == "worker" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					errorsCh <- ctx.Err()
					return
				case <-ticker.C:
					if e := q.Relay(ctx, db); e != nil {
						slog.Warn("outbox retry scheduled", "error_code", "OUTBOX_RELAY_FAILED")
					}
				}
			}
		}()
	}
	subject, durable := "telmivo.jobs.notification", "notifications-v1"
	if role == "media-worker" {
		subject, durable = "telmivo.jobs.download", "media-fixture-v1"
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		errorsCh <- q.Consume(ctx, db, subject, durable, func(ctx context.Context, tx pgx.Tx, e queue.Envelope) error {
			if e.Type == "group.action" {
				client := telegramclient.New(c.GroupBotToken, !c.Synthetic)
				s := &groupbot.Service{DB: db, Authority: client, Transport: client}
				return s.Deliver(ctx, tx, e.WorkspaceID, e.CorrelationID, e.Payload)
			}
			if e.Type == "group.reply" && !c.Synthetic {
				var p struct {
					Chat int64  `json:"chat_id"`
					Body string `json:"body"`
				}
				if json.Unmarshal(e.Payload, &p) != nil || p.Chat >= 0 {
					return database.ErrInvalid
				}
				_, err := telegramclient.New(c.GroupBotToken, true).Send(ctx, telegramtransport.Message{ChatID: p.Chat, Text: p.Body})
				return err
			}
			var payload struct {
				UserID        string `json:"user_id"`
				DownloadJobID string `json:"download_job_id"`
				Body          string `json:"body"`
				Markup        any    `json:"markup"`
				Key           string `json:"idempotency_key"`
			}
			if err := json.Unmarshal(e.Payload, &payload); err != nil {
				return err
			}
			if role == "media-worker" {
				if !c.Synthetic {
					return errors.New("live media disabled")
				}
				return downloads.CompleteFixture(ctx, tx, e.WorkspaceID, payload.DownloadJobID)
			}
			if !c.Synthetic {
				if e.Type != "bot.menu" {
					return errors.New("live notification type disabled")
				}
				chat, err := identity.TelegramID(ctx, tx, payload.UserID)
				if err != nil {
					return err
				}
				_, err = telegramclient.New(func() string {
					if strings.HasPrefix(payload.Key, "group-bot:") {
						return c.GroupBotToken
					}
					return c.BotToken
				}(), true).Send(ctx, telegramtransport.Message{ChatID: chat, Text: payload.Body, Markup: payload.Markup})
				return err
			}
			_, err := tx.Exec(ctx, "INSERT INTO notifications(workspace_id,user_id,message_key,parameters,status,idempotency_key) VALUES($1,$2,$3,$4,'SIMULATED',$5) ON CONFLICT(workspace_id,idempotency_key) DO NOTHING", e.WorkspaceID, payload.UserID, e.Type, map[string]any{"body": payload.Body, "markup": payload.Markup}, payload.Key)
			return err
		})
	}()
	select {
	case e := <-errorsCh:
		return e
	case <-ctx.Done():
		return ctx.Err()
	}
}
