package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	cloudpubsub "cloud.google.com/go/pubsub"

	"github.com/google/uuid"
	"github.com/jinishshah00/sentinelflow/internal/shared"
)

type scenario struct {
	name         string
	eventType    string
	baseDesc     string
	label        shared.Severity
	labels       []string
	severityHint string
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
	if len(os.Args) < 2 {
		fmt.Println("usage: go run . [seed|train|test]")
		os.Exit(2)
	}
	switch os.Args[1] {
	case "seed":
		runSeed() // writes 50 json files into data/udm-samples
	case "train":
		runPublish(true, 50) // publish all 50
	case "test":
		runPublish(false, 30) // publish random 30
	default:
		fmt.Println("unknown mode:", os.Args[1])
		os.Exit(2)
	}
}

func runSeed() {
	root := must(os.Getwd())
	outDir := filepath.Join(root, "data", "udm-samples")
	check(os.MkdirAll(outDir, 0o755))

	scenarios := coreScenarios()
	rand.Seed(time.Now().UnixNano())
	now := time.Now().UTC()

	idCounter := 1
	write := func(ev shared.LabeledEvent, filename string) {
		path := filepath.Join(outDir, filename)
		f := must(os.Create(path))
		defer f.Close()
		enc := json.NewEncoder(f)
		enc.SetIndent("", "  ")
		check(enc.Encode(ev))
	}

	for _, sc := range scenarios {
		for i := 0; i < 10; i++ {
			id := uuid.New().String()
			minAgo := rand.Intn(7 * 24 * 60) // last 7 days
			ts := now.Add(-time.Duration(minAgo) * time.Minute)

			principal := fmt.Sprintf("user:%s@corp.example.com", pick([]string{"alice", "bob", "carol", "dave", "erin"}))
			target := pick([]string{
				"projects/acme-prod/instances/web-01",
				"projects/acme-prod/buckets/site-assets",
				"projects/acme-secops/serviceAccounts/ci-deployer",
				"projects/acme-stg/instances/api-02",
			})
			network := fmt.Sprintf("10.%d.%d.%d/32", rand.Intn(255), rand.Intn(255), rand.Intn(255))

			desc := sc.baseDesc
			if sc.name == "vm_unusual_ingress" {
				desc += fmt.Sprintf(" from %s", pick([]string{"AS15169", "AS13335", "AS14061"}))
			}
			if sc.name == "risky_iam_binding" {
				desc += fmt.Sprintf(" role=%s", pick([]string{"roles/owner", "roles/editor", "roles/iam.serviceAccountKeyAdmin"}))
			}

			ev := shared.LabeledEvent{
				Event: shared.Event{
					ID:           id,
					EventType:    sc.eventType,
					Principal:    principal,
					Target:       target,
					Network:      network,
					SeverityHint: sc.severityHint,
					Labels:       sc.labels,
					Description:  desc,
					TS:           ts,
				},
				Y: sc.label,
			}

			filename := fmt.Sprintf("%03d_%s_%s.json", idCounter, sc.name, strings.ToLower(string(sc.label)))
			write(ev, filename)
			idCounter++
		}
	}

	fmt.Printf("Seeded %d files in %s\n", idCounter-1, filepath.Join(root, "data", "udm-samples"))
}

func runPublish(all bool, count int) {
	projectID := os.Getenv("GOOGLE_CLOUD_PROJECT")
	topicName := os.Getenv("TOPIC_RAW")
	if projectID == "" || topicName == "" {
		log.Fatal("GOOGLE_CLOUD_PROJECT and TOPIC_RAW must be set in env")
	}

	// load labeled samples
	root := must(os.Getwd())
	dir := filepath.Join(root, "data", "udm-samples")
	files := must(os.ReadDir(dir))
	var paths []string
	for _, f := range files {
		if !f.IsDir() && strings.HasSuffix(f.Name(), ".json") {
			paths = append(paths, filepath.Join(dir, f.Name()))
		}
	}
	sort.Strings(paths)

	var labeled []shared.LabeledEvent
	for _, p := range paths {
		var ev shared.LabeledEvent
		b := must(os.ReadFile(p))
		check(json.Unmarshal(b, &ev))
		labeled = append(labeled, ev)
	}

	if !all {
		// pick a random 30
		rand.Shuffle(len(labeled), func(i, j int) { labeled[i], labeled[j] = labeled[j], labeled[i] })
		if len(labeled) > count {
			labeled = labeled[:count]
		}
	} // if all=true we use all 50

	ctx := context.Background()
	client := must(cloudpubsub.NewClient(ctx, projectID))
	defer client.Close()
	topic := client.Topic(topicName)

	published := 0
	for _, lv := range labeled {
		raw := shared.Event{
			ID:           lv.ID,
			EventType:    lv.EventType,
			Principal:    lv.Principal,
			Target:       lv.Target,
			Network:      lv.Network,
			SeverityHint: lv.SeverityHint,
			Labels:       lv.Labels,
			Description:  lv.Description,
			TS:           lv.TS,
		}
		payload := must(json.Marshal(raw))
		msg := &cloudpubsub.Message{
			Data: payload,
			Attributes: map[string]string{
				"id":            lv.ID,
				"event_type":    lv.EventType,
				"gt_y":          string(lv.Y), // ground truth for later metrics
				"severity_hint": lv.SeverityHint,
			},
		}
		// publish synchronously for clarity
		id := must(topic.Publish(ctx, msg).Get(ctx))
		_ = id
		published++
	}
	fmt.Printf("Published %d messages to %s (project %s)\n", published, topicName, projectID)
}

func coreScenarios() []scenario {
	return []scenario{
		{"risky_iam_binding", "iam.setIamPolicy.bindingAdd", "IAM binding added granting elevated role", shared.SeverityHigh, []string{"iam", "policy", "role", "elevated"}, "high"},
		{"sa_key_created", "iam.serviceAccountKeys.create", "New service account key created outside change window", shared.SeverityHigh, []string{"serviceAccount", "key", "credential"}, "high"},
		{"vm_unusual_ingress", "compute.firewall.ingress", "Unusual ingress observed to VM NIC from unfamiliar ASN", shared.SeverityMedium, []string{"network", "ingress", "nic", "asn"}, "medium"},
		{"public_bucket_policy", "storage.setIamPolicy.public", "Bucket policy changed to allow public read", shared.SeverityMedium, []string{"storage", "bucket", "policy", "public"}, "medium"},
		{"mass_deletes_storage", "storage.objects.delete.bulk", "Burst of object deletions detected in short window", shared.SeverityHigh, []string{"storage", "delete", "bulk"}, "high"},
	}
}

func pick[T any](list []T) T { return list[rand.Intn(len(list))] }
