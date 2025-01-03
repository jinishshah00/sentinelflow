# We create the secrets but NOT the versions. You will add versions via gcloud after apply.
resource "google_secret_manager_secret" "slack_webhook" {
  secret_id = "SLACK_WEBHOOK"
  replication {
    auto {}
  }
  depends_on = [google_project_service.enabled]
}

resource "google_secret_manager_secret" "api_key" {
  secret_id = "API_KEY"
  replication {
    auto {}
  }
  depends_on = [google_project_service.enabled]
}
