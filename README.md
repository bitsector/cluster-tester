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

### Run tests

### Simple connectivity test (make sure you connect to the cluster):
```bash
go test -v ./simple_connectivity_test.go -ginkgo.focus "Basic cluster connectivity test"
```

### Deployment tests
```bash
go test -v ./topology_constraint_deployment_test.go -ginkgo.focus "Deployment Topology E2E test"
go test -v ./affinity_deployment_test.go -ginkgo.focus "Deployment Affinity Test Suite"
go test -v ./anti_affinity_deployment_test.go -ginkgo.focus "Deployment Anti Affinity Test Suite"
go test -v ./pdb_deployment_test.go  -ginkgo.focus "Deployment PDB E2E test"
go test -v ./rolling_update_deployment_test.go -ginkgo.focus "Deployment Rolling Update E2E test"
```
### StatefulSet tests
```bash
go test -v ./affinity_statefulset_test.go -ginkgo.focus "StatefulSet Affinity Test Suite"
go test -v ./anti_affinity_statefulset_test.go -ginkgo.focus "StatefulSet Anti Affinity E2E test"
go test -v ./topology_constraint_statefulset_test.go -ginkgo.focus "StatefulSet Topology E2E test"
go test -v ./pdb_sts_test.go  -ginkgo.focus "StatefulSet PDB E2E test"
go test -v ./rolling_update_sts_test.go -ginkgo.focus "StatefulSet Rolling Update E2E test"
```