# Integration status and deployment instructions

The files added in this branch are **environment-ready examples**, not claims of active deployed infrastructure. Cloud integrations require credentials, billing and controlled provisioning.

## GCP
Set up an authorized Google Cloud project, enable BigQuery and Cloud Storage, and authenticate using workload identity federation (recommended) rather than permanent keys. From `infrastructure/gcp`, run `terraform init`, `terraform validate`, then review `terraform plan -var=project_id=PROJECT -var=bucket_name=UNIQUE_NAME`. Applying the plan creates billed resources; nothing is applied automatically. Dataset access is isolated via dedicated service accounts.

## Security / DevSecOps
CI validates Terraform syntax, Python where applicable and Kubernetes manifests. Security scans and project-specific CI remain required gates. Review image digests, SBOM, dependency vulnerabilities, secret scanning, OIDC/IAM boundaries, network egress, data retention and audit logging prior to deployment. A passing static validation does **not** assert integration, privacy or operational readiness.

## Real-time
Kafka consumer examples are opt-in and require actual brokers plus durable DLQ/replay configuration. Verify ordering, outbox atomicity, backpressure, idempotency, checkpoints, monitoring and alerting before production use. No live service was provisioned.

## Kubernetes
Run `kubectl kustomize infrastructure/kubernetes`. Pin image to a built immutable digest and audit nonroot user, configured port, secrets, DB/Redis endpoints and probes before applying. The deployment remains an example until cluster and cloud credentials are available.
