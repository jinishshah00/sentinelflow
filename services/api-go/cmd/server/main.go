package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
	cloudpubsub "cloud.google.com/go/pubsub"
	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	smpb "cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
	"google.golang.org/api/iterator"

	"github.com/google/uuid"
	"github.com/jinishshah00/sentinelflow/internal/shared"
)

// -------- shared local types (match what triage writes) --------
type triageResult struct {
	Severity     shared.Severity `json:"severity" firestore:"severity"`
	Confidence   float64         `json:"confidence" firestore:"confidence"`
	ReasonTokens []string        `json:"reason_tokens" firestore:"reason_tokens"`
}

type alertDoc struct {
	AlertID string       `json:"alert_id" firestore:"alert_id"`
	Event   shared.Event `json:"event" firestore:"event"`
	Triage  triageResult `json:"triage" firestore:"triage"`
	Status  string       `json:"status" firestore:"status"`
	Created time.Time    `json:"created" firestore:"created"`
}

type actionDoc struct {
	ActionID       string            `json:"action_id" firestore:"action_id"`
	AlertID        string            `json:"alert_id" firestore:"alert_id"`
	ProposedAction string            `json:"proposed_action" firestore:"proposed_action"`
	Status         string            `json:"status" firestore:"status"` // queued|executed
	Simulation     bool              `json:"simulation" firestore:"simulation"`
	Details        map[string]string `json:"details,omitempty" firestore:"details"`
	Created        time.Time         `json:"created" firestore:"created"`
	ExecutedAt     *time.Time        `json:"executed_at,omitempty" firestore:"executed_at"`
}

// ----------------- globals -----------------
var (
	projectID    string
	apiKey       string // loaded from Secret Manager
	apiSecret    string // secret id, default: API_KEY
	fsClient     *firestore.Client
	pubClient    *cloudpubsub.Client
	smClient     *secretmanager.Client
	alertsCol    string
	actionsCol   string
	topicActions string
	slackSecret  string
)

// ----------------- helpers -----------------
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

func main() {
	ctx := context.Background()

	projectID = getenv("GOOGLE_CLOUD_PROJECT", "")
	if projectID == "" {
		log.Fatal("GOOGLE_CLOUD_PROJECT must be set")
	}
	apiSecret = getenv("API_SECRET_ID", "API_KEY")
	slackSecret = getenv("SLACK_SECRET_ID", "SLACK_WEBHOOK")
	alertsCol = getenv("FIRESTORE_COLLECTION_ALERTS", "alerts")
	actionsCol = getenv("FIRESTORE_COLLECTION_ACTIONS", "actions")
	topicActions = getenv("TOPIC_ACTIONS_QUEUE", "actions.queue")

	// clients
	fsClient = must(firestore.NewClient(ctx, projectID))
	pubClient = must(cloudpubsub.NewClient(ctx, projectID))
	smClient = must(secretmanager.NewClient(ctx))

	// load API key once
	apiKey = loadSecret(ctx, apiSecret)
	if apiKey == "" {
		log.Fatal("API key missing in Secret Manager")
	}

	// http mux
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", handleHealth)
	mux.HandleFunc("/alerts", withAuth(handleListAlerts))
	mux.HandleFunc("/alerts/", withAuth(handleAlertByID)) // /alerts/{id}
	mux.HandleFunc("/metrics", withAuth(handleMetrics))

	addr := ":" + getenv("PORT", "8083")
	log.Printf("api-go listening on %s", addr)
	check(http.ListenAndServe(addr, mux))
}

// ------------- middleware -------------
func withAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("X-API-Key")
		if key == "" || key != apiKey {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

// ------------- handlers -------------
func handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"service": "api-go",
		"status":  "ok",
		"time":    time.Now().UTC(),
	})
}

func handleListAlerts(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	limit := 50
	if q := r.URL.Query().Get("limit"); q != "" {
		if n, err := strconv.Atoi(q); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}

	iter := fsClient.Collection(alertsCol).OrderBy("created", firestore.Desc).Limit(limit).Documents(ctx)
	var out []alertDoc
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.Printf("firestore list error: %v", err)
			http.Error(w, "firestore error", http.StatusInternalServerError)
			return
		}
		var a alertDoc
		if err := doc.DataTo(&a); err != nil {
			log.Printf("decode error: %v", err)
			http.Error(w, "decode error", http.StatusInternalServerError)
			return
		}
		out = append(out, a)
	}
	writeJSON(w, http.StatusOK, map[string]any{"alerts": out})

}

func handleAlertByID(w http.ResponseWriter, r *http.Request) {
	// paths: /alerts/{id} [GET], /alerts/{id}/approve [POST]
	ctx := r.Context()
	path := strings.TrimPrefix(r.URL.Path, "/alerts/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	id := parts[0]

	if len(parts) == 1 && r.Method == http.MethodGet {
		doc, err := fsClient.Collection(alertsCol).Doc(id).Get(ctx)
		if err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		var a alertDoc
		if err := doc.DataTo(&a); err != nil {
			http.Error(w, "decode error", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, a)
		return
	}

	if len(parts) == 2 && parts[1] == "approve" && r.Method == http.MethodPost {
		// fetch alert
		docRef := fsClient.Collection(alertsCol).Doc(id)
		doc, err := docRef.Get(ctx)
		if err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		var a alertDoc
		if err := doc.DataTo(&a); err != nil {
			http.Error(w, "decode error", http.StatusInternalServerError)
			return
		}
		if a.Status != "awaiting_approval" {
			http.Error(w, "alert not awaiting approval", http.StatusConflict)
			return
		}

		// record an executed simulated remediation (post-approval)
		now := time.Now().UTC()
		ad := actionDoc{
			ActionID:       uuid.New().String(),
			AlertID:        id,
			ProposedAction: "remediate_require_approval",
			Status:         "executed",
			Simulation:     true,
			Details:        map[string]string{"note": "approved via API"},
			Created:        now,
			ExecutedAt:     &now,
		}
		_, err = fsClient.Collection(actionsCol).Doc(ad.ActionID).Set(ctx, ad)
		if err != nil {
			http.Error(w, "firestore write error", http.StatusInternalServerError)
			return
		}

		// update alert status
		_, err = docRef.Update(ctx, []firestore.Update{{Path: "status", Value: "action_executed"}})
		if err != nil {
			http.Error(w, "firestore update error", http.StatusInternalServerError)
			return
		}

		// publish to actions.queue for visibility
		b, _ := json.Marshal(ad)
		_, _ = pubClient.Topic(getenv("TOPIC_ACTIONS_QUEUE", "actions.queue")).Publish(ctx, &cloudpubsub.Message{
			Data: b,
			Attributes: map[string]string{
				"alert_id": id,
				"action":   ad.ProposedAction,
			},
		}).Get(ctx)

		// Slack notify
		webhook := loadSecret(ctx, slackSecret)
		if webhook != "" {
			msg := fmt.Sprintf(":white_check_mark: Approval granted for alert `%s`; simulated remediation recorded.", id)
			_ = postSlack(ctx, webhook, msg)
		}

		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "alert_id": id})
		return
	}

	http.NotFound(w, r)
}

func handleMetrics(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	iter := fsClient.Collection(alertsCol).OrderBy("created", firestore.Desc).Limit(200).Documents(ctx)
	type C struct{ Low, Med, High, Awaiting, Executed, Pending int }
	var c C
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.Printf("firestore metrics error: %v", err)
			http.Error(w, "firestore error", http.StatusInternalServerError)
			return
		}
		var a alertDoc
		if err := doc.DataTo(&a); err != nil {
			continue
		}
		switch a.Triage.Severity {
		case shared.SeverityLow:
			c.Low++
		case shared.SeverityMedium:
			c.Med++
		case shared.SeverityHigh:
			c.High++
		}
		switch a.Status {
		case "awaiting_approval":
			c.Awaiting++
		case "action_executed":
			c.Executed++
		case "pending":
			c.Pending++
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"sample_window": 200, "counts": c})

}

// ----------------- utils -----------------
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func loadSecret(ctx context.Context, secretID string) string {
	name := fmt.Sprintf("projects/%s/secrets/%s/versions/latest", projectID, secretID)
	resp, err := smClient.AccessSecretVersion(ctx, &smpb.AccessSecretVersionRequest{Name: name})
	if err != nil {
		log.Printf("secret %s error: %v", secretID, err)
		return ""
	}
	return string(resp.Payload.Data)
}

func postSlack(ctx context.Context, webhook, text string) error {
	body := map[string]any{"text": text}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequestWithContext(ctx, "POST", webhook, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	return nil
}
