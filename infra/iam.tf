# Service accounts
resource "google_service_account" "triage" {
  account_id   = "triage-sa"
  display_name = "Triage Service Account"
}

resource "google_service_account" "actions" {
  account_id   = "actions-sa"
  display_name = "Actions Service Account"
}

resource "google_service_account" "api" {
  account_id   = "api-sa"
  display_name = "API Service Account"
}

resource "google_service_account" "ui" {
  account_id   = "console-sa"
  display_name = "UI Service Account"
}

# Role sets per service account
locals {
  triage_roles = toset([
    "roles/pubsub.subscriber",
    "roles/pubsub.publisher",
    "roles/datastore.user",
    "roles/secretmanager.secretAccessor",
    "roles/logging.logWriter",
  ])
  actions_roles = toset([
    "roles/pubsub.subscriber",
    "roles/pubsub.publisher",
    "roles/datastore.user",
    "roles/secretmanager.secretAccessor",
    "roles/logging.logWriter",
  ])
  api_roles = toset([
    "roles/pubsub.publisher",
    "roles/datastore.user",
    "roles/secretmanager.secretAccessor",
    "roles/logging.logWriter",
  ])
  ui_roles = toset([
    "roles/datastore.user",
    "roles/pubsub.publisher",
    "roles/logging.logWriter",
  ])
}

resource "google_project_iam_member" "triage_bindings" {
  for_each = local.triage_roles
  project  = var.project_id
  role     = each.key
  member   = "serviceAccount:${google_service_account.triage.email}"
}

resource "google_project_iam_member" "actions_bindings" {
  for_each = local.actions_roles
  project  = var.project_id
  role     = each.key
  member   = "serviceAccount:${google_service_account.actions.email}"
}

resource "google_project_iam_member" "api_bindings" {
  for_each = local.api_roles
  project  = var.project_id
  role     = each.key
  member   = "serviceAccount:${google_service_account.api.email}"
}

resource "google_project_iam_member" "ui_bindings" {
  for_each = local.ui_roles
  project  = var.project_id
  role     = each.key
  member   = "serviceAccount:${google_service_account.ui.email}"
}
