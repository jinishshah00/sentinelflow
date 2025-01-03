terraform {
  required_version = ">= 1.6.0"
  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 5.43"
    }
  }
}

provider "google" {
  project = var.project_id
  region  = var.region
}
