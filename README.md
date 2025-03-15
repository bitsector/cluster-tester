# Golang Ginkgo E2E cluster-tester

### Go version
go version 1.24.1


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
ACCESS_MODE=KUBECONFIG, LOCAL_K8S_API or EXTERNAL_K8S_API
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

## Documentation - The test cases and how they work:



### Connectivity Test
A basic connectivity test. Will attempt to connect to the cluster, list nodes, create a namespace and finish.
simple_connectivity_test.go

### Deployment Topology Constraints E2E test
This test will deploy a HPA and a deployment with a topologySpreadConstraints in its manifests. 
The Deployment pods will trigger high CPU simulation, this will trigger the HPA, the HPA will trigger the cluster to create more pods.
Once more pods are created the test code will collect data on all the pods and their zones of schedule, verifying that the 
topologySpreadConstraints condition is met. The test will fail if and only if the condition is not met.
File:
topology_constraint_deployment_test.go

### Deployment PDB E2E test
The test will deploy a PDB, an HPA and a Deployment. The 2 sub-tests will be attempted:
1. The test code will attempt a rolling update on the deployment - since the deployment has no limitation on unavailable pods 
(maxUnavailable and maxSurge 6) - all pods will be deleted. If the PDB works it will keep a minimum of 5 running pods. Otherwise the
number of running pods will drop to 0 momentarily. The test will sample the number of pods during this rolling update period. If 
at no point there were less than 5 running pods - this sub test has passed, as it indicates the PDB has worked. Otherwist it will fail.
2. The test code will attempt to delete all the deployment's pods individually (i.e not deleting the deployment itself). If the PDB 
is working there still must be at least 5 running pods despite of the deletion. The test will sample the number of running pods right
after the deletion. If at no point there were less than 5 running pods - the test will pass, otherwise the test will fail. 
Both subtests must pass in order for the PDB test to pass. 
Note: As of this writing PDB tests always fail, we have not yet discovered a reproducible case where PDB was applied and actually worked. 
File: pdb_deployment_test.go

### Deployment Affinity E2E test
The test will deploy a zone-marker pod (placed a random zone by K8s), deploy an HPA, and a dependent-app deployment with a pod affinity 
requirement (requiredDuringSchedulingIgnoredDuringExecution). The goal of the test is to trigger the deployment to create more pods and
assert that all these pods satisfy the affinity requirement, relative to the zone-marker pod. The deployment's first pod will start running,
simulate high CPU demand, this will trigger the HPA to create more of the deployment's pods. The test code will then verify that all 
the pods are placed in the same zone as the zone-marker pod. The test will fail if and only if this condition is not met.  
File: affinity_deployment_test.go

### Deployment Anti Affinity E2E test
The test will deploy a zone-marker pod (placed a random zone by K8s), deploy an HPA, and a dependent-app deployment with a pod anti affinity 
requirement (podAntiAffinity). The goal of the test is to trigger the deployment to create more pods and
assert that all these pods satisfy the anti affinity requirement, relative to the zone-marker pod. The deployment's first pod will start running,
simulate high CPU demand, this will trigger the HPA to create more of the deployment's pods. The test code will then verify that all 
the pods are placed outside the zone of the zone-marker pod. The test will fail if and only if this condition is not met.  
File: anti_affinity_deployment_test.go

### Deployment Rolling Update E2E test
The test will deploy a deployment with a RollingUpdate strategy. Once the deployment is up and running, the test code will initiate a rolling
update (it will change the CPU of the container from 50m to 100m). During the update, the test code will sample repeatedly the state of the pods
making sure they are in the confines of maxSurge: 1 and maxUnavailable: 25% values. If at no point the deployment pods' status violate the
rolling update's strategy - the test will pass.
File: rolling_update_deployment_test.go

### StatefulSet PDB E2E test
pdb_sts_test.go
### StatefulSet Affinity E2E test
affinity_statefulset_test.go
### StatefulSet Rolling Update E2E test
rolling_update_sts_test.go:
### StatefulSet Anti Affinity E2E test
anti_affinity_statefulset_test.go
### StatefulSet Topology Constraints E2E test
topology_constraint_statefulset_test.go
