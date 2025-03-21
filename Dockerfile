
FROM golang:1.24.1-bullseye AS builder

WORKDIR /app

RUN mkdir -p /app/temp && \
    chown -R 65534:65534 /app/temp && \
    chmod 755 /app/temp

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

# Create .env file with required content - we want to prevent copying it
RUN printf "KUBECONFIG=/path/to/.kube/config\nACCESS_MODE=LOCAL_K8S_API\n" > .env

# Allos non root user 65534 access all thefiles
RUN chown -R 65534:65534 . && \
    chmod -R 755 . && \
    find . -type f -exec chmod 644 {} \;


# Build binary (assuming you have setup.go/util.go as well)
# Build binary (compile all .go files explicitly)
RUN CGO_ENABLED=0 GOOS=linux go test -c -o cluster-tester \
    ./setup.go \
    ./util.go \
    ./affinity_deployment_test.go \
    ./anti_affinity_deployment_test.go \
    ./topology_constraint_deployment_test.go \
    ./rolling_update_deployment_test.go \
    ./pdb_deployment_test.go \ 
    ./affinity_statefulset_test.go \
    ./anti_affinity_statefulset_test.go \
    ./topology_constraint_statefulset_test.go \ 
    ./pdb_sts_test.go \
    ./rolling_update_sts_test.go 
    
FROM gcr.io/distroless/static-debian11:debug 

# Copy binary and manifests from builder stage explicitly
COPY --from=builder /app/cluster-tester /app/
COPY --from=builder /app/.env /app/
COPY --from=builder --chown=65534:65534 /app/temp /app/temp

# Explicitly copy each *test_yamls directory separately into container /app/ dir
COPY --from=builder /app/affinity_test_deployment_yamls /app/affinity_test_deployment_yamls
COPY --from=builder /app/affinity_test_statefulset_yamls /app/affinity_test_statefulset_yamls
COPY --from=builder /app/anti_affinity_statefulset_test_yamls /app/anti_affinity_statefulset_test_yamls
COPY --from=builder /app/anti_affinity_test_deployment_yamls /app/anti_affinity_test_deployment_yamls
COPY --from=builder /app/pdb_deployment_test_yamls /app/pdb_deployment_test_yamls
COPY --from=builder /app/pdb_statefulset_test_yamls /app/pdb_statefulset_test_yamls
COPY --from=builder /app/rolling_update_deployment_test_yamls /app/rolling_update_deployment_test_yamls
COPY --from=builder /app/rolling_update_sts_yamls /app/rolling_update_sts_yamls
COPY --from=builder /app/topology_test_deployment_yamls /app/topology_test_deployment_yamls
COPY --from=builder /app/topology_test_statefulset_yamls /app/topology_test_statefulset_yamls

WORKDIR /app

USER 65534:65534

ENTRYPOINT ["sh", "-c", "./cluster-tester -test.v; sleep 19800"]