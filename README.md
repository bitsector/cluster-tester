go version 1.21

go get github.com/onsi/ginkgo/v2
go get github.com/onsi/gomega
go get k8s.io/client-go@v0.28.0
go get k8s.io/api@v0.28.0
go get k8s.io/apimachinery@v0.28.0


Make sure the nodes are in seperate regions

kubectl get nodes -o custom-columns='NAME:.metadata.name,ZONE:.metadata.labels.topology\.kubernetes\.io/zone'

