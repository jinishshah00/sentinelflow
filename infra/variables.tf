variable "project_id" {
  description = "GCP project id"
  type        = string
}

variable "region" {
  description = "Default region for regional resources"
  type        = string
  default     = "us-central1"
}

variable "firestore_location" {
  description = "Firestore database location (e.g., us-central)"
  type        = string
  default     = "nam5"
}
