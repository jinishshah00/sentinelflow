package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"cloud.google.com/go/firestore"
	cloudpubsub "cloud.google.com/go/pubsub"

	"github.com/jinishshah00/sentinelflow/internal/shared"
	"github.com/jinishshah00/sentinelflow/internal/shared/classifier"
)

// ---------- helpers ----------
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

type pubsubPush struct {
	Message struct {
		Data       []byte            `json:"data"`
		Attributes map[string]string `json:"attributes"`
	} `json:"message"`
	Subscription string `json:"subscription"`
}

type triageResult struct {
	Severity     shared.Severity `json:"severity"`
	Confidence   float64         `json:"confidence"`
	ReasonTokens []string        `json:"reason_tokens"`
}

// ---------- globals ----------
var (
	nb           *classifier.NB
	fsClient     *firestore.Client
	pubClient    *cloudpubsub.Client
	projectID    string
	topicTriaged string
	fsAlertsCol  string
	devPull      bool
	subPull      string
)

func main() {
	ctx := context.Background()

	// env
	projectID = getenv("GOOGLE_CLOUD_PROJECT", "")
	topicTriaged = getenv("TOPIC_TRIAGED", "alerts.triaged")
	fsAlertsCol = getenv("FIRESTORE_COLLECTION_ALERTS", "alerts")
	devPull = getenv("DEV_PULL", "") == "1"
	subPull = getenv("SUBSCRIPTION_PULL", "triage-dev")

	if projectID == "" {
		log.Fatal("GOOGLE_CLOUD_PROJECT must be set")
	}

	// classifier: train from data dir (works both local & Cloud Run)
	nb = classifier.New(1.0)
	var train []shared.LabeledEvent
	root, _ := os.Getwd()
	dataDir := getenv("DATA_DIR", root+"/data/udm-samples")

	entries, err := os.ReadDir(dataDir)
	if err != nil {
		log.Fatalf("cannot read training data dir %q: %v", dataDir, err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		b := must(os.ReadFile(dataDir + "/" + e.Name()))
		var le shared.LabeledEvent
		check(json.Unmarshal(b, &le))
		train = append(train, le)
	}
	nb.Train(train)
	log.Printf("triage-go: trained on %d labeled events (dir=%s)", len(train), dataDir)

	// clients
	fsClient = must(firestore.NewClient(ctx, projectID))
	pubClient = must(cloudpubsub.NewClient(ctx, projectID))

	// http mux
	mux := http.NewServeMux()

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		shared.WriteJSON(w, http.StatusOK, map[string]any{
			"service": "triage-go",
			"status":  "ok",
			"time":    time.Now().UTC(),
			"devPull": devPull,
		})
	})

	// optional: make "/" return something simple
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("triage-go alive"))
	})

	mux.HandleFunc("/pubsub/push", handlePush)

	// start optional puller first so it runs alongside the server
	if devPull {
		go runPuller(ctx)
	}

	// graceful shutdown watcher
	port := getenv("PORT", "8080")
	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
		<-ch
		log.Printf("shutting down http server")
		shCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shCtx); err != nil {
			log.Printf("http shutdown error: %v", err)
		}
	}()

	log.Printf("triage-go listening on :%s", port)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("ListenAndServe error: %v", err)
	}

}

func handlePush(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var envelope pubsubPush
	if err := json.NewDecoder(r.Body).Decode(&envelope); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	var ev shared.Event
	if err := json.Unmarshal(envelope.Message.Data, &ev); err != nil {
		http.Error(w, "bad data", http.StatusBadRequest)
		return
	}
	process(ctx, ev)
	w.WriteHeader(http.StatusNoContent)
}

func runPuller(ctx context.Context) {
	sub := pubClient.Subscription(subPull)
	sub.ReceiveSettings.Synchronous = true
	sub.ReceiveSettings.MaxOutstandingMessages = 10

	log.Printf("triage-go: starting pull on subscription %q", subPull)
	err := sub.Receive(ctx, func(ctx context.Context, msg *cloudpubsub.Message) {
		defer msg.Ack()
		var ev shared.Event
		if err := json.Unmarshal(msg.Data, &ev); err != nil {
			log.Printf("bad message: %v", err)
			return
		}
		process(ctx, ev)
	})
	if err != nil {
		log.Fatalf("pull error: %v", err)
	}
}

func process(ctx context.Context, ev shared.Event) {
	// classify
	y, conf, reasons := nb.Predict(ev)

	// deterministic policy bump
	if ev.EventType == "iam.serviceAccountKeys.create" {
		y = shared.SeverityHigh
	}
	if ev.EventType == "storage.setIamPolicy.public" && y == shared.SeverityLow {
		y = shared.SeverityMedium
	}

	res := triageResult{Severity: y, Confidence: conf, ReasonTokens: reasons}

	// write to Firestore
	doc := map[string]any{
		"alert_id": ev.ID,
		"event":    ev,
		"triage":   res,
		"status":   "pending",
		"created":  time.Now().UTC(),
	}
	_, err := fsClient.Collection(fsAlertsCol).Doc(ev.ID).Set(ctx, doc)
	if err != nil {
		log.Printf("firestore set error: %v", err)
	}

	// publish to alerts.triaged
	payload := map[string]any{
		"event":  ev,
		"triage": res,
	}
	b, _ := json.Marshal(payload)
	topic := pubClient.Topic(topicTriaged)
	id, err := topic.Publish(ctx, &cloudpubsub.Message{
		Data: b,
		Attributes: map[string]string{
			"severity":   string(y),
			"confidence": formatFloat(conf),
			"source":     "triage-go",
		},
	}).Get(ctx)
	if err != nil {
		log.Printf("publish triaged error: %v", err)
	} else {
		log.Printf("triaged %s -> %s (id=%s) severity=%s conf=%.3f reasons=%v",
			ev.ID, topicTriaged, id, y, conf, reasons)
	}
}

func getenv(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
func formatFloat(f float64) string {
	return strings.TrimRight(strings.TrimRight(fmtFloat(f, 3), "0"), ".")
}
func fmtFloat(f float64, prec int) string {
	return strconv.FormatFloat(f, 'f', prec, 64)
}
