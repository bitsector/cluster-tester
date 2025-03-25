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
ALLOWED_TO_FAIL=StatefulSetPDBTest,DeploymentPDBTest # all tags are listed in .env
```

### Make sure the nodes are in seperate regions
```bash
kubectl get nodes -o custom-columns='NAME:.metadata.name,ZONE:.metadata.labels.topology\.kubernetes\.io/zone'
```

### Run tests

### Simple connectivity test (make sure you connect to the cluster):
```bash
go test -v -ginkgo.label-filter=safe-in-production -ginkgo.focus="Basic cluster connectivity test" ./...

```

### Deployment tests
```bash
go test -v -ginkgo.label-filter=safe-in-production -ginkgo.focus="Deployment Topology Constraints E2E test" ./...
go test -v -ginkgo.label-filter=safe-in-production -ginkgo.focus="Deployment Affinity E2E test" ./...
go test -v -ginkgo.label-filter=safe-in-production -ginkgo.focus="Deployment Anti Affinity E2E test" ./...
go test -v -ginkgo.label-filter=safe-in-production -ginkgo.focus="Deployment PDB E2E test" ./...
go test -v -ginkgo.label-filter=safe-in-production -ginkgo.focus="Deployment Rolling Update E2E test" ./...
```
### StatefulSet tests
```bash
go test -v -ginkgo.label-filter=safe-in-production -ginkgo.focus="StatefulSet Affinity E2E test" ./...
go test -v -ginkgo.label-filter=safe-in-production -ginkgo.focus="StatefulSet Anti Affinity E2E test" ./...
go test -v -ginkgo.label-filter=safe-in-production -ginkgo.focus="StatefulSet Topology Constraints E2E test" ./...
go test -v -ginkgo.label-filter=safe-in-production -ginkgo.focus="StatefulSet PDB E2E test" ./...
go test -v -ginkgo.label-filter=safe-in-production -ginkgo.focus="StatefulSet Rolling Update E2E test" ./...
```

## Cronjob and debug-pod - How to run it inside a K8s cluster:

```bash
docker build --platform=linux/amd64 --no-cache -t your-repo-name/image-name:tag .

docker push your-repo-name/image-name:tag
```
Then change the `image` element cronjob.yaml and debug-pod.yaml to `your-repo-name/image-name:tag`

Then
```bash
# apply the manifest
kubectl create ns e2e-admin-ns
kubectl apply -f cronjob.yaml

# Optional: run the job manually
kubectl create job e2e-cluster-tester-cronjob-manual-$(date +%s) \
  --from=cronjob/e2e-cluster-tester-cronjob \
  -n e2e-admin-ns

# get the pod running the tests
CRONJOB_POD_NAME=$(kubectl get pods -n e2e-admin-ns \
  --field-selector=status.phase=Running \
  --sort-by=.metadata.creationTimestamp \
  -o name | tail -1 | cut -d'/' -f2) && echo "e2e pod name: $CRONJOB_POD_NAME"

# get the logs from the pod
kubectl logs $CRONJOB_POD_NAME -n e2e-admin-ns --follow


# get json filname
JSON_LOGS_FILE_NAME=$(kubectl exec $CRONJOB_POD_NAME -n e2e-admin-ns -- ls /app/temp | tr -d '\r') && echo "json filname: $JSON_LOGS_FILE_NAME"

# print json file contents
kubectl exec $CRONJOB_POD_NAME -n e2e-admin-ns -- sh -c "cat \"/app/temp/${JSON_LOGS_FILE_NAME}\""

# download the json file from the pod to temp/ dir
kubectl cp -n e2e-admin-ns $CRONJOB_POD_NAME:/app/temp/$JSON_LOGS_FILE_NAME temp/$JSON_LOGS_FILE_NAME
```

## How to get logs json file manually:
```bash
# Get all the cronjob pods in the e2e-admin-ns namespace:
kubectl get pods -n e2e-admin-ns

# Output:
# NAME                                                 READY   STATUS    RESTARTS   AGE
# e2e-cluster-tester-cronjob-manual-1742878565-fbv4b   1/1     Running   0          4m25s
# e2e-cluster-tester-cronjob-manual-1742878569-b5j88   0/1     Pending   0          4m21s

# Find the name of the Running pod (there should be only one)
# In this case
# e2e-cluster-tester-cronjob-manual-1742878565-fbv4b   1/1     Running 

# Now get the log file name it's format is test_suite_log_<timestamp>.json
kubectl exec e2e-cluster-tester-cronjob-manual-1742878565-fbv4b -n e2e-admin-ns -- ls /app/temp | tr -d '\r'

# Result:
# test_suite_log_20250325-045612.json

# Downlaod the file's content:
kubectl exec e2e-cluster-tester-cronjob-manual-1742878565-fbv4b -n e2e-admin-ns -- sh -c "cat \"/app/temp/test_suite_log_20250325-045612.json\""

# Result:
# {
#  "test_timestamp": "03/25/2025 04:56:12",
#  "failing_tests": [],
#  "succeeding_tests": [],
#  "allowed_to_fail_tests": [],
#  "failed_but_not_allowed_to_fail": [],
#  "success_ratio": "42%",
#  "logs_by_tags": {<logs>}
# }

# Store the file contents into a local file:
kubectl exec e2e-cluster-tester-cronjob-manual-1742878565-fbv4b -n e2e-admin-ns -- sh -c "cat \"/app/temp/test_suite_log_20250325-045612.json\"" > /path/to/local/file.txt

```
## Or, alternatively, use the debug pod
```bash
kubectl create ns e2e-admin-ns
kubectl apply -f debug-pod.yaml

# ssh into the pod
kubectl exec -it cluster-tester-debug-pod -n e2e-admin-ns -- /busybox/sh

# Run all the tests in the debug pod by executing the binary
./cluster-tester
```

### Deployment tests (run in a debug-pod or or cronjob)
```bash
./cluster-tester --ginkgo.label-filter=safe-in-production --ginkgo.focus="Deployment Affinity E2E test" -test.v
./cluster-tester --ginkgo.label-filter=safe-in-production --ginkgo.focus="Deployment Anti Affinity E2E test" -test.v
./cluster-tester --ginkgo.label-filter=safe-in-production --ginkgo.focus="Deployment Topology Constraints E2E test" -test.v
./cluster-tester --ginkgo.label-filter=safe-in-production --ginkgo.focus="Deployment PDB E2E test" -test.v
./cluster-tester --ginkgo.label-filter=safe-in-production --ginkgo.focus="Deployment Rolling Update E2E test" -test.v
```
### StatefulSet tests (run in a debug-pod or or cronjob)
```bash
./cluster-tester --ginkgo.label-filter=safe-in-production --ginkgo.focus="StatefulSet Affinity E2E test" -test.v
./cluster-tester --ginkgo.label-filter=safe-in-production --ginkgo.focus="StatefulSet Anti Affinity E2E test" -test.v
./cluster-tester --ginkgo.label-filter=safe-in-production --ginkgo.focus="StatefulSet Topology Constraints E2E test" -test.v
./cluster-tester --ginkgo.label-filter=safe-in-production --ginkgo.focus="StatefulSet PDB E2E test" -test.v
./cluster-tester --ginkgo.label-filter=safe-in-production --ginkgo.focus="StatefulSet Rolling Update E2E test" -test.v
```

## Documentation - The test cases and how they work:

### Connectivity Test
A basic connectivity test. Will attempt to connect to the cluster, list nodes, create a namespace and finish.
Files: 
- simple_connectivity_test.go

### Deployment Topology Constraints E2E test
This test will deploy an HPA and a deployment with a topologySpreadConstraints in its manifests. 
The Deployment pods will trigger high CPU simulation, this will trigger the HPA, the HPA will trigger the cluster to create more pods.
Once more pods are created the test code will collect data on all the pods and their zones of schedule, verifying that the 
topologySpreadConstraints condition is met. The test will fail if and only if the condition is not met.
Files:
- topology_constraint_deployment_test.go
- topology_test_deployment_yamls/hpa-trigger.yaml 
- topology_test_deployment_yamls/topology-dep.yaml

### Deployment PDB E2E test
The test will deploy a PDB and a Deployment. The 2 sub-tests will be attempted:
1. The test code will attempt a rolling update on the deployment - since the deployment has no limitation on unavailable pods 
(maxUnavailable and maxSurge 6) - all pods will be deleted. If the PDB works it will keep a minimum of 5 running pods. Otherwise the
number of running pods will drop to 0 momentarily. The test will sample the number of pods during this rolling update period. If 
at no point there were less than 5 running pods - this sub test has passed, as it indicates the PDB has worked. Otherwist it will fail.
2. The test code will attempt to delete all the deployment's pods individually (i.e not deleting the deployment itself). If the PDB 
is working there still must be at least 5 running pods despite of the deletion. The test will sample the number of running pods right
after the deletion. If at no point there were less than 5 running pods - the test will pass, otherwise the test will fail. 
Both subtests must pass in order for the PDB test to pass. 
Note: As of this writing PDB tests always fail, we have not yet discovered a reproducible case where PDB was applied and actually worked. 
Files: 
- pdb_deployment_test.go
- pdb_deployment_test_yamls/deployment.yaml 
- pdb_deployment_test_yamls/pdb.yaml   

### Deployment Affinity E2E test
The test will deploy a zone-marker pod (placed in a random zone by K8s), deploy an HPA, and a dependent-app deployment with a pod affinity 
requirement (podAffinity). The goal of the test is to trigger the deployment to create more pods and
assert that all these pods satisfy the affinity requirement, relative to the zone-marker pod. The deployment's first pod will start running,
simulate high CPU demand, this will trigger the HPA to create more of the deployment's pods. The test code will then verify that all 
the pods are placed in the same zone as the zone-marker pod. The test will fail if and only if this condition is not met.  
Files:
- affinity_deployment_test.go
- affinity_test_deployment_yamls/zone-marker.yaml
- affinity_test_deployment_yamls/hpa-trigger.yaml
- affinity_test_deployment_yamls/affinity-dependent-app.yaml
 
### Deployment Anti Affinity E2E test
The test will deploy a zone-marker pod (placed a random zone by K8s), deploy an HPA, and a dependent-app deployment with a pod anti affinity 
requirement (podAntiAffinity). The goal of the test is to trigger the deployment to create more pods and
assert that all these pods satisfy the anti affinity requirement, relative to the zone-marker pod. The deployment's first pod will start running,
simulate high CPU demand, this will trigger the HPA to create more of the deployment's pods. The test code will then verify that all 
the pods are placed outside the zone of the zone-marker pod. The test will fail if and only if this condition is not met.  
Files: 
- anti_affinity_deployment_test.go
- anti_affinity_test_deployment_yamls/anti-affinity-dependent-app.yaml 
- anti_affinity_test_deployment_yamls/hpa-trigger.yaml 
- anti_affinity_test_deployment_yamls/zone-marker.yaml

### Deployment Rolling Update E2E test
The test will deploy a deployment with a RollingUpdate strategy. Once the deployment is up and running, the test code will initiate a rolling
update (it will change the CPU of the container from 50m to 100m). During the update, the test code will sample repeatedly the state of the pods
making sure they are in the confines of maxSurge: 1 and maxUnavailable: 25% values. If at no point the deployment pods' status violate the
rolling update's strategy - the test will pass.
Files: 
- rolling_update_deployment_test.go
- rolling_update_deployment_test_yamls/deployment_start.yaml 

### StatefulSet PDB E2E test
The test will deploy a PDB and a stateful set. The 2 sub-tests will be attempted:
1.The test code will attempt to delete all the stateful set's pods individually (i.e not deleting the stateful set itself). If the PDB 
is working there still must be at least 5 running pods despite the deletion. The test will sample the number of running pods right
after the deletion. If at no point there were less than 5 running pods - the test will pass, otherwise the test will fail. 
Both subtests must pass in order for the PDB test to pass. 
Note: As of this writing PDB tests always fail, we have not yet discovered a reproducible case where PDB was applied and actually worked. 
Files:
- pdb_sts_test.go
- pdb_statefulset_test_yamls/pdb.yaml 
- pdb_statefulset_test_yamls/sts.yaml

### StatefulSet Affinity E2E test
The test will deploy a zone-marker pod (placed a random zone by K8s), deploy an HPA, and a dependent-app stateful set with a pod affinity 
requirement (podAffinity). The goal of the test is to trigger the stateful set to create more pods and
assert that all these pods satisfy the affinity requirement, relative to the zone-marker pod. The stateful set's first pod will start running,
simulate high CPU demand, this will trigger the HPA to create more of the stateful set's pods. The test code will then verify that all 
the pods are placed in the same zone as the zone-marker pod. The test will fail if and only if this condition is not met.  
Files: 
- affinity_statefulset_test.go
- affinity_test_statefulset_yamls/zone-marker.yaml
- affinity_test_statefulset_yamls/hpa-trigger.yaml 
- affinity_test_statefulset_yamls/affinity-dependent-app.yaml    

### StatefulSet Rolling Update E2E test
The test will deploy a stateful set with a rolling update strategy (updateStrategy). Once the stateful set is up and running, the test code 
will initiate a rolling update (it will change the CPU of the container from 50m to 100m). During the update, the test code will sample
repeatedly the state of the pods making sure there is at most one unavailable pod in any time. If at no point the stateful set pods' status 
violate this condition - the test will pass.
Files: 
- rolling_update_sts_test.go
- rolling_update_sts_yamls/sts_start.yaml

### StatefulSet Anti Affinity E2E test
The test will deploy a zone-marker pod (placed a random zone by K8s), deploy an HPA, and a dependent-app stateful set with a pod anti affinity 
requirement (podAntiAffinity). The goal of the test is to trigger the stateful set to create more pods and
assert that all these pods satisfy the anti affinity requirement, relative to the zone-marker pod. The stateful set's first pod will start running,
simulate high CPU demand, this will trigger the HPA to create more of the stateful set's pods. The test code will then verify that all 
the pods are placed in any zone different from the zone-marker's pod zone. The test will fail if and only if this condition is not met.  
Files: 
- anti_affinity_statefulset_test.go
- anti_affinity_statefulset_test_yamls/zone-marker.yaml
- anti_affinity_statefulset_test_yamls/anti-affinity-dependent-app.yaml 
- anti_affinity_statefulset_test_yamls/hpa-trigger.yaml

### StatefulSet Topology Constraints E2E test
This test will deploy a HPA and a stateful set with a topologySpreadConstraints in its manifests. 
The stateful set pods will trigger high CPU simulation, this will trigger the HPA, the HPA will trigger the cluster to create more pods.
Once more pods are created the test code will collect data on all the pods and their zones of schedule, verifying that the 
topologySpreadConstraints condition is met. The test will fail if and only if the condition is not met.
Files: 
- topology_constraint_statefulset_test.go
- topology_test_statefulset_yamls/hpa-trigger.yaml
- topology_test_statefulset_yamls/topology-statefulset.yaml

### PDB Testing Observations:
We have never observed a Pod Disruption Budget (PDB) being successfully applied and functioning as expected. Several attempts were made to demonstrate a functional PDB configuration without success (tested on GKE Kubernetes v1.31).

**Attempt 1: Deployment/StatefulSet with Replica Guarantee**
- Deployed a StatefulSet with 6 replicas
- Applied PDB requiring minimum 5 available pods
- Manually deleted all pods individually (`kubectl delete` and programmatic deletion)
- **Expected**: PDB should maintain ≥5 available pods
- **Actual**: All pods were deleted with temporary zero availability
- _Implementation_: See Go code in this repository

**Attempt 2: Rolling Update Without Availability Limits**
- Configured deployment rolling update with no `maxUnavailable` restriction
- Applied PDB requiring minimum 5 available pods during updates
- **Expected**: PDB would prevent total pod unavailability
- **Actual**: Immediate deletion of all old pods resulted in temporary zero availability
- _Implementation_: See Go code in this repository

**Attempt 3: HPA Scaling vs PDB Minimum**
- Initial deployment: 3 replicas
- HPA configured to scale down when CPU < 70%
- PDB requiring ≥2 available pods
- **Expected**: HPA would be blocked from scaling below 2 pods
- **Actual**: HPA successfully scaled to 1 pod despite PDB
- _Conditions_: Tested with idle pods (CPU utilization < 70%)

**Attempt 4: External Reproducibility Test**
- Cloned and executed [k8s-pdb-demo](https://github.com/phenixblue/k8s-pdb-demo)
- **Outcome**: Observed same PDB failure patterns as our tests