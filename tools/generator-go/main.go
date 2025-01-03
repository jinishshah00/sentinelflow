package main

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"

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
		fmt.Println("usage: go run . seed")
		os.Exit(2)
	}
	switch os.Args[1] {
	case "seed":
		runSeed()
	default:
		fmt.Println("unknown mode:", os.Args[1])
		os.Exit(2)
	}
}

func runSeed() {
	// Ensure output dir
	root := must(os.Getwd())
	outDir := filepath.Join(root, "data", "udm-samples")
	check(os.MkdirAll(outDir, 0o755))

	// Five core scenarios (10 files each = 50 total)
	scenarios := []scenario{
		{
			name:         "risky_iam_binding",
			eventType:    "iam.setIamPolicy.bindingAdd",
			baseDesc:     "IAM binding added granting elevated role",
			label:        shared.SeverityHigh,
			labels:       []string{"iam", "policy", "role", "elevated"},
			severityHint: "high",
		},
		{
			name:         "sa_key_created",
			eventType:    "iam.serviceAccountKeys.create",
			baseDesc:     "New service account key created outside change window",
			label:        shared.SeverityHigh,
			labels:       []string{"serviceAccount", "key", "credential"},
			severityHint: "high",
		},
		{
			name:         "vm_unusual_ingress",
			eventType:    "compute.firewall.ingress",
			baseDesc:     "Unusual ingress observed to VM NIC from unfamiliar ASN",
			label:        shared.SeverityMedium,
			labels:       []string{"network", "ingress", "nic", "asn"},
			severityHint: "medium",
		},
		{
			name:         "public_bucket_policy",
			eventType:    "storage.setIamPolicy.public",
			baseDesc:     "Bucket policy changed to allow public read",
			label:        shared.SeverityMedium,
			labels:       []string{"storage", "bucket", "policy", "public"},
			severityHint: "medium",
		},
		{
			name:         "mass_deletes_storage",
			eventType:    "storage.objects.delete.bulk",
			baseDesc:     "Burst of object deletions detected in short window",
			label:        shared.SeverityHigh,
			labels:       []string{"storage", "delete", "bulk"},
			severityHint: "high",
		},
	}

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
			minAgo := rand.Intn(7 * 24 * 60) // within last 7 days
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

	fmt.Printf("Seeded %d files in %s\n", idCounter-1, outDir)
}

func pick[T any](list []T) T {
	return list[rand.Intn(len(list))]
}
