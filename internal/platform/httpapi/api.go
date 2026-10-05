package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/naseri-coder/TELMIVO/internal/domains/appleservices"
	"github.com/naseri-coder/TELMIVO/internal/domains/botfactory"
	"github.com/naseri-coder/TELMIVO/internal/domains/botmodules"
	"github.com/naseri-coder/TELMIVO/internal/domains/businessplugins"
	"github.com/naseri-coder/TELMIVO/internal/domains/channels"
	"github.com/naseri-coder/TELMIVO/internal/domains/commerce"
	"github.com/naseri-coder/TELMIVO/internal/domains/contentplugins"
	"github.com/naseri-coder/TELMIVO/internal/domains/downloads"
	"github.com/naseri-coder/TELMIVO/internal/domains/entitlements"
	"github.com/naseri-coder/TELMIVO/internal/domains/groupbot"
	"github.com/naseri-coder/TELMIVO/internal/domains/identity"
	"github.com/naseri-coder/TELMIVO/internal/domains/intelligence"
	"github.com/naseri-coder/TELMIVO/internal/domains/policies"
	"github.com/naseri-coder/TELMIVO/internal/domains/subscriptions"
	"github.com/naseri-coder/TELMIVO/internal/domains/supportplugins"
	"github.com/naseri-coder/TELMIVO/internal/domains/tickets"
	"github.com/naseri-coder/TELMIVO/internal/domains/uploads"
	"github.com/naseri-coder/TELMIVO/internal/domains/wallets"
	"github.com/naseri-coder/TELMIVO/internal/integrations/media"
	"github.com/naseri-coder/TELMIVO/internal/platform/auth"
	"github.com/naseri-coder/TELMIVO/internal/platform/cache"
	"github.com/naseri-coder/TELMIVO/internal/platform/config"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	"github.com/naseri-coder/TELMIVO/internal/platform/entitlement"
	"github.com/naseri-coder/TELMIVO/internal/platform/localization"
	sdk "github.com/naseri-coder/TELMIVO/internal/platform/plugincontract"
	"github.com/naseri-coder/TELMIVO/internal/platform/queue"
	"github.com/naseri-coder/TELMIVO/internal/platform/storage"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

type API struct {
	Apple        *appleservices.Service
	Business     *businessplugins.Service
	Channels     *channels.Service
	Intelligence *intelligence.Service
	DB           *database.Store
	Factory      *botfactory.Service
	Support      *supportplugins.Service
	Content      *contentplugins.Service
	Cache        *cache.Client
	Queue        *queue.Client
	Config       config.Config
	Identity     *identity.Service
	Requests     atomic.Uint64
	Groups       *groupbot.Service
	Storage      *storage.Scoped
	Uploads      *uploads.Service
}
type ctxKey string

const userKey ctxKey = "user"
const requestKey ctxKey = "request"

func (a *API) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health/live", func(w http.ResponseWriter, r *http.Request) { respond(w, 200, map[string]string{"status": "live"}) })
	mux.HandleFunc("GET /health/ready", a.ready)
	mux.HandleFunc("POST /api/v1/auth/telegram", a.login)
	mux.HandleFunc("POST /api/v1/customer-bot-updates/{account}/{bot}", a.contentWebhook)
	protected := http.NewServeMux()
	protected.HandleFunc("GET /api/v1/customer-bots/{bot}/store", a.storeList)
	protected.HandleFunc("GET /api/v1/customer-bots/{bot}/store/{plugin}", a.storeDetail)
	protected.HandleFunc("POST /api/v1/commercial/effective", a.storeEffective)
	protected.HandleFunc("GET /api/v1/commercial/audit", a.commercialAudit)
	protected.HandleFunc("GET /api/v1/customer-bots/{bot}/business/{plugin}/definitions", a.businessDefinitions)
	protected.HandleFunc("POST /api/v1/customer-bots/{bot}/business/{plugin}/definitions", a.businessSave)
	protected.HandleFunc("POST /api/v1/customer-bots/{bot}/business/{plugin}/definitions/{definition}", a.businessSave)
	protected.HandleFunc("POST /api/v1/customer-bots/{bot}/business/{plugin}/resources/{resource}/actions", a.businessAction)
	protected.HandleFunc("POST /api/v1/customer-bots/{bot}/business/broadcast/preview", a.businessPreview)
	protected.HandleFunc("GET /api/v1/customer-bots/{bot}/business/broadcast/campaigns/{campaign}", a.businessCampaign)
	protected.HandleFunc("GET /api/v1/customer-bots/{bot}/business/broadcast/campaigns/{campaign}/recipients", a.businessRecipients)
	protected.HandleFunc("GET /api/v1/customer-bots/{bot}/business/broadcast/campaigns/{campaign}/history", a.businessCampaignHistory)
	protected.HandleFunc("GET /api/v1/customer-bots/{bot}/business/forms/submissions/{form}", a.businessSubmissions)
	protected.HandleFunc("GET /api/v1/customer-bots/{bot}/business/commerce/orders", a.businessOrders)
	protected.HandleFunc("GET /api/v1/customer-bots/{bot}/business/commerce/orders/{order}/history", a.businessOrderHistory)
	protected.HandleFunc("POST /api/v1/customer-bots/{bot}/business/commerce/orders/{order}/checkout", a.businessCheckout)
	protected.HandleFunc("GET /api/v1/customer-bots/{bot}/business/commerce/payments", a.businessPayments)
	protected.HandleFunc("GET /api/v1/customer-bots/{bot}/business/commerce/payments/{payment}/history", a.businessPaymentHistory)
	protected.HandleFunc("POST /api/v1/customer-bots/{bot}/business/commerce/payments/{payment}/refund", a.businessRefund)
	protected.HandleFunc("POST /api/v1/customer-bots/{bot}/business/commerce/payments/{payment}/reconcile", a.businessReconcile)
	protected.HandleFunc("GET /api/v1/customer-bots/{bot}/automation/options", a.intelligenceOptions)
	protected.HandleFunc("GET /api/v1/customer-bots/{bot}/automation/workflows", a.intelligenceWorkflows)
	protected.HandleFunc("POST /api/v1/customer-bots/{bot}/automation/workflows", a.intelligenceWorkflowSave)
	protected.HandleFunc("POST /api/v1/customer-bots/{bot}/automation/workflows/{workflow}", a.intelligenceWorkflowSave)
	protected.HandleFunc("POST /api/v1/customer-bots/{bot}/automation/preview", a.intelligencePreview)
	protected.HandleFunc("POST /api/v1/customer-bots/{bot}/automation/workflows/{workflow}/run", a.intelligenceManual)
	protected.HandleFunc("POST /api/v1/customer-bots/{bot}/automation/executions/{execution}/run", a.intelligenceRun)
	protected.HandleFunc("GET /api/v1/customer-bots/{bot}/automation/history", a.intelligenceHistory)
	protected.HandleFunc("GET /api/v1/customer-bots/{bot}/assistant", a.intelligenceConfig)
	protected.HandleFunc("POST /api/v1/customer-bots/{bot}/assistant", a.intelligenceConfigSave)
	protected.HandleFunc("GET /api/v1/customer-bots/{bot}/assistant/models", a.intelligenceModels)
	protected.HandleFunc("POST /api/v1/platform/ai-models", a.intelligenceModelSave)
	protected.HandleFunc("GET /api/v1/customer-bots/{bot}/assistant/knowledge", a.intelligenceKnowledge)
	protected.HandleFunc("POST /api/v1/customer-bots/{bot}/assistant/knowledge", a.intelligenceKnowledgeSave)
	protected.HandleFunc("POST /api/v1/customer-bots/{bot}/assistant/ingest", a.intelligenceIngest)
	protected.HandleFunc("POST /api/v1/customer-bots/{bot}/assistant/chat", a.intelligenceChat)
	protected.HandleFunc("POST /api/v1/customer-bots/{bot}/assistant/threads/{thread}/reset", a.intelligenceReset)
	protected.HandleFunc("GET /api/v1/customer-bots/{bot}/assistant/usage", a.intelligenceUsage)

	protected.HandleFunc("GET /api/v1/apple-services/profile", a.appleProfile)
	protected.HandleFunc("POST /api/v1/apple-services/profile", a.appleProfileEnsure)
	protected.HandleFunc("POST /api/v1/apple-services/profile/actions", a.appleProfileAction)
	protected.HandleFunc("GET /api/v1/apple-services/operations", a.appleOperations)
	protected.HandleFunc("POST /api/v1/apple-services/operations", a.appleOperationCreate)
	protected.HandleFunc("POST /api/v1/apple-services/operations/{operation}/actions", a.appleOperationAction)
	protected.HandleFunc("GET /api/v1/apple-services/operations/{operation}/events", a.appleOperationEvents)

	protected.HandleFunc("GET /api/v1/uploader/workflows", a.uploadWorkflows)
	protected.HandleFunc("POST /api/v1/uploader/workflows", a.uploadWorkflowSave)
	protected.HandleFunc("POST /api/v1/uploader/workflows/{workflow}", a.uploadWorkflowSave)
	protected.HandleFunc("GET /api/v1/uploader/uploads", a.uploadList)
	protected.HandleFunc("POST /api/v1/uploader/uploads", a.uploadCreate)
	protected.HandleFunc("PUT /api/v1/uploader/uploads/{upload}/content", a.uploadContent)
	protected.HandleFunc("POST /api/v1/uploader/uploads/{upload}/actions", a.uploadAction)
	protected.HandleFunc("GET /api/v1/uploader/uploads/{upload}/content", a.uploadOpen)
	protected.HandleFunc("GET /api/v1/uploader/uploads/{upload}/signed-url", a.uploadSignedURL)
	protected.HandleFunc("POST /api/v1/uploader/uploads/{upload}/file-manager", a.uploadHandoff)

	protected.HandleFunc("GET /api/v1/channels", a.channelList)
	protected.HandleFunc("POST /api/v1/channels/connect", a.channelConnect)
	protected.HandleFunc("POST /api/v1/channels/{channel}/verify", a.channelVerify)
	protected.HandleFunc("POST /api/v1/channels/{channel}/disconnect", a.channelDisconnect)
	protected.HandleFunc("GET /api/v1/channels/{channel}/posts", a.channelPosts)
	protected.HandleFunc("POST /api/v1/channels/{channel}/posts", a.channelPostCreate)
	protected.HandleFunc("POST /api/v1/channels/{channel}/posts/{post}", a.channelPostEdit)
	protected.HandleFunc("POST /api/v1/channels/{channel}/posts/{post}/actions", a.channelPostAction)
	protected.HandleFunc("GET /api/v1/channels/{channel}/posts/{post}/history", a.channelPostHistory)

	protected.HandleFunc("GET /api/v1/support-bots", a.supportBots)
	protected.HandleFunc("GET /api/v1/customer-bots/{bot}/crm/contacts", a.supportContacts)
	protected.HandleFunc("POST /api/v1/customer-bots/{bot}/crm/contacts", a.supportContactCreate)
	protected.HandleFunc("GET /api/v1/customer-bots/{bot}/crm/contacts/{contact}", a.supportContact)
	protected.HandleFunc("POST /api/v1/customer-bots/{bot}/crm/contacts/{contact}", a.supportContactSave)
	protected.HandleFunc("GET /api/v1/customer-bots/{bot}/support/tickets", a.supportTickets)
	protected.HandleFunc("GET /api/v1/customer-bots/{bot}/support/tickets/{ticket}", a.supportTicket)
	protected.HandleFunc("POST /api/v1/customer-bots/{bot}/support/tickets/{ticket}/actions", a.supportAction)
	protected.HandleFunc("POST /api/v1/customer-bots/{bot}/support/tickets/{ticket}/messages", a.supportMessage)
	protected.HandleFunc("GET /api/v1/customer-bots/{bot}/support/departments", a.supportDepartments)
	protected.HandleFunc("POST /api/v1/customer-bots/{bot}/support/departments", a.supportDepartmentSave)
	protected.HandleFunc("POST /api/v1/customer-bots/{bot}/support/departments/{department}", a.supportDepartmentSave)
	protected.HandleFunc("GET /api/v1/customer-bots/{bot}/support/templates", a.supportTemplates)
	protected.HandleFunc("POST /api/v1/customer-bots/{bot}/support/templates", a.supportTemplateSave)
	protected.HandleFunc("POST /api/v1/customer-bots/{bot}/support/templates/{template}", a.supportTemplateSave)
	protected.HandleFunc("GET /api/v1/customer-bots/{bot}/support/agents", a.supportAgents)
	protected.HandleFunc("POST /api/v1/customer-bots/{bot}/access-policies/{policy}/check", a.contentPolicyCheck)
	protected.HandleFunc("GET /api/v1/customer-bots/{bot}/access-policies", a.contentPolicies)
	protected.HandleFunc("POST /api/v1/customer-bots/{bot}/access-policies", a.contentPolicySave)
	protected.HandleFunc("POST /api/v1/customer-bots/{bot}/access-policies/{policy}", a.contentPolicySave)
	protected.HandleFunc("GET /api/v1/customer-bots/{bot}/managed-files", a.contentFiles)
	protected.HandleFunc("POST /api/v1/customer-bots/{bot}/managed-files", a.contentFileSave)
	protected.HandleFunc("POST /api/v1/customer-bots/{bot}/managed-files/{reference}", a.contentFileSave)
	protected.HandleFunc("GET /api/v1/customer-bots/{bot}/runtime", a.contentRuntime)
	protected.HandleFunc("POST /api/v1/customer-bots/{bot}/connect", a.contentConnect)
	protected.HandleFunc("POST /api/v1/customer-bots/{bot}/disconnect", a.contentDisconnect)
	protected.HandleFunc("GET /api/v1/customer-bots", a.factoryList)
	protected.HandleFunc("GET /api/v1/customer-bots/catalog", a.factoryCatalog)
	protected.HandleFunc("POST /api/v1/customer-bots/preview", a.factoryPreview)
	protected.HandleFunc("POST /api/v1/customer-bots", a.factoryCreate)
	protected.HandleFunc("GET /api/v1/customer-bots/{bot}/plugins", a.factoryCatalog)
	protected.HandleFunc("POST /api/v1/customer-bots/{bot}/preview", a.factoryPreview)
	protected.HandleFunc("POST /api/v1/customer-bots/{bot}/plugins/{plugin}/impact", a.factoryImpact)
	protected.HandleFunc("POST /api/v1/customer-bots/{bot}/plugins/{plugin}/actions", a.factoryChange)
	protected.HandleFunc("POST /api/v1/customer-bots/{bot}/plugins/{plugin}/trial", a.factoryTrial)
	protected.HandleFunc("GET /api/v1/commercial/catalog", a.commercialCatalog)
	protected.HandleFunc("GET /api/v1/commercial/catalog/{kind}/{key}/history", a.commercialHistory)
	protected.HandleFunc("POST /api/v1/commercial/catalog", a.commercialSave)
	protected.HandleFunc("GET /api/v1/commercial/agreements", a.commercialAgreements)
	protected.HandleFunc("POST /api/v1/commercial/agreements", a.commercialIssue)
	protected.HandleFunc("PATCH /api/v1/commercial/agreements/{agreement}", a.commercialState)
	protected.HandleFunc("POST /api/v1/commercial/decision", a.commercialDecision)
	protected.HandleFunc("POST /api/v1/commercial/trials", a.commercialTrial)
	protected.HandleFunc("POST /api/v1/commercial/quota", a.commercialQuota)
	protected.HandleFunc("GET /api/v1/groups", a.groupList)
	protected.HandleFunc("POST /api/v1/groups/claims", a.groupClaim)
	protected.HandleFunc("GET /api/v1/groups/{group}", a.groupDetail)
	protected.HandleFunc("PATCH /api/v1/groups/{group}/policy", a.groupConfigure)
	protected.HandleFunc("POST /api/v1/groups/{group}/actions", a.groupModerate)
	protected.HandleFunc("GET /api/v1/groups/{group}/history", a.groupHistory)
	protected.HandleFunc("GET /api/v1/auth/me", a.me)
	protected.HandleFunc("GET /api/v1/auth/permissions", a.adminPermissions)
	protected.HandleFunc("GET /api/v1/access-policies", a.policyList)
	protected.HandleFunc("POST /api/v1/access-policies", a.policySave)
	protected.HandleFunc("POST /api/v1/access-policies/preview", a.policyPreview)
	protected.HandleFunc("POST /api/v1/access/decision", a.policyRuntime)
	protected.HandleFunc("GET /api/v1/safety-limits", a.safetyLimits)
	protected.HandleFunc("GET /api/v1/operations", a.operations)
	protected.HandleFunc("GET /api/v1/storage/{owner}/{object}", a.storageOpen)
	protected.HandleFunc("POST /api/v1/auth/logout", a.logout)
	protected.HandleFunc("GET /api/v1/workspaces", a.workspaces)
	protected.HandleFunc("GET /api/v1/dashboard", a.dashboard)
	protected.HandleFunc("GET /api/v1/customer/overview", a.customerOverview)
	protected.HandleFunc("GET /api/v1/customer/access", a.customerAccess)
	protected.HandleFunc("GET /api/v1/customer/catalog", a.customerCatalog)
	protected.HandleFunc("GET /api/v1/customer/settings", a.customerSettings)
	protected.HandleFunc("PATCH /api/v1/customer/settings", a.customerSettingsUpdate)
	protected.HandleFunc("GET /api/v1/customer/wallet/history", a.customerHistory)
	protected.HandleFunc("GET /api/v1/entitlements", a.access)
	protected.HandleFunc("POST /api/v1/entitlements/preview", a.preview)
	protected.HandleFunc("GET /api/v1/plans", a.plans)
	protected.HandleFunc("POST /api/v1/plans", a.createPlan)
	protected.HandleFunc("PATCH /api/v1/plans/{plan}", a.editPlan)
	protected.HandleFunc("PUT /api/v1/plans/{plan}/entitlements/{feature}", a.editEntitlement)
	protected.HandleFunc("PUT /api/v1/plans/{plan}/plugins/{plugin}", a.editPluginEntitlement)
	protected.HandleFunc("GET /api/v1/features", a.features)
	protected.HandleFunc("PUT /api/v1/features/{feature}/flag", a.editFlag)
	protected.HandleFunc("GET /api/v1/plugins", a.plugins)
	protected.HandleFunc("PUT /api/v1/plugins/{plugin}/status", a.editPluginStatus)
	protected.HandleFunc("PUT /api/v1/entitlements/overrides/{feature}", a.override)
	protected.HandleFunc("DELETE /api/v1/entitlements/overrides/{feature}", a.removeOverride)
	protected.HandleFunc("PUT /api/v1/entitlements/grants/{feature}", a.grant)
	protected.HandleFunc("DELETE /api/v1/entitlements/grants/{feature}", a.removeGrant)
	protected.HandleFunc("PUT /api/v1/subscriptions", a.changePlan)
	protected.HandleFunc("GET /api/v1/wallets", a.wallet)
	protected.HandleFunc("POST /api/v1/wallets/adjustments", a.adjust)
	protected.HandleFunc("GET /api/v1/ledger", a.ledger)
	protected.HandleFunc("GET /api/v1/tickets", a.ticketList)
	protected.HandleFunc("POST /api/v1/tickets", a.ticketCreate)
	protected.HandleFunc("GET /api/v1/tickets/{ticket}", a.ticketDetail)
	protected.HandleFunc("POST /api/v1/tickets/{ticket}/replies", a.ticketReply)
	protected.HandleFunc("POST /api/v1/tickets/{ticket}/actions", a.ticketAction)
	protected.HandleFunc("POST /api/v1/downloader", a.downloaderSubmit)
	protected.HandleFunc("GET /api/v1/downloader", a.downloaderHistory)
	protected.HandleFunc("GET /api/v1/downloader/{job}", a.downloaderDetail)
	protected.HandleFunc("POST /api/v1/downloader/{job}/format", a.downloaderSelect)
	protected.HandleFunc("POST /api/v1/downloader/{job}/cancel", a.downloaderCancel)
	protected.HandleFunc("GET /api/v1/downloads", a.downloadList)
	protected.HandleFunc("GET /api/v1/downloads/formats", a.formats)
	protected.HandleFunc("POST /api/v1/downloads", a.downloadCreate)
	protected.HandleFunc("GET /api/v1/audit", a.audit)
	protected.HandleFunc("GET /api/v1/team", a.team)
	protected.HandleFunc("/api/v1/", func(w http.ResponseWriter, r *http.Request) { fail(w, 501, "not_implemented") })
	mux.Handle("/api/v1/", a.authenticated(protected))
	return a.secure(mux)
}
func respond(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
func fail(w http.ResponseWriter, status int, key string) {
	respond(w, status, map[string]any{"error": map[string]string{"code": key, "message": localization.Text(key)}})
}
func decode(w http.ResponseWriter, r *http.Request, out any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if e := d.Decode(out); e != nil {
		return e
	}
	var rest any
	if e := d.Decode(&rest); e != io.EOF {
		return errors.New("extra request data")
	}
	return nil
}
func (a *API) secure(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a.Requests.Add(1)
		id, err := auth.Token()
		if err != nil {
			fail(w, 500, "internal")
			return
		}
		id = id[:24]
		outcome := &responseOutcome{ResponseWriter: w, status: 200}
		w = outcome
		defer func() {
			slog.Info("request completed", "request_id", id, "correlation_id", id, "method", r.Method, "status", outcome.status)
		}()
		w.Header().Set("X-Request-ID", id)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors https://web.telegram.org https://telegram.org")
		if a.Config.Environment == "production" {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		if r.Method != "GET" && r.Method != "HEAD" {
			origin := r.Header.Get("Origin")
			if origin != "" {
				u, e := url.Parse(origin)
				configured, _ := url.Parse(a.Config.MiniAppURL)
				if e != nil || configured == nil || u.Host != configured.Host || u.Scheme != configured.Scheme || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
					fail(w, 403, "forbidden")
					return
				}
			} else if _, e := r.Cookie("telmivo_session"); e == nil {
				fail(w, 403, "forbidden")
				return
			}
		}
		if strings.HasPrefix(r.URL.Path, "/api/") && a.Cache != nil {
			host, _, _ := net.SplitHostPort(r.RemoteAddr)
			allowed, e := a.Cache.Allow(r.Context(), "ip:"+host, 120, time.Minute)
			if e != nil {
				fail(w, 503, "unavailable")
				return
			}
			if !allowed {
				fail(w, 429, "rate_limit")
				return
			}
		}
		defer func() {
			if recover() != nil {
				fail(w, 500, "internal")
			}
		}()
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestKey, id)))
	})
}
func (a *API) authenticated(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, e := r.Cookie("telmivo_session")
		if e != nil {
			fail(w, 401, "unauthorized")
			return
		}
		id, e := a.Identity.Session(r.Context(), c.Value)
		if e != nil {
			if errors.Is(e, auth.ErrInvalid) {
				fail(w, 401, "unauthorized")
			} else {
				handleError(w, e)
			}
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userKey, id)))
	})
}
func user(r *http.Request) string        { v, _ := r.Context().Value(userKey).(string); return v }
func correlation(r *http.Request) string { v, _ := r.Context().Value(requestKey).(string); return v }
func workspace(r *http.Request) string   { return r.Header.Get("X-Workspace-ID") }
func handleError(w http.ResponseWriter, e error) {
	var mediaFailure media.Failure
	if errors.As(e, &mediaFailure) {
		respond(w, 400, map[string]any{"error": map[string]string{"code": string(mediaFailure), "message": downloads.PersianError(string(mediaFailure))}})
		return
	}
	if errors.Is(e, database.ErrForbidden) {
		fail(w, 403, "forbidden")
		return
	}
	if errors.Is(e, storage.ErrDenied) {
		fail(w, 403, "forbidden")
		return
	}
	if errors.Is(e, storage.ErrUnavailable) {
		fail(w, 503, "unavailable")
		return
	}
	if errors.Is(e, contentplugins.ErrRuntimeNotRunning) {
		fail(w, 409, "BOT_RUNTIME_NOT_RUNNING")
		return
	}
	if errors.Is(e, channels.ErrAuthority) {
		fail(w, 503, "CHANNEL_AUTHORITY_UNAVAILABLE")
		return
	}
	if errors.Is(e, channels.ErrBotRights) {
		fail(w, 403, "CHANNEL_BOT_RIGHT_MISSING")
		return
	}
	if errors.Is(e, groupbot.ErrAuthority) {
		fail(w, 503, "GROUP_AUTHORITY_UNAVAILABLE")
		return
	}
	if errors.Is(e, groupbot.ErrRights) {
		fail(w, 403, "GROUP_BOT_RIGHT_MISSING")
		return
	}
	if errors.Is(e, groupbot.ErrProtected) {
		fail(w, 403, "GROUP_TARGET_PROTECTED")
		return
	}
	var quotaDenied *contentplugins.QuotaDenied
	if errors.As(e, &quotaDenied) {
		respond(w, 403, map[string]any{"error": map[string]string{"code": quotaDenied.Error(), "message": "سهمیهٔ عملیات کافی نیست"}, "quota": quotaDenied.Result})
		return
	}
	var factoryRejected *botfactory.Rejected
	if errors.As(e, &factoryRejected) {
		respond(w, 409, map[string]any{"error": map[string]string{"code": "BOT_SELECTION_REJECTED", "message": "انتخاب پلاگین‌ها نیاز به بررسی دارد"}, "review": factoryRejected.Review})
		return
	}
	var commercialDenied *commerce.Denied
	if errors.As(e, &commercialDenied) {
		respond(w, 403, map[string]any{"error": map[string]string{"code": "forbidden"}, "decision": commercialDenied.Decision})
		return
	}
	var policyDenied *policies.Denied
	if errors.As(e, &policyDenied) {
		respond(w, 403, map[string]any{"error": map[string]string{"code": "forbidden", "message": localization.Text("forbidden")}, "decision": policyDenied.Decision})
		return
	}
	var denied *entitlements.Denied
	if errors.As(e, &denied) {
		fail(w, 403, "forbidden")
		return
	}
	for _, failure := range []error{sdk.ErrManifest, sdk.ErrDependency, sdk.ErrCycle, sdk.ErrConflict, sdk.ErrVersion, sdk.ErrState, sdk.ErrConfiguration, sdk.ErrPermission} {
		if errors.Is(e, failure) {
			status := 409
			if failure == sdk.ErrPermission {
				status = 403
			}
			respond(w, status, map[string]any{"error": map[string]string{"code": failure.Error(), "message": "عملیات پلاگین مجاز نیست؛ وضعیت و وابستگی‌ها را بررسی کنید"}})
			return
		}
	}
	if errors.Is(e, database.ErrConflict) {
		fail(w, 409, "conflict")
		return
	}
	if errors.Is(e, pgx.ErrNoRows) {
		fail(w, 404, "invalid")
		return
	}
	if errors.Is(e, database.ErrInvalid) {
		fail(w, 400, "invalid")
		return
	}
	var pe *pgconn.PgError
	if errors.As(e, &pe) {
		switch pe.Code {
		case "23505":
			fail(w, 409, "conflict")
			return
		case "23503", "23514", "22P02":
			fail(w, 400, "invalid")
			return
		case "42501":
			fail(w, 403, "forbidden")
			return
		}
	}
	slog.Error("request failed", "error_code", "INTERNAL_FAILURE")
	fail(w, 500, "internal")
}
func (a *API) tenant(w http.ResponseWriter, r *http.Request, admin bool, fn func(pgx.Tx) (any, error)) {
	if admin && !a.DB.HasPlatformPermission(r.Context(), user(r), platformAction(r)) {
		fail(w, 403, "forbidden")
		return
	}
	var out any
	e := a.DB.Tenant(r.Context(), user(r), workspace(r), admin, func(tx pgx.Tx) error { var e error; out, e = fn(tx); return e })
	if e != nil {
		handleError(w, e)
		return
	}
	respond(w, 200, out)
}
func rows(ctx context.Context, tx pgx.Tx, query string, args ...any) ([]map[string]any, error) {
	rs, e := tx.Query(ctx, query, args...)
	if e != nil {
		return nil, e
	}
	defer rs.Close()
	out := []map[string]any{}
	for rs.Next() {
		vs, e := rs.Values()
		if e != nil {
			return nil, e
		}
		m := map[string]any{}
		for i, f := range rs.FieldDescriptions() {
			m[f.Name] = vs[i]
		}
		out = append(out, m)
	}
	return out, rs.Err()
}
func (a *API) ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if a.DB == nil || a.DB.Pool == nil || a.Cache == nil || a.Cache.Redis == nil || a.Queue == nil || a.Queue.Conn == nil || a.DB.Pool.Ping(ctx) != nil || a.Cache.Redis.Ping(ctx).Err() != nil || !a.Queue.Conn.IsConnected() {
		fail(w, 503, "unavailable")
		return
	}
	respond(w, 200, map[string]string{"status": "ready"})
}
func (a *API) login(w http.ResponseWriter, r *http.Request) {
	var in struct {
		InitData string `json:"init_data"`
	}
	if decode(w, r, &in) != nil {
		fail(w, 400, "invalid")
		return
	}
	var u auth.TelegramUser
	e := auth.ErrInvalid
	for _, token := range []string{a.Config.BotToken, a.Config.GroupBotToken, a.Config.DownloaderBotToken} {
		if token == "" {
			continue
		}
		u, e = auth.ValidateInitData(in.InitData, token, time.Now(), 10*time.Minute)
		if e == nil {
			break
		}
	}
	if e != nil {
		fail(w, 401, "unauthorized")
		return
	}
	out, token, e := a.Identity.Login(r.Context(), u, in.InitData)
	if e != nil {
		if errors.Is(e, auth.ErrInvalid) {
			fail(w, 401, "unauthorized")
		} else {
			handleError(w, e)
		}
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "telmivo_session", Value: token, Path: "/api/v1", HttpOnly: true, Secure: a.Config.Environment == "production", SameSite: http.SameSiteStrictMode, MaxAge: int(a.Config.SessionTTL.Seconds())})
	respond(w, 200, out)
}
func (a *API) logout(w http.ResponseWriter, r *http.Request) {
	cookie, e := r.Cookie("telmivo_session")
	if e != nil {
		fail(w, 401, "unauthorized")
		return
	}
	_, e = a.DB.Pool.Exec(r.Context(), "DELETE FROM sessions WHERE token_hash=$1", auth.Digest(cookie.Value))
	if e != nil {
		fail(w, 503, "unavailable")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "telmivo_session", Path: "/api/v1", MaxAge: -1, HttpOnly: true, Secure: a.Config.Environment == "production", SameSite: http.SameSiteStrictMode})
	respond(w, 200, map[string]bool{"logged_out": true})
}
func (a *API) me(w http.ResponseWriter, r *http.Request) {
	var name string
	e := a.DB.Pool.QueryRow(r.Context(), "SELECT display_name FROM users WHERE id=$1", user(r)).Scan(&name)
	if e != nil {
		handleError(w, e)
		return
	}
	respond(w, 200, map[string]any{"user_id": user(r), "name": name, "is_admin": a.DB.IsAdmin(r.Context(), user(r)), "synthetic_mode": a.Config.Synthetic, "can_admin_policies": a.DB.HasPlatformPermission(r.Context(), user(r), "platform.access_policies.read"), "can_admin_commercial": a.DB.HasPlatformPermission(r.Context(), user(r), "platform.commercial.read"), "can_adjust_wallet": a.DB.HasPlatformPermission(r.Context(), user(r), "platform.wallets.adjust")})
}
func (a *API) workspaces(w http.ResponseWriter, r *http.Request) {
	tx, e := a.DB.Pool.Begin(r.Context())
	if e != nil {
		handleError(w, e)
		return
	}
	defer tx.Rollback(r.Context())
	out, e := rows(r.Context(), tx, "SELECT w.id::text,w.name,m.role_key FROM workspaces w JOIN workspace_members m ON m.workspace_id=w.id WHERE m.user_id=$1 ORDER BY w.created_at,w.id LIMIT 100", user(r))
	if e != nil {
		handleError(w, e)
		return
	}
	respond(w, 200, out)
}
func (a *API) dashboard(w http.ResponseWriter, r *http.Request) {
	a.tenant(w, r, false, func(tx pgx.Tx) (any, error) {
		plan, e := entitlements.EffectivePlan(r.Context(), tx, workspace(r))
		if e != nil {
			return nil, e
		}
		counts, e := rows(r.Context(), tx, "SELECT (SELECT count(*) FROM channels WHERE workspace_id=$1) AS channels,(SELECT count(*) FROM groups WHERE workspace_id=$1) AS groups,(SELECT count(*) FROM customer_bots WHERE workspace_id=$1) AS bots,(SELECT count(*) FROM tickets WHERE workspace_id=$1 AND status NOT IN('CLOSED','RESOLVED')) AS open_tickets,(SELECT count(*) FROM tickets WHERE workspace_id=$1 AND priority='URGENT' AND status NOT IN('CLOSED','RESOLVED')) AS urgent_tickets,(SELECT count(*) FROM workspace_members WHERE workspace_id=$1) AS members", workspace(r))
		return map[string]any{"plan": plan, "counts": counts}, e
	})
}
func (a *API) access(w http.ResponseWriter, r *http.Request) {
	a.tenant(w, r, false, func(tx pgx.Tx) (any, error) {
		role, e := entitlements.Role(r.Context(), tx, user(r), workspace(r))
		if e != nil {
			return nil, e
		}
		return accessMap(r.Context(), tx, workspace(r), role, user(r))
	})
}
func accessMap(ctx context.Context, tx pgx.Tx, workspace, role string, subject ...string) (any, error) {
	fs, e := rows(ctx, tx, "SELECT key,permission_key FROM features ORDER BY key")
	if e != nil {
		return nil, e
	}
	out := map[string]entitlement.Decision{}
	for _, f := range fs {
		k := f["key"].(string)
		var d entitlement.Decision
		if len(subject) > 0 {
			d, e = entitlements.ResolveForUser(ctx, tx, subject[0], workspace, role, k, "", f["permission_key"].(string), 0)
		} else {
			d, e = entitlements.Resolve(ctx, tx, workspace, role, k, "", f["permission_key"].(string), 0)
		}
		if e != nil {
			return nil, e
		}
		out[k] = d
	}
	return out, nil
}
func (a *API) preview(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Role string `json:"role"`
	}
	if decode(w, r, &in) != nil {
		fail(w, 400, "invalid")
		return
	}
	a.tenant(w, r, true, func(tx pgx.Tx) (any, error) { return accessMap(r.Context(), tx, workspace(r), in.Role) })
}
func (a *API) plans(w http.ResponseWriter, r *http.Request) {
	a.tenant(w, r, false, func(tx pgx.Tx) (any, error) {
		ps, e := rows(r.Context(), tx, "SELECT key,name,active,version FROM plans ORDER BY key")
		if e != nil {
			return nil, e
		}
		es, e := rows(r.Context(), tx, "SELECT plan_key,feature_key,enabled,limit_value::text FROM plan_entitlements ORDER BY plan_key,feature_key")
		if e != nil {
			return nil, e
		}
		plugins, e := rows(r.Context(), tx, "SELECT plan_key,plugin_key,enabled FROM plan_plugin_entitlements ORDER BY plan_key,plugin_key")
		return map[string]any{"plans": ps, "entitlements": es, "plugins": plugins}, e
	})
}
func (a *API) features(w http.ResponseWriter, r *http.Request) {
	a.tenant(w, r, false, func(tx pgx.Tx) (any, error) {
		return rows(r.Context(), tx, "SELECT f.key,f.name_key,ff.enabled,ff.rollout_status FROM features f JOIN feature_flags ff ON ff.feature_key=f.key ORDER BY f.key")
	})
}
func (a *API) plugins(w http.ResponseWriter, r *http.Request) {
	a.tenant(w, r, false, func(tx pgx.Tx) (any, error) {
		catalog, e := rows(r.Context(), tx, "SELECT key,name_key,version,status,required_feature,configuration_schema FROM plugins ORDER BY key")
		if e != nil {
			return nil, e
		}
		capabilities, e := pluginGovernanceRows(r, tx)
		return map[string]any{"catalog": catalog, "modules": botmodules.Defaults().List(), "capabilities": capabilities}, e
	})
}
func (a *API) changePlan(w http.ResponseWriter, r *http.Request) {
	var in subscriptions.Change
	if decode(w, r, &in) != nil {
		fail(w, 400, "invalid")
		return
	}
	a.tenant(w, r, true, func(tx pgx.Tx) (any, error) {
		e := subscriptions.ChangePlan(r.Context(), tx, user(r), workspace(r), correlation(r), in)
		return map[string]bool{"saved": e == nil}, e
	})
}
func (a *API) wallet(w http.ResponseWriter, r *http.Request) {
	a.tenant(w, r, false, func(tx pgx.Tx) (any, error) {
		if !database.Permission(r.Context(), tx, user(r), workspace(r), "wallet.read") {
			return nil, database.ErrForbidden
		}
		return rows(r.Context(), tx, "SELECT asset,balance_atomic FROM wallet_balances WHERE workspace_id=$1 ORDER BY asset", workspace(r))
	})
}
func (a *API) adjust(w http.ResponseWriter, r *http.Request) {
	if !a.Config.Synthetic {
		fail(w, 501, "not_implemented")
		return
	}
	var in wallets.Adjustment
	if decode(w, r, &in) != nil {
		fail(w, 400, "invalid")
		return
	}
	a.tenant(w, r, true, func(tx pgx.Tx) (any, error) {
		id, e := wallets.Adjust(r.Context(), tx, user(r), workspace(r), correlation(r), in)
		return map[string]string{"ledger_transaction_id": id}, e
	})
}
func (a *API) ledger(w http.ResponseWriter, r *http.Request) {
	a.tenant(w, r, false, func(tx pgx.Tx) (any, error) {
		if !database.Permission(r.Context(), tx, user(r), workspace(r), "wallet.read") {
			return nil, database.ErrForbidden
		}
		return rows(r.Context(), tx, "SELECT t.id::text,t.asset,t.transaction_type,t.reason,t.created_at,e.amount_atomic::text FROM ledger_transactions t JOIN ledger_entries e ON e.transaction_id=t.id JOIN wallet_accounts a ON a.id=e.account_id WHERE t.workspace_id=$1 AND a.kind='WALLET' ORDER BY t.created_at DESC LIMIT 100", workspace(r))
	})
}
func (a *API) ticketList(w http.ResponseWriter, r *http.Request) {
	a.tenant(w, r, false, func(tx pgx.Tx) (any, error) {
		if e := entitlements.Require(r.Context(), tx, user(r), workspace(r), "tickets", "tickets.read", 0); e != nil {
			return nil, e
		}
		return rows(r.Context(), tx, "SELECT id::text,subject,status,priority,creator_id::text,created_at FROM tickets WHERE workspace_id=$1 ORDER BY updated_at DESC LIMIT 100", workspace(r))
	})
}
func (a *API) ticketCreate(w http.ResponseWriter, r *http.Request) {
	var in tickets.CreateInput
	if decode(w, r, &in) != nil {
		fail(w, 400, "invalid")
		return
	}
	a.tenant(w, r, false, func(tx pgx.Tx) (any, error) {
		id, e := tickets.Create(r.Context(), tx, user(r), workspace(r), correlation(r), in)
		return map[string]string{"id": id}, e
	})
}
func (a *API) ticketDetail(w http.ResponseWriter, r *http.Request) {
	a.tenant(w, r, false, func(tx pgx.Tx) (any, error) {
		if e := entitlements.Require(r.Context(), tx, user(r), workspace(r), "tickets", "tickets.read", 0); e != nil {
			return nil, e
		}
		ts, e := rows(r.Context(), tx, "SELECT id::text,subject,status,priority,creator_id::text FROM tickets WHERE workspace_id=$1 AND id=$2", workspace(r), r.PathValue("ticket"))
		if e != nil {
			return nil, e
		}
		if len(ts) == 0 {
			return nil, pgx.ErrNoRows
		}
		ms, e := rows(r.Context(), tx, "SELECT id::text,body,actor_id::text,created_at FROM ticket_messages WHERE workspace_id=$1 AND ticket_id=$2 ORDER BY created_at", workspace(r), r.PathValue("ticket"))
		return map[string]any{"ticket": ts[0], "messages": ms}, e
	})
}
func (a *API) ticketReply(w http.ResponseWriter, r *http.Request) {
	var in tickets.ReplyInput
	if decode(w, r, &in) != nil {
		fail(w, 400, "invalid")
		return
	}
	a.tenant(w, r, false, func(tx pgx.Tx) (any, error) {
		e := tickets.Reply(r.Context(), tx, user(r), workspace(r), r.PathValue("ticket"), correlation(r), in)
		return map[string]bool{"saved": e == nil}, e
	})
}
func (a *API) ticketAction(w http.ResponseWriter, r *http.Request) {
	var in tickets.ActionInput
	if decode(w, r, &in) != nil {
		fail(w, 400, "invalid")
		return
	}
	a.tenant(w, r, false, func(tx pgx.Tx) (any, error) {
		e := tickets.Act(r.Context(), tx, user(r), workspace(r), r.PathValue("ticket"), correlation(r), in)
		return map[string]bool{"saved": e == nil}, e
	})
}
func (a *API) formats(w http.ResponseWriter, r *http.Request) {
	if !a.Config.Synthetic {
		fail(w, 501, "not_implemented")
		return
	}
	if _, e := downloads.Provider(r.URL.Query().Get("url")); e != nil {
		fail(w, 400, "invalid")
		return
	}
	a.tenant(w, r, false, func(tx pgx.Tx) (any, error) {
		if e := entitlements.Require(r.Context(), tx, user(r), workspace(r), "downloads", "downloads.create", 0); e != nil {
			return nil, e
		}
		return map[string]any{"synthetic": true, "formats": downloads.FixtureFormats()}, nil
	})
}
func (a *API) downloadList(w http.ResponseWriter, r *http.Request) {
	a.tenant(w, r, false, func(tx pgx.Tx) (any, error) {
		if !database.Permission(r.Context(), tx, user(r), workspace(r), "downloads.read") {
			return nil, database.ErrForbidden
		}
		return rows(r.Context(), tx, "SELECT id::text,provider,state,synthetic,created_at FROM download_jobs WHERE workspace_id=$1 ORDER BY created_at DESC LIMIT 100", workspace(r))
	})
}
func (a *API) downloadCreate(w http.ResponseWriter, r *http.Request) {
	if !a.Config.Synthetic {
		fail(w, 501, "not_implemented")
		return
	}
	var in downloads.CreateInput
	if decode(w, r, &in) != nil {
		fail(w, 400, "invalid")
		return
	}
	a.tenant(w, r, false, func(tx pgx.Tx) (any, error) {
		id, e := downloads.CreateSynthetic(r.Context(), tx, user(r), workspace(r), correlation(r), in)
		return map[string]any{"id": id, "synthetic": true}, e
	})
}
func (a *API) audit(w http.ResponseWriter, r *http.Request) {
	a.tenant(w, r, false, func(tx pgx.Tx) (any, error) {
		if !database.Permission(r.Context(), tx, user(r), workspace(r), "audit.read") {
			return nil, database.ErrForbidden
		}
		return rows(r.Context(), tx, "SELECT id::text,action,entity,entity_id,actor_id::text,correlation_id,created_at FROM audit_logs WHERE workspace_id=$1 ORDER BY created_at DESC LIMIT 100", workspace(r))
	})
}
func (a *API) team(w http.ResponseWriter, r *http.Request) {
	a.tenant(w, r, false, func(tx pgx.Tx) (any, error) {
		if !database.Permission(r.Context(), tx, user(r), workspace(r), "team.read") {
			return nil, database.ErrForbidden
		}
		return rows(r.Context(), tx, "SELECT m.user_id::text,u.display_name,m.role_key FROM workspace_members m JOIN users u ON u.id=m.user_id WHERE m.workspace_id=$1 ORDER BY m.created_at", workspace(r))
	})
}
func parseLimit(s *string) (*int64, error) {
	if s == nil {
		return nil, nil
	}
	n, e := strconv.ParseInt(*s, 10, 64)
	if e != nil || n < 0 {
		return nil, errors.New("invalid limit")
	}
	return &n, nil
}

func platformAction(r *http.Request) string {
	p := r.URL.Path
	switch {
	case strings.HasPrefix(p, "/api/v1/wallets/adjustments"):
		return "platform.wallets.adjust"
	case strings.HasPrefix(p, "/api/v1/plugins"), strings.Contains(p, "/plugins/"):
		return "platform.plugins.manage"
	case strings.HasPrefix(p, "/api/v1/features"):
		return "platform.features.manage"
	case strings.HasSuffix(p, "/preview"):
		return "platform.plans.read"
	default:
		return "platform.plans.manage"
	}
}

type responseOutcome struct {
	http.ResponseWriter
	status int
}

func (w *responseOutcome) WriteHeader(code int) { w.status = code; w.ResponseWriter.WriteHeader(code) }
