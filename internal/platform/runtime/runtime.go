package runtime

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/domains/downloads"
	"github.com/naseri-coder/TELMIVO/internal/domains/identity"
	"github.com/naseri-coder/TELMIVO/internal/platform/cache"
	"github.com/naseri-coder/TELMIVO/internal/platform/config"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	"github.com/naseri-coder/TELMIVO/internal/platform/httpapi"
	"github.com/naseri-coder/TELMIVO/internal/platform/queue"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
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
	api := &httpapi.API{DB: db, Cache: redis, Queue: q, Config: c, Identity: ident}
	if role == "worker" || role == "media-worker" {
		return worker(ctx, role, db, q, c)
	}
	handler := api.Handler()
	if role != "api" {
		mux := http.NewServeMux()
		mux.HandleFunc("GET /health/live", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
		mux.HandleFunc("GET /health/ready", func(w http.ResponseWriter, r *http.Request) {
			if role != "main-bot" || c.WebhookSecret == "" {
				w.WriteHeader(503)
				return
			}
			w.WriteHeader(200)
		})
		mux.HandleFunc("POST /webhook", func(w http.ResponseWriter, r *http.Request) {
			if c.WebhookSecret == "" || subtle.ConstantTimeCompare([]byte(c.WebhookSecret), []byte(r.Header.Get("X-Telegram-Bot-Api-Secret-Token"))) != 1 {
				w.WriteHeader(401)
				return
			}
			if role != "main-bot" {
				w.WriteHeader(501)
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
			var u identity.Update
			if json.NewDecoder(r.Body).Decode(&u) != nil {
				w.WriteHeader(400)
				return
			}
			if u.Message == nil || u.Message.Text != "/start" {
				w.WriteHeader(501)
				return
			}
			if _, e := ident.Start(r.Context(), role, c.MiniAppURL, "telegram:"+identity.UpdateKey(u.ID), u, nil); e != nil {
				w.WriteHeader(503)
				return
			}
			w.WriteHeader(200)
		})
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
	errorsCh := make(chan error, 2)
	if role == "worker" {
		go func() {
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
	go func() {
		errorsCh <- q.Consume(ctx, db, subject, durable, func(ctx context.Context, tx pgx.Tx, e queue.Envelope) error {
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
				return errors.New("live notifications disabled")
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
