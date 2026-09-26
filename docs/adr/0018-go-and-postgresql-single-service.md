# Go and PostgreSQL single service

V1 runs as one Go binary with embedded dashboard assets and one dedicated PostgreSQL database. A bounded database pool handles normal work, one dedicated connection receives wakeups, and PostgreSQL stores leased durable work; Redis, a message broker, Kubernetes, and separate worker processes are outside V1.
