go version 1.24

### Installation
```bash
go get ./...
go mod tidy
go install github.com/onsi/ginkgo/v2/ginkgo@latest
```

### Make sure the nodes are in seperate regions
```bash
kubectl get nodes -o custom-columns='NAME:.metadata.name,ZONE:.metadata.labels.topology\.kubernetes\.io/zone'
```

### run tests
```bash
ginkgo -v --focus "Topology E2E test" 
```