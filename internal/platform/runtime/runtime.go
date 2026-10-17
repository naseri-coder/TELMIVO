package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/domains/contentplugins"
	"github.com/naseri-coder/TELMIVO/internal/domains/downloads"
	"github.com/naseri-coder/TELMIVO/internal/domains/groupbot"
	"github.com/naseri-coder/TELMIVO/internal/domains/identity"
	"github.com/naseri-coder/TELMIVO/internal/domains/mainbot"
	"github.com/naseri-coder/TELMIVO/internal/integrations/media"
	telegramclient "github.com/naseri-coder/TELMIVO/internal/integrations/telegram"
	"github.com/naseri-coder/TELMIVO/internal/platform/cache"
	"github.com/naseri-coder/TELMIVO/internal/platform/config"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	"github.com/naseri-coder/TELMIVO/internal/platform/httpapi"
	"github.com/naseri-coder/TELMIVO/internal/platform/installationlicense"
	sdk "github.com/naseri-coder/TELMIVO/internal/platform/plugincontract"
	"github.com/naseri-coder/TELMIVO/internal/platform/queue"
	"github.com/naseri-coder/TELMIVO/internal/platform/storage"
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
	// Fail before signing, contacting Authority or opening network sockets:
	// each process sees only its own DATABASE_URL, not both credentials.
	var expectedDatabaseRole string
	if c.DatabaseRoleSplitRequired {
		if e = config.ValidateDatabaseRoleDSN(role, c.DatabaseURL, c.Environment); e != nil {
			return e
		}
		expectedDatabaseRole, e = config.ExpectedDatabaseRole(role)
		if e != nil {
			return e
		}
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	license, licenseClient, cleanupLicense, strictTestStartup, licenseErr := openStartupLicense(role, c)
	defer cleanupLicense()
	if strictTestStartup && licenseErr != nil {
		// Deliberately fail before the API listens; test-tagged binaries must
		// never serve requests with a synthetic or absent license by fallback.
		return licenseErr
	}
	if licenseErr != nil {
		license = nil
		licenseClient = nil
	}
	ctx = installationlicense.RequireContext(ctx)
	installationlicense.InstallRuntime(license)
	if licenseErr != nil {
		slog.Warn("installation restricted", "error_code", "LICENSE_CONFIGURATION_INVALID")
	}
	if license != nil {
		ctx = installationlicense.WithManager(ctx, license)
	}
	if licenseClient != nil {
		licenseDone := make(chan struct{})
		go func() { defer close(licenseDone); licenseClient.Poll(ctx) }()
		defer func() { stop(); <-licenseDone }()
	}

	// In explicitly required split mode the pool authenticates current_user
	// and non-elevated role flags on every new physical PostgreSQL connection.
	// No shared telmivo_runtime credential can silently serve both API/Worker.
	var db *database.Store
	if expectedDatabaseRole != "" {
		db, e = database.OpenForExpectedRole(ctx, c.DatabaseURL, expectedDatabaseRole)
	} else {
		db, e = database.Open(ctx, c.DatabaseURL, true)
	}
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
	// A restricted installation still serves read-only license diagnostics.
	// Stream creation must be denied by the signed RuntimeEffect gate;
	// suppress ONLY that denial, never broker/configuration failures.
	if e = q.Ensure(ctx); e != nil && !errors.Is(e, installationlicense.ErrRestricted) {
		return e
	}
	ident := &identity.Service{DB: db, Admins: c.AdminIDs, SessionTTL: c.SessionTTL}
	groupClient := telegramclient.New(c.GroupBotToken, !c.Synthetic)
	groups := &groupbot.Service{DB: db, Identity: ident, Authority: groupClient, Transport: groupClient, MiniAppURL: c.MiniAppURL}
	if c.GroupTestAuthority {
		groups.Authority = testfixtures.GroupAuthority{}
	}
	api := &httpapi.API{DB: db, Cache: redis, Queue: q, Config: c, Identity: ident, Groups: groups}
	if c.BotFactoryTestFixture && role == "api" {
		registry, err := testfixtures.FactoryRegistry()
		if err != nil {
			return err
		}
		api.SetFactoryRegistry(registry)
	}
	if c.ContentTestFixture && role == "api" {
		extra := []sdk.Manifest{}
		if c.BotFactoryTestFixture {
			fixture, e := testfixtures.FactoryRegistry()
			if e != nil {
				return e
			}
			for _, id := range fixture.IDs() {
				m, _ := fixture.Manifest(id)
				extra = append(extra, m)
			}
		}
		registry, e := contentplugins.Registry(extra...)
		if e != nil {
			return e
		}
		api.SetFactoryRegistry(registry)
	}
	if c.DownloaderTestFixture && role == "api" {
		local, err := storage.NewLocal("/tmp/telmivo-r3c-fixture-storage", c.MaxMediaBytes)
		if err != nil {
			return err
		}
		defer local.Close()
		fixtureWorker := &downloads.Worker{Product: &downloads.Product{DB: db, Synthetic: true, MaxBytes: c.MaxMediaBytes}, Adapter: testfixtures.DownloaderProvider{}, Delivery: testfixtures.DownloaderDelivery{}, Storage: storage.Scoped{Provider: local}, Root: "/tmp/telmivo-r3c-fixture-work", Owner: "test-api-worker", Timeout: 5 * time.Minute, MaxBytes: c.MaxMediaBytes}
		// Only the synthetic browser customer belongs to this injected worker.
		// Native recovery fixtures must remain abandoned until the restore drill.
		fixtureWorker.DispatchFilter = func(ctx context.Context, u, ws string) bool {
			var telegram int64
			err := db.Pool.QueryRow(ctx, "SELECT telegram_id FROM telegram_accounts WHERE user_id=$1", u).Scan(&telegram)
			allowed := err == nil && telegram == 98001001
			slog.Info("downloader diagnostic", "event", "downloader_fixture_account_lookup", "coordinate_id", downloads.CoordinateDiagnosticID(u, ws), "lookup_succeeded", err == nil, "allowed", allowed)
			return allowed
		}

		go func() {
			if err := fixtureWorker.Poll(ctx, 2); err != nil && ctx.Err() == nil {
				slog.Error("fixture worker stopped", "error_code", "FIXTURE_WORKER_FAILED")
			}
		}()
	}
	if role == "media-worker" && !c.Synthetic {
		local, err := storage.NewLocal(c.MediaStorageRoot, c.MaxMediaBytes)
		if err != nil {
			return err
		}
		defer local.Close()
		if installationlicense.CheckRuntime(ctx) == nil {
			if err = local.Cleanup(ctx); err != nil {
				return err
			}
		}
		worker := &downloads.Worker{Product: &downloads.Product{DB: db, MaxBytes: c.MaxMediaBytes}, Adapter: media.Extractor{Python: "/opt/extractor/bin/python", Script: "/app/extract.py", Timeout: time.Minute, MaxFormats: 8}, Delivery: telegramclient.MediaClient{Token: c.DownloaderBotToken, MaxBytes: c.MaxMediaBytes, Timeout: time.Minute}, Storage: storage.Scoped{Provider: local}, Root: c.MediaWorkRoot, Owner: "media-worker", Timeout: 5 * time.Minute, MaxBytes: c.MaxMediaBytes}
		return worker.Poll(ctx, int(c.MediaConcurrency))
	}
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
			if (role != "main-bot" && role != "group-bot" && role != "downloader-bot") || c.WebhookSecret == "" || (role == "main-bot" && c.BotToken == "") || (role == "group-bot" && c.GroupBotToken == "") || (role == "downloader-bot" && c.DownloaderBotToken == "") {
				w.WriteHeader(503)
				return
			}
			w.WriteHeader(200)
		})
		if role == "main-bot" && !c.Synthetic {
			client := telegramclient.New(c.BotToken, true)
			mux.Handle("POST /webhook", telegramclient.MainBotWebhook(c.WebhookSecret, &mainbot.Service{Identity: ident, MiniAppURL: c.MiniAppURL, Membership: client}))
		} else if role == "downloader-bot" && !c.Synthetic {
			mux.Handle("POST /webhook", telegramclient.DownloaderWebhook(c.WebhookSecret, &downloads.Bot{Identity: ident, Product: &downloads.Product{DB: db, MaxBytes: c.MaxMediaBytes}, MiniAppURL: c.MiniAppURL}))
		} else if role == "group-bot" && !c.Synthetic {
			mux.Handle("POST /webhook", telegramclient.GroupBotWebhook(c.WebhookSecret, groups))
		} else {
			mux.HandleFunc("POST /webhook", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(501) })
		}
		handler = mux
	}
	server := &http.Server{Addr: c.HTTPAddr, Handler: installationlicense.Middleware(license, handler), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
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
	// Five worker producers can finish together during shutdown.
	errorsCh := make(chan error, 5)
	if role == "worker" {
		service := (&httpapi.API{DB: db, Config: c}).IntelligenceService()
		wg.Add(2)
		go func() { defer wg.Done(); errorsCh <- service.Poll(ctx) }()
		wg.Add(1)
		go func() { defer wg.Done(); errorsCh <- (&httpapi.API{DB: db, Config: c}).BusinessService().Poll(ctx) }()
		go func() {
			defer wg.Done()
			errorsCh <- q.Consume(ctx, db, "telmivo.events.business", "automation-events-v1", service.DispatchTx)
		}()
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
		consume := q.Consume
		if role == "worker" && !c.Synthetic {
			consume = q.ConsumeExternal
		}
		errorsCh <- consume(ctx, db, subject, durable, func(ctx context.Context, tx pgx.Tx, e queue.Envelope) error {
			if e.Type == "group.action" {
				client := telegramclient.New(c.GroupBotToken, !c.Synthetic)
				s := &groupbot.Service{DB: db, Authority: client, Transport: client}
				return s.Deliver(ctx, tx, e.WorkspaceID, e.CorrelationID, e.Payload)
			}
			if e.Type == "group.reply" && !c.Synthetic {
				var p struct {
					Chat   int64  `json:"chat_id"`
					UserID string `json:"user_id"`
					Body   string `json:"body"`
					Key    string `json:"idempotency_key"`
				}
				if json.Unmarshal(e.Payload, &p) != nil || p.Chat >= 0 || p.UserID == "" || p.Key == "" {
					return database.ErrInvalid
				}
				messageID, code, retryAfter := telegramclient.New(c.GroupBotToken, true).SendClassified(
					ctx,
					telegramtransport.Message{ChatID: p.Chat, Text: p.Body},
				)
				return persistNotificationOutcome(ctx, tx, e.WorkspaceID, e.Type, notificationOutcome{
					UserID: p.UserID,
					Key:    p.Key,
					Body:   p.Body,
					ChatID: p.Chat,
				}, messageID, code, retryAfter)
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
				messageID, code, retryAfter := telegramclient.New(func() string {
					if strings.HasPrefix(payload.Key, "downloader-bot:") {
						return c.DownloaderBotToken
					}
					if strings.HasPrefix(payload.Key, "group-bot:") {
						return c.GroupBotToken
					}
					return c.BotToken
				}(), true).SendClassified(ctx, telegramtransport.Message{ChatID: chat, Text: payload.Body, Markup: payload.Markup})
				return persistNotificationOutcome(ctx, tx, e.WorkspaceID, e.Type, notificationOutcome{
					UserID: payload.UserID,
					Key:    payload.Key,
					Body:   payload.Body,
					Markup: payload.Markup,
					ChatID: chat,
				}, messageID, code, retryAfter)
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
