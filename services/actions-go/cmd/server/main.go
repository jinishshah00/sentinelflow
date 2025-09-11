package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"cloud.google.com/go/firestore"
	cloudpubsub "cloud.google.com/go/pubsub"
	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	smpb "cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"

	"github.com/google/uuid"

	"github.com/jinishshah00/sentinelflow/internal/shared"
)

// ----------- types -----------
type triageEnvelope struct {
	Event  shared.Event `json:"event"`
	Triage struct {
		Severity     shared.Severity `json:"severity"`
		Confidence   float64         `json:"confidence"`
		ReasonTokens []string        `json:"reason_tokens"`
	} `json:"triage"`
}

type actionDoc struct {
	ActionID       string            `json:"action_id"`
	AlertID        string            `json:"alert_id"`
	ProposedAction string            `json:"proposed_action"`
	Status         string            `json:"status"` // queued|awaiting_approval|executed
	Simulation     bool              `json:"simulation"`
	Details        map[string]string `json:"details,omitempty"`
	Created        time.Time         `json:"created"`
	ExecutedAt     *time.Time        `json:"executed_at,omitempty"`
}

// ----------- globals -----------
var (
	projectID    string
	fsClient     *firestore.Client
	pubClient    *cloudpubsub.Client
	smClient     *secretmanager.Client
	actionsCol   string
	alertsCol    string
	devPull      bool
	subPull      string
	slackSecret  string
	topicActions string
)

// ----------- helpers -----------
func getenv(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}
func check(err error) {
	if err != nil {
		panic(err)
	}
}

// ----------- main -----------
func main() {
	ctx := context.Background()

	projectID = getenv("GOOGLE_CLOUD_PROJECT", "")
	actionsCol = getenv("FIRESTORE_COLLECTION_ACTIONS", "actions")
	alertsCol = getenv("FIRESTORE_COLLECTION_ALERTS", "alerts")
	devPull = getenv("DEV_PULL", "") == "1"
	subPull = getenv("SUBSCRIPTION_PULL", "actions-dev")
	slackSecret = getenv("SLACK_SECRET_ID", "SLACK_WEBHOOK")
	topicActions = getenv("TOPIC_ACTIONS_QUEUE", "actions.queue")

	if projectID == "" {
		log.Fatal("GOOGLE_CLOUD_PROJECT must be set")
	}

	// clients
	fsClient = must(firestore.NewClient(ctx, projectID))
	pubClient = must(cloudpubsub.NewClient(ctx, projectID))
	smClient = must(secretmanager.NewClient(ctx))

	// http server (health + future push endpoint)
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"service": "actions-go",
			"status":  "ok",
			"time":    time.Now().UTC(),
			"devPull": devPull,
		})
	})
	mux.HandleFunc("/pubsub/push", handlePush)

	addr := ":" + getenv("PORT", "8082")
	go func() {
		log.Printf("actions-go listening on %s", addr)
		check(http.ListenAndServe(addr, mux))
	}()

	// local dev: pull subscription on alerts.triaged
	if devPull {
		go runPuller(ctx)
	}

	select {}
}

// ----------- pull / push -----------
func runPuller(ctx context.Context) {
	sub := pubClient.Subscription(subPull)
	sub.ReceiveSettings.Synchronous = true
	sub.ReceiveSettings.MaxOutstandingMessages = 10

	log.Printf("actions-go: starting pull on subscription %q", subPull)
	err := sub.Receive(ctx, func(ctx context.Context, msg *cloudpubsub.Message) {
		defer msg.Ack()
		var env triageEnvelope
		if err := json.Unmarshal(msg.Data, &env); err != nil {
			log.Printf("bad triaged message: %v", err)
			return
		}
		process(ctx, env)
	})
	if err != nil {
		log.Fatalf("actions-go pull error: %v", err)
	}
}

func handlePush(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var envelope struct {
		Message struct {
			Data []byte `json:"data"`
		} `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&envelope); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	var env triageEnvelope
	if err := json.Unmarshal(envelope.Message.Data, &env); err != nil {
		http.Error(w, "bad data", http.StatusBadRequest)
		return
	}
	process(ctx, env)
	w.WriteHeader(http.StatusNoContent)
}

// ----------- core -----------
func process(ctx context.Context, env triageEnvelope) {
	act, needsApproval := decide(env.Event, env.Triage.Severity)

	if act == "" {
		// nothing to do for low/noise; update alert status lightly
		_, _ = fsClient.Collection(alertsCol).Doc(env.Event.ID).
			Update(ctx, []firestore.Update{{Path: "status", Value: "reviewed"}})
		return
	}

	now := time.Now().UTC()
	a := actionDoc{
		ActionID:       uuid.New().String(),
		AlertID:        env.Event.ID,
		ProposedAction: act,
		Status:         "queued",
		Simulation:     true, // ALWAYS simulated in prototype
		Details:        map[string]string{"severity": string(env.Triage.Severity)},
		Created:        now,
	}

	// record action
	_, err := fsClient.Collection(actionsCol).Doc(a.ActionID).Set(ctx, a)
	if err != nil {
		log.Printf("firestore actions set error: %v", err)
	}

	// publish to actions.queue (for future workers)
	payload, _ := json.Marshal(a)
	_, err = pubClient.Topic(topicActions).Publish(ctx, &cloudpubsub.Message{
		Data: payload,
		Attributes: map[string]string{
			"alert_id": env.Event.ID,
			"action":   a.ProposedAction,
		},
	}).Get(ctx)
	if err != nil {
		log.Printf("pub actions.queue error: %v", err)
	}

	if needsApproval {
		// set alert awaiting approval, ping Slack
		_, _ = fsClient.Collection(alertsCol).Doc(env.Event.ID).
			Update(ctx, []firestore.Update{{Path: "status", Value: "awaiting_approval"}})
		notifySlack(ctx, fmt.Sprintf(":warning: Approval requested for *%s* on alert `%s` (severity=%s)",
			act, env.Event.ID, env.Triage.Severity))
		log.Printf("queued approval for %s action=%s", env.Event.ID, act)
		return
	}

	// simulate immediate execution
	res := simulate(act, env.Event)
	a.Status = "executed"
	a.Details["result"] = res
	execTime := time.Now().UTC()
	a.ExecutedAt = &execTime
	_, _ = fsClient.Collection(actionsCol).Doc(a.ActionID).Set(ctx, a)
	_, _ = fsClient.Collection(alertsCol).Doc(env.Event.ID).
		Update(ctx, []firestore.Update{{Path: "status", Value: "action_executed"}})

	notifySlack(ctx, fmt.Sprintf(":white_check_mark: Executed *%s* on alert `%s` (simulated) — result: %s",
		act, env.Event.ID, res))
	log.Printf("executed action=%s for alert=%s", act, env.Event.ID)
}

func decide(ev shared.Event, sev shared.Severity) (action string, needsApproval bool) {
	switch ev.EventType {
	case "iam.setIamPolicy.bindingAdd":
		if sev == shared.SeverityHigh {
			return "require_approval", true
		}
	case "iam.serviceAccountKeys.create":
		if sev == shared.SeverityHigh {
			return "revoke_sa_key", false
		}
	case "storage.setIamPolicy.public":
		if sev == shared.SeverityHigh || sev == shared.SeverityMedium {
			return "revert_bucket_policy", false
		}
	case "compute.firewall.ingress":
		if sev == shared.SeverityHigh || sev == shared.SeverityMedium {
			return "isolate_vm_nic", false
		}
	}
	// default: nothing (let human review)
	return "", false
}

func simulate(action string, ev shared.Event) string {
	switch action {
	case "revoke_sa_key":
		return "would call iam.projects.serviceAccounts.keys.delete"
	case "revert_bucket_policy":
		return "would set bucket policy to private (remove allUsers/allAuthenticatedUsers)"
	case "isolate_vm_nic":
		return "would tag instance and apply deny-all ingress firewall"
	default:
		return "noop"
	}
}

// ----------- slack -----------
func notifySlack(ctx context.Context, text string) {
	webhook := getSlackWebhook(ctx)
	if webhook == "" {
		log.Printf("slack: webhook missing; message: %s", text)
		return
	}
	body := map[string]any{"text": text}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequestWithContext(ctx, "POST", webhook, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("slack error: %v", err)
		return
	}
	_ = resp.Body.Close()
}

var slackCache string

func getSlackWebhook(ctx context.Context) string {
	if slackCache != "" {
		return slackCache
	}
	name := fmt.Sprintf("projects/%s/secrets/%s/versions/latest", projectID, slackSecret)
	resp, err := smClient.AccessSecretVersion(ctx, &smpb.AccessSecretVersionRequest{Name: name})
	if err != nil {
		log.Printf("secret manager access error: %v", err)
		return ""
	}
	slackCache = string(resp.Payload.Data)
	return slackCache
}
