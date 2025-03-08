go version 1.23

### Installation


go get github.com/onsi/ginkgo/v2
go get github.com/onsi/gomega
go get k8s.io/client-go@v0.28.0
go get k8s.io/api@v0.28.0
go get k8s.io/apimachinery@v0.28.0


### Make sure the nodes are in seperate regions

kubectl get nodes -o custom-columns='NAME:.metadata.name,ZONE:.metadata.labels.topology\.kubernetes\.io/zone'

### run tests

ginkgo run ./...
Running Suite: Example Suite - /home/ak/playground/go_playground/cluster-tester
===============================================================================
Random Seed: 1741412346

Will run 1 of 1 specs
•

Ran 1 of 1 Specs in 0.000 seconds
SUCCESS! -- 1 Passed | 0 Failed | 0 Pending | 0 Skipped
PASS

Ginkgo ran 1 suite in 2.176832496s
Test Suite Passed