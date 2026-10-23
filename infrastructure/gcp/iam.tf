variable "runtime_service_account_id" {
  type = string
  default = "pixelgo-analytics-runtime"
}
resource "google_service_account" "runtime" {
  account_id = var.runtime_service_account_id
  display_name = "Pixel Go analytics runtime"
}
resource "google_storage_bucket_iam_member" "lake_reader" {
  bucket = google_storage_bucket.lake.name
  role = "roles/storage.objectViewer"
  member = "serviceAccount:${google_service_account.runtime.email}"
}
resource "google_bigquery_dataset_iam_member" "dataset_reader" {
  dataset_id = google_bigquery_dataset.analytics.dataset_id
  role = "roles/bigquery.dataViewer"
  member = "serviceAccount:${google_service_account.runtime.email}"
}
output "runtime_service_account" { value = google_service_account.runtime.email }
