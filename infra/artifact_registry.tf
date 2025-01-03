resource "google_artifact_registry_repository" "sentinelflow" {
  location      = var.region
  repository_id = "sentinelflow"
  description   = "Containers for SentinelFlow"
  format        = "DOCKER"
  depends_on    = [google_project_service.enabled]
}
