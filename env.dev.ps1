# Common environment for SentinelFlow local dev
$env:GOOGLE_CLOUD_PROJECT      = (gcloud config get-value project)
$env:TOPIC_RAW                 = "alerts.raw"
$env:TOPIC_TRIAGED             = "alerts.triaged"
$env:TOPIC_ACTIONS_QUEUE       = "actions.queue"
$env:FIRESTORE_COLLECTION_ALERTS  = "alerts"
$env:FIRESTORE_COLLECTION_ACTIONS = "actions"
$env:DEV_PULL                  = "1"
