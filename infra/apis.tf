locals {
  apis = [
    "run.googleapis.com",
    "pubsub.googleapis.com",
    "firestore.googleapis.com",
    "artifactregistry.googleapis.com",
    "secretmanager.googleapis.com",
    "logging.googleapis.com",
    "cloudbuild.googleapis.com"
  ]
}

resource "google_project_service" "enabled" {
  for_each           = toset(local.apis)
  project            = var.project_id
  service            = each.key
  disable_on_destroy = false
}
