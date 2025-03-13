# 1. Create namespace (if not exists)
kubectl create namespace test-ns --dry-run=client -o yaml | kubectl apply -f -

# 2. Create service account
kubectl apply -n test-ns -f - <<EOF
apiVersion: v1
kind: ServiceAccount
metadata:
  name: e2e-test-sa
EOF

# 3. Create ClusterRole with required permissions
kubectl apply -f - <<EOF
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: e2e-test-role
rules:
- apiGroups: [""]
  resources: ["pods", "pods/exec"]  # Add these
  verbs: ["*"]
- apiGroups: [""]
  resources: ["namespaces", "persistentvolumes"]
  verbs: ["*"]
- apiGroups: ["apps"]
  resources: ["deployments", "statefulsets"]
  verbs: ["*"]
- apiGroups: ["policy"]
  resources: ["poddisruptionbudgets"]
  verbs: ["*"]
- apiGroups: ["autoscaling"]
  resources: ["horizontalpodautoscalers"]
  verbs: ["*"]
EOF

# 4. Create ClusterRoleBinding
kubectl apply -f - <<EOF
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: e2e-test-binding
subjects:
- kind: ServiceAccount
  name: e2e-test-sa
  namespace: test-ns
roleRef:
  kind: ClusterRole
  name: e2e-test-role
  apiGroup: rbac.authorization.k8s.io
EOF

# 5. Create Secret first to get UID
kubectl apply -n test-ns -f - <<EOF
apiVersion: v1
kind: Secret
metadata:
  name: e2e-test-token
  annotations:
    kubernetes.io/service-account.name: e2e-test-sa
type: kubernetes.io/service-account-token
EOF

# 6. Get Secret UID
SECRET_UID=$(kubectl get secret e2e-test-token -n test-ns -o jsonpath='{.metadata.uid}')

# 7. Create token with proper binding
kubectl create token e2e-test-sa -n test-ns \
  --duration=1h \
  --bound-object-kind Secret \
  --bound-object-name e2e-test-token \
  --bound-object-uid $SECRET_UID \
  --audience=kubernetes.default.svc  # Explicit audience

# 8. Export to env variables:

# Get API Server URL
export K8S_API_URL=$(kubectl config view --minify -o jsonpath='{.clusters[0].cluster.server}')

# Get Token (short-lived)
export K8S_TOKEN=$(kubectl create token e2e-test-sa -n test-ns --duration=1h)

# Get CA Cert (formatted for environment variable)
export K8S_CA_CERT=$(kubectl get secret e2e-test-token -n test-ns -o jsonpath='{.data.ca\.crt}')

# 9. Delete all elementes from the cluster/project

# 9.1. Delete ClusterRoleBinding (cluster-scoped)
kubectl delete clusterrolebinding e2e-test-binding

# 9.2. Delete ClusterRole (cluster-scoped)
kubectl delete clusterrole e2e-test-role

# 9.3. Delete namespace and all contained resources (service account, secret)
kubectl delete namespace test-ns


# 10. Test connection:

### 10.1 This should return a yaml with all deployments running in test-ns namespace
curl -X GET "$K8S_API_URL/apis/apps/v1/namespaces/test-ns/deployments" \
  -H "Authorization: Bearer $K8S_TOKEN" \
  --cacert <(echo "$K8S_CA_CERT" | base64 -d)

