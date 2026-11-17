package main

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/analytics/usecases/ingestevents"
	"github.com/diegobermudez03/playhoot/budget"
	"github.com/diegobermudez03/playhoot/budget/usecases/admitwork"
	"github.com/diegobermudez03/playhoot/budget/usecases/manualladder"
	"github.com/diegobermudez03/playhoot/budget/usecases/recordspend"
	"github.com/diegobermudez03/playhoot/budget/usecases/setcontrols"
	"github.com/diegobermudez03/playhoot/budget/usecases/settlework"
	budgetstatus "github.com/diegobermudez03/playhoot/budget/usecases/status"
	"github.com/diegobermudez03/playhoot/budget/workflows/evaluateladder"
	"github.com/diegobermudez03/playhoot/compliance"
	"github.com/diegobermudez03/playhoot/compliance/usecases/acceptances"
	"github.com/diegobermudez03/playhoot/compliance/usecases/marketaccess"
	"github.com/diegobermudez03/playhoot/game"
	"github.com/diegobermudez03/playhoot/game/workflows/gamelifecycle"
	"github.com/diegobermudez03/playhoot/identity"
	"github.com/diegobermudez03/playhoot/identity/loginproviders"
	"github.com/diegobermudez03/playhoot/identity/usecases/grantadmin"
	"github.com/diegobermudez03/playhoot/identity/usecases/readcontact"
	"github.com/diegobermudez03/playhoot/identity/usecases/secondfactor"
	"github.com/diegobermudez03/playhoot/identity/workflows/accountlogin"
	"github.com/diegobermudez03/playhoot/notifications"
	"github.com/diegobermudez03/playhoot/notifications/emailproviders"
	"github.com/diegobermudez03/playhoot/notifications/usecases/acceptentry"
	"github.com/diegobermudez03/playhoot/notifications/usecases/listentries"
	"github.com/diegobermudez03/playhoot/notifications/usecases/markread"
	"github.com/diegobermudez03/playhoot/notifications/usecases/recordsuppression"
	"github.com/diegobermudez03/playhoot/notifications/usecases/sendemail"
	"github.com/diegobermudez03/playhoot/orchestrator/analyticsingestion"
	"github.com/diegobermudez03/playhoot/orchestrator/costcontrol"
	orchestratordeadtasks "github.com/diegobermudez03/playhoot/orchestrator/deadtasks"
	"github.com/diegobermudez03/playhoot/orchestrator/notificationdelivery"
	"github.com/diegobermudez03/playhoot/orchestrator/spendingestion"
	"github.com/diegobermudez03/playhoot/orchestrator/stepup"
	"github.com/diegobermudez03/playhoot/play/session"
	"github.com/diegobermudez03/playhoot/play/session/storage"
	"github.com/diegobermudez03/playhoot/play/session/workflows/sessionlifecycle"
	"github.com/diegobermudez03/playhoot/utils"
	"github.com/diegobermudez03/playhoot/utils/txqueue"
)

// walkthroughDB is the whole application's schema and local seed, as main
// migrates it, in a database dropped when the run ends.
var walkthroughDB = utils.DisposableTestDB{
	Key:        "walkthrough",
	NameSuffix: "walkthrough_test",
	Migrate: func(db *gorm.DB) error {
		if err := PostgresMigrate(db); err != nil {
			return err
		}
		return PostgresSeed(db)
	},
}

func TestMain(m *testing.M) { os.Exit(walkthroughDB.Run(m)) }

// walkthroughQueues retries quickly so backoff, dead tasks and paused work
// can be watched within a test run; the policy is otherwise the default one.
var walkthroughQueues = txqueue.WorkerConfig{
	PollInterval: 10 * time.Millisecond, Batch: 50, Lease: time.Minute,
	MaxAttempts: 3, BaseBackoff: 200 * time.Millisecond, MaxBackoff: 800 * time.Millisecond,
}

// Seeded tap race game: playable, so a Session can be created and started.
const walkthroughTapRaceGameUUID = "55555555-5555-4555-8555-555555555555"

var (
	walkthroughAccessKey   = []byte("walkthrough-access-key-32-bytes!")
	walkthroughTotpKey     = []byte("walkthrough-totp-sealing-key-32b")
	walkthroughUnsubscribe = []byte("walkthrough-unsubscribe-key-32by")
	walkthroughWebhook     = []byte("walkthrough-webhook-secret")
)

// h1System is the H1 part of main's wiring over one database. Building it
// again over the same database is a process restart.
type h1System struct {
	db                                               *gorm.DB
	queues                                           *txqueue.Registry
	identityQ, budgetQ, notificationsQ, gameQ, sessQ *txqueue.Queue

	admit        *admitwork.Service
	settle       *settlework.Service
	controls     *setcontrols.Service
	evaluate     *evaluateladder.Workflow
	status       *budgetstatus.Service
	costControls *costcontrol.Manager
	deadTasks    *orchestratordeadtasks.Manager

	logins       *accountlogin.Manager
	secondFactor *secondfactor.Service

	delivery     *notificationdelivery.Delivery
	sender       *sendemail.Service
	entries      *listentries.Service
	reads        *markread.Service
	suppressions *recordsuppression.Service

	declarations *marketaccess.Service
	games        *gamelifecycle.Manager
	sessions     *sessionlifecycle.Manager
}

func wireH1(t *testing.T, db *gorm.DB, resend, ses *emailproviders.Simulated) *h1System {
	t.Helper()
	s := &h1System{db: db, queues: txqueue.NewRegistry(db, walkthroughQueues)}
	s.identityQ = s.queues.Queue(identity.QueueDomain)
	s.budgetQ = s.queues.Queue(budget.QueueDomain)
	s.notificationsQ = s.queues.Queue(notifications.QueueDomain)
	s.gameQ = s.queues.Queue(game.QueueDomain)
	s.sessQ = s.queues.Queue(session.QueueDomain)

	var err error
	s.secondFactor, err = secondfactor.New(db, walkthroughTotpKey, walkthroughAccessKey)
	require.NoError(t, err)
	registry := loginproviders.New(loginproviders.Config{Local: &loginproviders.LocalConfig{
		AuthorizeURL: "/v1/identity/login/local/authorize", CallbackURL: "/v1/identity/login/local/callback",
	}})
	s.logins, err = accountlogin.New(db, registry, walkthroughAccessKey)
	require.NoError(t, err)

	s.admit, s.settle, s.controls, s.status = admitwork.New(db), settlework.New(db), setcontrols.New(db), budgetstatus.New(db)
	s.evaluate = evaluateladder.New(db, s.budgetQ)
	s.evaluate.Register(s.budgetQ)
	s.settle.Register(s.budgetQ)
	spendingestion.New(recordspend.New(db, s.budgetQ)).Register(s.identityQ, s.budgetQ, s.notificationsQ)

	contacts := readcontact.New(db)
	s.delivery = notificationdelivery.New(contacts, acceptances.New(db), acceptentry.New(db, s.notificationsQ))
	s.delivery.Register(s.identityQ, s.budgetQ, s.notificationsQ)
	s.sender, err = sendemail.New(db, s.notificationsQ, walkthroughUnsubscribe, sendemail.Links{
		OpenURL: "http://localhost:5173/notifications", UnsubscribeURL: "http://localhost:8080/v1/email/unsubscribe/",
	}, resend, ses)
	require.NoError(t, err)
	s.suppressions, err = recordsuppression.New(db, walkthroughUnsubscribe, resend, ses)
	require.NoError(t, err)
	notificationdelivery.NewEmail(contacts, s.admit, s.sender, s.settle).Register(s.notificationsQ)
	s.entries, s.reads = listentries.New(db), markread.New(db)
	s.costControls = costcontrol.New(stepup.New(s.secondFactor), s.controls, manualladder.New(db, s.budgetQ))
	s.deadTasks = orchestratordeadtasks.New(s.queues)

	s.declarations = marketaccess.New(db)
	analyticsingestion.New(ingestevents.New(db)).Register(s.gameQ, s.sessQ)
	s.games = gamelifecycle.New(db, s.gameQ)
	store, _, err := storage.Open(context.Background(), "", "")
	require.NoError(t, err)
	s.sessions = sessionlifecycle.New(db, store, s.sessQ, 0)
	return s
}

// drain runs every queue until nothing is due; tasks waiting out a backoff
// stay for a later drain.
func (s *h1System) drain(t *testing.T) {
	t.Helper()
	for range 100 {
		total := 0
		for _, q := range []*txqueue.Queue{s.identityQ, s.budgetQ, s.notificationsQ, s.gameQ, s.sessQ} {
			n, err := q.ProcessDue(context.Background())
			require.NoError(t, err)
			total += n
		}
		if total == 0 {
			return
		}
	}
	t.Fatal("queues never settled")
}

// drainFor keeps draining for d, so retries whose backoff ends meanwhile run.
func (s *h1System) drainFor(t *testing.T, d time.Duration) {
	t.Helper()
	for end := time.Now().Add(d); time.Now().Before(end); time.Sleep(20 * time.Millisecond) {
		s.drain(t)
	}
	s.drain(t)
}

// signIn signs name in with the local provider and returns the user UUID.
func (s *h1System) signIn(t *testing.T, name string) string {
	t.Helper()
	ctx := context.Background()
	verifierBytes := make([]byte, 32)
	_, err := rand.Read(verifierBytes)
	require.NoError(t, err)
	verifier := base64.RawURLEncoding.EncodeToString(verifierBytes)
	sum := sha256.Sum256([]byte(verifier))
	started, err := s.logins.StartLogin(ctx, string(identity.ProviderLocal), base64.RawURLEncoding.EncodeToString(sum[:]), "")
	require.NoError(t, err)
	require.Equal(t, identity.LoginStarted, started.Outcome)
	authorize, err := url.Parse(started.AuthorizationURL)
	require.NoError(t, err)
	callback, err := s.logins.HandleCallback(ctx, string(identity.ProviderLocal), authorize.Query().Get("state"), name, "")
	require.NoError(t, err)
	require.Equal(t, identity.LoginCodeIssued, callback.Outcome)
	completed, err := s.logins.CompleteLogin(ctx, callback.LoginCode, verifier, "")
	require.NoError(t, err)
	require.Equal(t, identity.LoginCompleted, completed.Outcome)
	return completed.Credentials.User.UUID
}

// notify leaves a notify task in Identity's queue, as a domain would.
func (s *h1System) notify(t *testing.T, recipient string, typ notifications.Type, key string) {
	t.Helper()
	payload := notificationdelivery.Payload{Recipient: recipient, Type: string(typ), IdempotencyKey: key, ExternalToken: key,
		Skeleton: map[string]any{"title": "Walkthrough"}}
	require.NoError(t, s.identityQ.Enqueue(s.db, txqueue.TypeNotify, payload, time.Time{}, uuid.NewString()))
}

// spend reports a measured cost through Identity's queue, as a domain would.
func (s *h1System) spend(t *testing.T, category budget.Category, service, unit string, quantity float64) {
	t.Helper()
	key := uuid.NewString()
	payload := spendingestion.Payload{Category: string(category), Service: service, Reference: key, Quantity: quantity, Unit: unit,
		Measured: true, IdempotencyKey: key, OccurredAt: time.Now().UTC()}
	require.NoError(t, s.identityQ.Enqueue(s.db, txqueue.TypeSpendEvent, payload, time.Time{}, key))
}

// try admits one unit of work and releases it at once when admitted.
func (s *h1System) try(t *testing.T, category budget.Category, service string, discretionary bool) budget.Admission {
	t.Helper()
	admission, err := s.admit.AdmitWork(context.Background(), budget.WorkRequest{
		Category: category, Service: service, Quantity: 1, Reference: uuid.NewString(), Discretionary: discretionary, MaxDuration: time.Minute,
	})
	require.NoError(t, err)
	if admission.Outcome == budget.Admitted {
		_, err := s.settle.SettleWork(context.Background(), admission.ReservationUUID, budget.Settlement{})
		require.NoError(t, err)
	} else {
		require.Equal(t, budget.RestrictionMessage, admission.Message)
	}
	return admission
}

func (s *h1System) ladder(t *testing.T) budget.LadderState {
	t.Helper()
	status, err := s.status.Status(context.Background())
	require.NoError(t, err)
	return status.Ladder
}

// entry finds recipient's entry from source with type typ, newest first.
func (s *h1System) entry(t *testing.T, recipient string, typ notifications.Type) (notifications.Entry, int) {
	t.Helper()
	page, err := s.entries.ListEntries(context.Background(), recipient, notifications.EntryFilter{Type: typ})
	require.NoError(t, err)
	require.NotEmpty(t, page.Entries)
	return page.Entries[0], len(page.Entries)
}

type walkDelivery struct {
	Provider string
	State    string
	Detail   string
}

func (s *h1System) deliveries(t *testing.T, entryUUID string) []walkDelivery {
	t.Helper()
	var rows []walkDelivery
	require.NoError(t, s.db.Raw(`SELECT d.provider, d.state, d.detail FROM notifications_email_deliveries d
		JOIN notifications_entries e ON e.id = d.entry_id WHERE e.entry_uuid = ? ORDER BY d.id`, entryUUID).Scan(&rows).Error)
	return rows
}

// totpCodes produces an administrator's authenticator codes, never reusing a
// step: each code is one step after the last, waiting for the clock if the
// accepted window (one step either side) is used up.
type totpCodes struct {
	secret string
	last   int64
}

func (c *totpCodes) next(t *testing.T) string {
	t.Helper()
	for {
		current := time.Now().Unix() / 30
		step := max(c.last+1, current-1)
		if step <= current+1 {
			c.last = step
			key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(c.secret)
			require.NoError(t, err)
			var counter [8]byte
			binary.BigEndian.PutUint64(counter[:], uint64(step))
			mac := hmac.New(sha1.New, key)
			mac.Write(counter[:])
			sum := mac.Sum(nil)
			offset := sum[len(sum)-1] & 0x0f
			return fmt.Sprintf("%06d", (binary.BigEndian.Uint32(sum[offset:offset+4])&0x7fffffff)%1000000)
		}
		time.Sleep(time.Second)
	}
}

func (c *totpCodes) proof(t *testing.T, s *h1System, admin string) string {
	t.Helper()
	verified, err := s.secondFactor.VerifyTotp(context.Background(), admin, c.next(t), identity.PurposeStepUp)
	require.NoError(t, err)
	require.Equal(t, identity.SecondFactorAccepted, verified.Outcome)
	return verified.StepUpProof
}

// Services and units the walkthrough prices in the budget.
const (
	walkCompute     = "walkthrough-cpu"
	walkComputeUnit = "minutes"
	walkExport      = "walkthrough-export"
	walkExportUnit  = "exports"
)

var walkSES = notifications.SpendService(notifications.ProviderSES)

// TestH1Walkthrough demonstrates the H1 foundations end to end on the local
// database, in order: each step builds on the state the previous left. Every
// step name starts with the rule it shows.
func TestH1Walkthrough(t *testing.T) {
	ctx := context.Background()
	db := walkthroughDB.Open(t)
	// Spend is counted per UTC hour: start clear of the next hour boundary.
	if left := time.Until(time.Now().Truncate(time.Hour).Add(time.Hour)); left < 2*time.Minute {
		time.Sleep(left + time.Second)
	}

	resend := emailproviders.NewResend(walkthroughWebhook)
	ses := emailproviders.NewSES(walkthroughWebhook)
	s := wireH1(t, db, resend, ses)

	ana, admin, carla := s.signIn(t, "Ana"), s.signIn(t, "Beto"), s.signIn(t, "Carla")
	granted, err := grantadmin.New(db).EnsureAdmin(ctx, admin)
	require.NoError(t, err)
	require.NotEmpty(t, granted)
	enrollment, err := s.secondFactor.EnrollTotp(ctx, admin, "")
	require.NoError(t, err)
	require.Equal(t, identity.SecondFactorAccepted, enrollment.Outcome)
	codes := &totpCodes{secret: enrollment.Secret}
	confirmed, err := s.secondFactor.ConfirmTotp(ctx, admin, codes.next(t))
	require.NoError(t, err)
	require.Equal(t, identity.SecondFactorAccepted, confirmed.Outcome)

	// Controls the walkthrough runs under (the seeded email cap and rates stay).
	recorded, err := s.controls.AdminSetControls(ctx, admin, budget.Controls{Reason: "walkthrough caps", Caps: []budget.Cap{
		{Scope: budget.ScopeGlobal, Window: budget.WindowHour, USDLimit: 10},
		{Scope: budget.ScopeGlobal, Window: budget.WindowDay, USDLimit: 100},
		{Scope: budget.ScopeGlobal, Window: budget.WindowMonth, USDLimit: 100000},
		{Scope: string(budget.CategoryCompute), Window: budget.WindowHour, USDLimit: 9.5},
		{Scope: string(budget.CategoryExports), Window: budget.WindowHour, USDLimit: 1},
	}, Rates: []budget.Rate{
		{Category: budget.CategoryCompute, Service: walkCompute, Unit: walkComputeUnit, USDPerUnit: 0.1},
		{Category: budget.CategoryExports, Service: walkExport, Unit: walkExportUnit, USDPerUnit: 0.1},
	}})
	require.NoError(t, err)
	require.Equal(t, budget.ControlsRecorded, recorded)
	s.drain(t)
	require.Equal(t, budget.LevelNormal, s.ladder(t).Level)

	steps := []struct {
		name string
		run  func(t *testing.T)
	}{
		{"B principles·V-02 a failing task backs off, turns dead, admin retry succeeds and cancel stops it", func(t *testing.T) {
			var mu sync.Mutex
			var flakyRuns []time.Time
			doomedRuns := 0
			s.identityQ.Handle("walkthrough-flaky", func(context.Context, txqueue.Task) error {
				mu.Lock()
				defer mu.Unlock()
				flakyRuns = append(flakyRuns, time.Now())
				if len(flakyRuns) <= walkthroughQueues.MaxAttempts {
					return fmt.Errorf("flaky dependency down")
				}
				return nil
			})
			s.identityQ.Handle("walkthrough-doomed", func(context.Context, txqueue.Task) error {
				mu.Lock()
				defer mu.Unlock()
				doomedRuns++
				return fmt.Errorf("always failing")
			})
			require.NoError(t, s.identityQ.Enqueue(db, "walkthrough-flaky", struct{}{}, time.Time{}, "flaky-1"))
			require.NoError(t, s.identityQ.Enqueue(db, "walkthrough-doomed", struct{}{}, time.Time{}, "doomed-1"))

			// Two passes back to back: the second finds nothing due.
			for range 2 {
				_, err := s.identityQ.ProcessDue(ctx)
				require.NoError(t, err)
			}
			mu.Lock()
			require.Len(t, flakyRuns, 1, "a failed task waits out its backoff before running again")
			mu.Unlock()
			// Drain until the retries are spent, however slow the database is.
			for deadline := time.Now().Add(30 * time.Second); time.Now().Before(deadline); {
				s.drainFor(t, 500*time.Millisecond)
				mu.Lock()
				spent := len(flakyRuns) >= walkthroughQueues.MaxAttempts && doomedRuns >= walkthroughQueues.MaxAttempts
				mu.Unlock()
				if spent {
					break
				}
			}
			s.drainFor(t, time.Second)

			mu.Lock()
			require.Len(t, flakyRuns, walkthroughQueues.MaxAttempts, "retried up to the attempt limit, then dead")
			for i := 1; i < len(flakyRuns); i++ {
				minWait := walkthroughQueues.BaseBackoff << (i - 1) / 2
				require.GreaterOrEqual(t, flakyRuns[i].Sub(flakyRuns[i-1]), minWait, "backoff grows per attempt")
			}
			require.Equal(t, walkthroughQueues.MaxAttempts, doomedRuns)
			mu.Unlock()

			dead, err := s.queues.ListDeadTasks(ctx)
			require.NoError(t, err)
			byType := map[string]txqueue.DeadTask{}
			for _, task := range dead.Tasks {
				byType[task.Type] = task
			}
			require.Contains(t, byType, "walkthrough-flaky")
			require.Contains(t, byType, "walkthrough-doomed")
			require.Equal(t, "flaky dependency down", byType["walkthrough-flaky"].LastError)

			retried, err := s.deadTasks.RetryDeadTask(ctx, byType["walkthrough-flaky"].UUID)
			require.NoError(t, err)
			require.Equal(t, txqueue.DeadTaskRetried, retried)
			cancelled, err := s.deadTasks.CancelDeadTask(ctx, byType["walkthrough-doomed"].UUID)
			require.NoError(t, err)
			require.Equal(t, txqueue.DeadTaskCancelled, cancelled)
			s.drainFor(t, 500*time.Millisecond)

			mu.Lock()
			require.Len(t, flakyRuns, walkthroughQueues.MaxAttempts+1, "the admin retry ran once and succeeded")
			require.Equal(t, walkthroughQueues.MaxAttempts, doomedRuns, "a cancelled task never runs again")
			mu.Unlock()
			again, err := s.deadTasks.RetryDeadTask(ctx, byType["walkthrough-flaky"].UUID)
			require.NoError(t, err)
			require.Equal(t, txqueue.DeadTaskNotFound, again, "a completed task is gone")
			after, err := s.deadTasks.RetryDeadTask(ctx, byType["walkthrough-doomed"].UUID)
			require.NoError(t, err)
			require.NotEqual(t, txqueue.DeadTaskRetried, after, "a cancelled task cannot be retried")
		}},
		{"P32§2·P12§4 concurrent admissions near a cap never overspend it", func(t *testing.T) {
			const attempts = 30
			admissions := make([]budget.Admission, attempts)
			errs := make([]error, attempts)
			var wg sync.WaitGroup
			for i := range attempts {
				wg.Add(1)
				go func() {
					defer wg.Done()
					admissions[i], errs[i] = s.admit.AdmitWork(ctx, budget.WorkRequest{Category: budget.CategoryExports, Service: walkExport, Quantity: 1,
						Reference: fmt.Sprintf("export-%d", i), MaxDuration: time.Minute})
				}()
			}
			wg.Wait()
			for _, err := range errs {
				require.NoError(t, err)
			}
			admitted := 0
			for _, a := range admissions {
				if a.Outcome == budget.Admitted {
					admitted++
				} else {
					require.Equal(t, budget.RefusedCapReached, a.Reason)
				}
			}
			// 0.10 each against 1.00 with 10 % of each remainder kept free: 9 fit.
			require.Equal(t, 9, admitted)
			status, err := s.status.Status(ctx)
			require.NoError(t, err)
			for _, u := range status.Usage {
				if u.Scope == string(budget.CategoryExports) && u.Window == budget.WindowHour {
					require.LessOrEqual(t, u.Reserved, u.Limit)
					require.InDelta(t, 0.9, u.Reserved, 1e-9)
				}
			}
			for _, a := range admissions {
				if a.Outcome == budget.Admitted {
					_, err := s.settle.SettleWork(ctx, a.ReservationUUID, budget.Settlement{})
					require.NoError(t, err)
				}
			}
			_, err = s.evaluate.Evaluate(ctx)
			require.NoError(t, err)
			require.Equal(t, budget.LevelNormal, s.ladder(t).Level)
		}},
		{"P31§3·P14§4 a notification retried by its producer is accepted once", func(t *testing.T) {
			before := len(resend.Sent())
			s.notify(t, ana, notifications.TypeAccountSecurity, "security-1")
			s.notify(t, ana, notifications.TypeAccountSecurity, "security-1")
			s.drain(t)
			raw, err := json.Marshal(notificationdelivery.Payload{Recipient: ana, Type: string(notifications.TypeAccountSecurity), IdempotencyKey: "security-1"})
			require.NoError(t, err)
			require.NoError(t, s.delivery.Deliver(ctx, identity.QueueDomain, raw))
			s.drain(t)

			entry, count := s.entry(t, ana, notifications.TypeAccountSecurity)
			require.Equal(t, 1, count, "one logical entry per event")
			require.Equal(t, before+1, len(resend.Sent()), "one email")
			require.Equal(t, []walkDelivery{{string(notifications.ProviderResend), string(notifications.DeliverySent), ""}}, s.deliveries(t, entry.EntryUUID))
		}},
		{"P31§2·P31§3 a Resend quota refusal switches to SES and an accepted email is never resent", func(t *testing.T) {
			resendBefore, sesBefore := len(resend.Sent()), len(ses.Sent())
			resend.Inject(notifications.ProviderQuotaRejected)
			s.notify(t, ana, notifications.TypeWelcome, "welcome-1")
			s.drain(t)

			entry, _ := s.entry(t, ana, notifications.TypeWelcome)
			require.Equal(t, resendBefore, len(resend.Sent()))
			require.Equal(t, sesBefore+1, len(ses.Sent()))
			require.Equal(t, []walkDelivery{
				{string(notifications.ProviderResend), string(notifications.DeliveryRejected), notifications.DetailQuota},
				{string(notifications.ProviderSES), string(notifications.DeliverySent), ""},
			}, s.deliveries(t, entry.EntryUUID))

			outcome, err := s.sender.SendEmail(ctx, entry.EntryUUID, notifications.Recipient{Email: "ana@local.playhoot.invalid", Language: notifications.LanguageEnglish})
			require.NoError(t, err)
			require.Equal(t, notifications.EmailAlreadyDecided, outcome)
			require.Equal(t, resendBefore, len(resend.Sent()))
			require.Equal(t, sesBefore+1, len(ses.Sent()), "accepted by one provider, never sent again by either")
		}},
		{"P31§3·P14§4 an email failure keeps the in-app entry unread and never shows it delivered", func(t *testing.T) {
			resend.Inject(notifications.ProviderRefused)
			s.notify(t, ana, notifications.TypePaymentFailed, "payment-1")
			s.drain(t)

			entry, _ := s.entry(t, ana, notifications.TypePaymentFailed)
			require.True(t, entry.Email)
			require.Nil(t, entry.ReadAt)
			for _, d := range s.deliveries(t, entry.EntryUUID) {
				require.NotEqual(t, string(notifications.DeliverySent), d.State)
			}
			require.Equal(t, []walkDelivery{{string(notifications.ProviderResend), string(notifications.DeliveryRejected), notifications.DetailRefused}}, s.deliveries(t, entry.EntryUUID))
		}},
		{"P14§4 listing and sending never mark read; opening one marks only that one", func(t *testing.T) {
			first, err := s.entries.ListEntries(ctx, ana, notifications.EntryFilter{})
			require.NoError(t, err)
			require.Equal(t, 3, first.Unread)
			for _, e := range first.Entries {
				require.Nil(t, e.ReadAt, "emailed entries are not read")
			}
			second, err := s.entries.ListEntries(ctx, ana, notifications.EntryFilter{})
			require.NoError(t, err)
			require.Equal(t, 3, second.Unread, "opening the list marks nothing")

			welcome, _ := s.entry(t, ana, notifications.TypeWelcome)
			marked, err := s.reads.MarkRead(ctx, ana, welcome.EntryUUID)
			require.NoError(t, err)
			require.Equal(t, notifications.MarkedRead, marked)
			marked, err = s.reads.MarkRead(ctx, ana, welcome.EntryUUID)
			require.NoError(t, err)
			require.Equal(t, notifications.AlreadyRead, marked)
			foreign, err := s.reads.MarkRead(ctx, carla, welcome.EntryUUID)
			require.NoError(t, err)
			require.Equal(t, notifications.UnknownEntry, foreign, "only the recipient reads it")
			third, err := s.entries.ListEntries(ctx, ana, notifications.EntryFilter{})
			require.NoError(t, err)
			require.Equal(t, 2, third.Unread)
		}},
		{"P31§5 a bounce reported by SES is honored when Resend would send", func(t *testing.T) {
			body := []byte(`{"events":[{"type":"bounce","email":"ana@local.playhoot.invalid"}]}`)
			header := map[string][]string{emailproviders.SignatureHeader: {hex.EncodeToString(emailproviders.Sign(walkthroughWebhook, body))}}
			recordedCount, err := s.suppressions.ProviderWebhook(ctx, notifications.ProviderSES, header, body)
			require.NoError(t, err)
			require.Equal(t, 1, recordedCount)

			resendBefore, sesBefore := len(resend.Sent()), len(ses.Sent())
			s.notify(t, ana, notifications.TypeAccountSecurity, "security-2")
			s.drain(t)
			require.Equal(t, resendBefore, len(resend.Sent()))
			require.Equal(t, sesBefore, len(ses.Sent()))
			entry, count := s.entry(t, ana, notifications.TypeAccountSecurity)
			require.Equal(t, 2, count, "the in-app entry still exists")
			require.Equal(t, []walkDelivery{{string(notifications.ProviderNone), string(notifications.DeliveryRejected), notifications.DetailSuppressed}}, s.deliveries(t, entry.EntryUUID))
		}},
		{"P35§3·P35§1 a declaration is recorded once, a divergent one refused, a blocked market refused as a service restriction", func(t *testing.T) {
			declared, err := s.declarations.RecordDeclaration(ctx, compliance.Declaration{UserUUID: carla, Country: "CO", MeetsAge: true, Origin: compliance.DeclaredByUser})
			require.NoError(t, err)
			require.Equal(t, compliance.DeclarationRecorded, declared.Outcome)
			again, err := s.declarations.RecordDeclaration(ctx, compliance.Declaration{UserUUID: carla, Country: "CO", MeetsAge: true, Origin: compliance.DeclaredByUser})
			require.NoError(t, err)
			require.Equal(t, compliance.DeclarationUnchanged, again.Outcome)
			divergent, err := s.declarations.RecordDeclaration(ctx, compliance.Declaration{UserUUID: carla, Country: "MX", MeetsAge: true, Origin: compliance.DeclaredByUser})
			require.NoError(t, err)
			require.Equal(t, compliance.DeclarationConflict, divergent.Outcome, "only support corrects a declaration")
			allowed, err := s.declarations.CheckMarketAccess(ctx, compliance.AccessRequest{UserUUID: carla, Function: compliance.FunctionCreateGames})
			require.NoError(t, err)
			require.Equal(t, compliance.AccessAllowed, allowed.Outcome)
			require.Equal(t, "CO", allowed.Country)

			blockedDeclared, err := s.declarations.RecordDeclaration(ctx, compliance.Declaration{UserUUID: admin, Country: "AR", MeetsAge: true, Origin: compliance.DeclaredByUser})
			require.NoError(t, err)
			require.Equal(t, compliance.DeclarationRecorded, blockedDeclared.Outcome)
			blocked, err := s.declarations.CheckMarketAccess(ctx, compliance.AccessRequest{UserUUID: admin, Function: compliance.FunctionCreateGames})
			require.NoError(t, err)
			require.Equal(t, compliance.AccessBlocked, blocked.Outcome)
			require.Equal(t, compliance.BlockMarketUnavailable, blocked.Reason)

			undeclared, err := s.declarations.CheckMarketAccess(ctx, compliance.AccessRequest{UserUUID: ana, Function: compliance.FunctionCreateGames})
			require.NoError(t, err)
			require.Equal(t, compliance.AccessNeedsDeclaration, undeclared.Outcome)
		}},
		{"B·P34 analytics events arrive once from real game and session writes", func(t *testing.T) {
			type event struct {
				EventType      string
				IdempotencyKey string
				Properties     string
			}
			events := func(source string) []event {
				var rows []event
				require.NoError(t, db.Raw(`SELECT event_type, idempotency_key, properties::text AS properties FROM analytics_events WHERE source_domain = ? ORDER BY id`, source).Scan(&rows).Error)
				return rows
			}
			created, err := s.games.CreateGame(ctx, ana, "Walkthrough game", "create-1", nil)
			require.NoError(t, err)
			replayed, err := s.games.CreateGame(ctx, ana, "Walkthrough game", "create-1", nil)
			require.NoError(t, err)
			require.Equal(t, created.GameUUID, replayed.GameUUID)
			s.drain(t)
			s.drain(t)
			gameEvents := events(game.QueueDomain)
			require.Len(t, gameEvents, 1, "a replayed creation adds nothing")
			require.Equal(t, "game-created:"+created.GameUUID, gameEvents[0].IdempotencyKey)

			lobby, err := s.sessions.CreateSession(ctx, ana, walkthroughTapRaceGameUUID, "session-1")
			require.NoError(t, err)
			require.Equal(t, session.CreateSessionCreated, lobby.Outcome)
			commit := session.StartCommit{
				Participants: []session.StartParticipant{{UserUUID: ana, ParticipantID: "p1", DisplayName: "Ana"}, {UserUUID: carla, ParticipantID: "p2", DisplayName: "Carla"}},
				Checkpoint:   session.Checkpoint{ContextRevision: 1, Content: []byte(`{}`)},
			}
			started, err := s.sessions.CommitStart(ctx, lobby.SessionUUID, ana, commit)
			require.NoError(t, err)
			require.Equal(t, session.CommitStartCommitted, started)
			restarted, err := s.sessions.CommitStart(ctx, lobby.SessionUUID, ana, commit)
			require.NoError(t, err)
			require.NotEqual(t, session.CommitStartCommitted, restarted)
			s.drain(t)
			sessionEvents := events(session.QueueDomain)
			require.Len(t, sessionEvents, 1, "a repeated start adds nothing")
			require.Equal(t, "session-started:"+lobby.SessionUUID, sessionEvents[0].IdempotencyKey)
			require.Contains(t, sessionEvents[0].Properties, `"real_participants": true`)
		}},
		{"P32§5·P32§10 a free spend spike restricts, then pauses its category, then shuts down without anyone acting", func(t *testing.T) {
			s.spend(t, budget.CategoryCompute, walkCompute, walkComputeUnit, 82) // USD 8.20: 82 % of the global hour cap
			s.drain(t)
			require.Equal(t, budget.LevelRestricted, s.ladder(t).Level)
			require.Equal(t, budget.RefusedRestricted, s.try(t, budget.CategoryCompute, walkCompute, true).Reason, "free work refused first")
			require.Equal(t, budget.Admitted, s.try(t, budget.CategoryCompute, walkCompute, false).Outcome)
			require.Equal(t, budget.Admitted, s.try(t, budget.CategoryEmail, walkSES, false).Outcome)

			s.spend(t, budget.CategoryCompute, walkCompute, walkComputeUnit, 10) // compute at 97 % of its cap
			s.drain(t)
			state := s.ladder(t)
			require.Equal(t, budget.LevelPaused, state.Level)
			require.Equal(t, []budget.Category{budget.CategoryCompute}, state.PausedCategories)
			require.Equal(t, budget.RefusedPaused, s.try(t, budget.CategoryCompute, walkCompute, false).Reason)
			require.Equal(t, budget.Admitted, s.try(t, budget.CategoryEmail, walkSES, false).Outcome, "other categories continue")

			s.spend(t, budget.CategoryCompute, walkCompute, walkComputeUnit, 10) // USD 10.20: over the global hour cap
			s.drain(t)
			state = s.ladder(t)
			require.Equal(t, budget.LevelShutdown, state.Level)
			require.Equal(t, budget.ActorAutomatic, state.Actor)
			require.Equal(t, budget.RefusedShutdown, s.try(t, budget.CategoryEmail, walkSES, false).Reason, "even necessary work stops")
		}},
		{"P32§6 the shutdown reaches queued work and its retries: no provider is called", func(t *testing.T) {
			resendBefore, sesBefore := len(resend.Sent()), len(ses.Sent())
			s.notify(t, carla, notifications.TypeAccountSecurity, "security-carla")
			s.drain(t)
			entry, _ := s.entry(t, carla, notifications.TypeAccountSecurity)
			// The retries' backoff is short but real: keep draining until they
			// run out, however slow the database is today.
			found := false
			for deadline := time.Now().Add(30 * time.Second); !found && time.Now().Before(deadline); {
				s.drainFor(t, 500*time.Millisecond)
				dead, err := s.queues.ListDeadTasks(ctx)
				require.NoError(t, err)
				for _, task := range dead.Tasks {
					if task.Type == notifications.SendTaskType && json.Valid(task.Payload) {
						var payload notifications.SendTask
						require.NoError(t, json.Unmarshal(task.Payload, &payload))
						found = found || payload.EntryUUID == entry.EntryUUID
					}
				}
			}
			require.True(t, found, "the email's retries ran out while shut down and wait for an admin")
			entry, _ = s.entry(t, carla, notifications.TypeAccountSecurity)
			require.Nil(t, entry.ReadAt)
			require.Empty(t, s.deliveries(t, entry.EntryUUID), "no attempt reached a provider")
			require.Equal(t, resendBefore, len(resend.Sent()))
			require.Equal(t, sesBefore, len(ses.Sent()))
		}},
		{"P32§7 neither a restart nor raised caps lift the shutdown", func(t *testing.T) {
			s = wireH1(t, db, resend, ses)
			require.NoError(t, s.budgetQ.SchedulePeriodic(ctx))
			_, err := s.evaluate.Evaluate(ctx)
			require.NoError(t, err)
			require.Equal(t, budget.LevelShutdown, s.ladder(t).Level)

			_, err = s.costControls.SetControls(ctx, admin, "not-a-proof", budget.Controls{Reason: "no proof"})
			require.Error(t, err, "changing limits needs a fresh step-up proof")
			raised, err := s.costControls.SetControls(ctx, admin, codes.proof(t, s, admin), budget.Controls{Reason: "reviewed: raise caps", Caps: []budget.Cap{
				{Scope: budget.ScopeGlobal, Window: budget.WindowHour, USDLimit: 50},
				{Scope: string(budget.CategoryCompute), Window: budget.WindowHour, USDLimit: 40},
			}})
			require.NoError(t, err)
			require.Equal(t, budget.ControlsRecorded, raised)
			_, err = s.evaluate.Evaluate(ctx)
			require.NoError(t, err)
			require.Equal(t, budget.LevelShutdown, s.ladder(t).Level)
			require.Equal(t, budget.RefusedShutdown, s.try(t, budget.CategoryEmail, walkSES, false).Reason)
		}},
		{"P32§7 reopening needs step-up and is gradual, one category at a time", func(t *testing.T) {
			_, err := s.costControls.Reopen(ctx, admin, "not-a-proof", nil, "no proof")
			require.Error(t, err)
			require.Equal(t, budget.LevelShutdown, s.ladder(t).Level)

			started, err := s.costControls.Reopen(ctx, admin, codes.proof(t, s, admin), nil, "reviewed: start reopening")
			require.NoError(t, err)
			require.Equal(t, budget.ReopenStarted, started)
			state := s.ladder(t)
			require.Equal(t, budget.LevelRestricted, state.Level)
			require.ElementsMatch(t, budget.Categories, state.ReopenClosed, "every category still closed")
			require.Equal(t, budget.RefusedPaused, s.try(t, budget.CategoryEmail, walkSES, false).Reason)

			opened, err := s.costControls.Reopen(ctx, admin, codes.proof(t, s, admin), []budget.Category{budget.CategoryEmail}, "reviewed: email first")
			require.NoError(t, err)
			require.Equal(t, budget.ReopenOpened, opened)
			require.Equal(t, budget.Admitted, s.try(t, budget.CategoryEmail, walkSES, false).Outcome)
			require.Equal(t, budget.RefusedPaused, s.try(t, budget.CategoryCompute, walkCompute, false).Reason, "compute stays closed")
			require.Equal(t, budget.RefusedRestricted, s.try(t, budget.CategoryEmail, walkSES, true).Reason, "free email still waits")
			_, err = s.evaluate.Evaluate(ctx)
			require.NoError(t, err)
			require.Equal(t, budget.LevelRestricted, s.ladder(t).Level, "the evaluation never lifts a gradual reopening")

			// The email stopped by the shutdown goes out only when an admin retries it.
			dead, err := s.queues.ListDeadTasks(ctx)
			require.NoError(t, err)
			sesBefore, resendBefore := len(ses.Sent()), len(resend.Sent())
			for _, task := range dead.Tasks {
				if task.Type != notifications.SendTaskType {
					continue
				}
				var payload notifications.SendTask
				require.NoError(t, json.Unmarshal(task.Payload, &payload))
				if payload.Recipient == carla {
					retried, err := s.deadTasks.RetryDeadTask(ctx, task.UUID)
					require.NoError(t, err)
					require.Equal(t, txqueue.DeadTaskRetried, retried)
				}
			}
			s.drain(t)
			toCarla := 0
			for _, sent := range append(ses.Sent()[sesBefore:], resend.Sent()[resendBefore:]...) {
				if sent.To == "carla@local.playhoot.invalid" {
					toCarla++
				}
			}
			require.Equal(t, 1, toCarla, "sent once, after the admin's retry")
			entry, _ := s.entry(t, carla, notifications.TypeAccountSecurity)
			delivered := s.deliveries(t, entry.EntryUUID)
			require.Len(t, delivered, 1)
			require.Equal(t, string(notifications.DeliverySent), delivered[0].State)
		}},
	}
	for _, step := range steps {
		if !t.Run(step.name, step.run) {
			t.FailNow()
		}
	}
}
