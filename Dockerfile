
FROM golang:1.24.1-bullseye AS builder

WORKDIR /app

# Copy go module files first (for caching)
COPY go.mod .
COPY go.sum .

# Install dependencies explicitly
RUN go mod download
RUN go get github.com/joho/godotenv

# Copy all Go files at root
COPY *.go ./

# Copy .env file explicitly
COPY .env .

# Explicitly copy all *test_yamls directories and their contents
COPY affinity_test_deployment_yamls ./affinity_test_deployment_yamls
COPY affinity_test_statefulset_yamls ./affinity_test_statefulset_yamls
COPY anti_affinity_statefulset_test_yamls ./anti_affinity_statefulset_test_yamls
COPY anti_affinity_test_deployment_yamls ./anti_affinity_test_deployment_yamls
COPY pdb_deployment_test_yamls ./pdb_deployment_test_yamls
COPY pdb_statefulset_test_yamls ./pdb_statefulset_test_yamls
COPY rolling_update_deployment_test_yamls ./rolling_update_deployment_test_yamls
COPY rolling_update_sts_yamls ./rolling_update_sts_yamls
COPY topology_test_deployment_yamls ./topology_test_deployment_yamls
COPY topology_test_statefulset_yamls ./topology_test_statefulset_yamls

# Copy Go source files explicitly from root directory
COPY *.go ./

# Copy .env file explicitly
COPY .env ./

# Build binary (assuming you have setup.go/util.go as well)
# Build binary (compile all .go files explicitly)
RUN CGO_ENABLED=0 GOOS=linux go test -c -o cluster-tester \
    ./affinity_deployment_test.go \
    ./affinity_statefulset_test.go \
    ./anti_affinity_deployment_test.go \
    ./anti_affinity_statefulset_test.go \
    ./pdb_deployment_test.go \
    ./pdb_sts_test.go \
    ./rolling_update_deployment_test.go \
    ./rolling_update_sts_test.go \
    ./setup.go \
    ./simple_connectivity_test.go \
    ./topology_constraint_deployment_test.go \
    ./topology_constraint_statefulset_test.go \
    ./util.go
    
FROM gcr.io/distroless/static-debian11

# Copy binary and manifests from builder stage explicitly
COPY --from=builder /app/cluster-tester /
COPY --from=builder /app/.env /

# Explicitly copy each *test_yamls directory separately into container
COPY --from=builder /app/affinity_test_deployment_yamls /test-manifests/affinity_test_deployment_yamls
COPY --from=builder /app/affinity_test_statefulset_yamls /test-manifests/affinity_test_statefulset_yamls
COPY --from=builder /app/anti_affinity_statefulset_test_yamls /test-manifests/anti_affinity_statefulset_test_yamls
COPY --from=builder /app/anti_affinity_test_deployment_yamls /test-manifests/anti_affinity_test_deployment_yamls
COPY --from=builder /app/pdb_deployment_test_yamls /test-manifests/pdb_deployment_test_yamls
COPY --from=builder /app/pdb_statefulset_test_yamls /test-manifests/pdb_statefulset_test_yamls
COPY --from=builder /app/rolling_update_deployment_test_yamls /test-manifests/rolling_update_deployment_test_yamls
COPY --from=builder /app/rolling_update_sts_yamls /test-manifests/rolling_update_sts_yamls
COPY --from=builder /app/topology_test_deployment_yamls /test-manifests/topology_test_deployment_yamls
COPY --from=builder /app/topology_test_statefulset_yamls /test-manifests/topology_test_statefulset_yamls

USER 65534:65534

ENTRYPOINT ["/cluster-tester", "-test.v", "-ginkgo.focus", "Deployment Topology E2E test"]