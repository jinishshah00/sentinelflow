resource "google_pubsub_topic" "alerts_raw" {
  name       = "alerts.raw"
  depends_on = [google_project_service.enabled]
}

resource "google_pubsub_topic" "alerts_triaged" {
  name       = "alerts.triaged"
  depends_on = [google_project_service.enabled]
}

resource "google_pubsub_topic" "actions_queue" {
  name       = "actions.queue"
  depends_on = [google_project_service.enabled]
}
