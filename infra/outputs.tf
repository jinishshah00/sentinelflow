output "project_id" {
  value = var.project_id
}

output "region" {
  value = var.region
}

output "service_accounts" {
  value = {
    triage  = google_service_account.triage.email
    actions = google_service_account.actions.email
    api     = google_service_account.api.email
    ui      = google_service_account.ui.email
  }
}

output "pubsub_topics" {
  value = {
    alerts_raw     = google_pubsub_topic.alerts_raw.name
    alerts_triaged = google_pubsub_topic.alerts_triaged.name
    actions_queue  = google_pubsub_topic.actions_queue.name
  }
}

output "artifact_registry_repo" {
  value = "${var.region}-docker.pkg.dev/${var.project_id}/${google_artifact_registry_repository.sentinelflow.repository_id}"
}

output "secrets_created" {
  value = [
    google_secret_manager_secret.slack_webhook.name,
    google_secret_manager_secret.api_key.name
  ]
}
