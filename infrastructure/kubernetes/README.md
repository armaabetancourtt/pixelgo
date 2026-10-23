# Experimental Kubernetes workload manifest

This is a *security-oriented deployment scaffold*, not deploy-ready production configuration. Replace image with a digest-pinned image built from server/Dockerfile, confirm server listening port, provide secrets with an external secret manager, persistent backing PostgreSQL/Redis, appropriate readiness/liveness probes, network policies, ingress/TLS, autoscaling and rollback procedure. Do not `kubectl apply` without matching container runtime requirements and validating non-root compatibility.
