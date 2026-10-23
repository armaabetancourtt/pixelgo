terraform {
  required_version = ">= 1.6.0"
  required_providers {
    google = { source = "hashicorp/google", version = "~> 6.0" }
  }
}
variable "project_id" { type = string }
variable "region" { type = string, default = "us-central1" }
variable "bucket_name" { type = string }
provider "google" { project = var.project_id, region = var.region }
resource "google_storage_bucket" "lake" {
  name = var.bucket_name
  location = var.region
  uniform_bucket_level_access = true
  public_access_prevention = "enforced"
  force_destroy = false
  versioning { enabled = true }
}
resource "google_bigquery_dataset" "analytics" {
  dataset_id = "portfolio_analytics"
  location = "US"
  delete_contents_on_destroy = false
}
output "lake_bucket" { value = google_storage_bucket.lake.name }
output "warehouse_dataset" { value = google_bigquery_dataset.analytics.dataset_id }
