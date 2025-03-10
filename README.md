go version 1.24

### Installation
```bash
go get ./...
go mod tidy
go install github.com/onsi/ginkgo/v2/ginkgo@latest
go get github.com/joho/godotenv
```
### Set the path to your local kube config in .env file
```bash
KUBECONFIG=/path/to/.kube/config
```

### Make sure the nodes are in seperate regions
```bash
kubectl get nodes -o custom-columns='NAME:.metadata.name,ZONE:.metadata.labels.topology\.kubernetes\.io/zone'
```

### run tests
```bash
go test -v ./simple_connectivity_test.go -ginkgo.focus "Basic cluster connectivity test"
go test -v ./topology_constraint_deployment_test.go -ginkgo.focus "Topology E2E test"
go test -v ./affinity_deployment_test.go -ginkgo.focus "Affinity E2E test"
go test -v ./anti_affinity_deployment_test.go -ginkgo.focus "Anti Affinity E2E test"
go test -v ./pdb_deployment_test.go  -ginkgo.focus "PDB E2E test"
go test -v ./rolling_update_deployment_test.go -ginkgo.focus "Rolling Update E2E test"
```