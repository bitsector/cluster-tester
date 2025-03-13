# 1. Create service account in kube-system (persistent namespace)
kubectl apply -n kube-system -f - <<EOF
apiVersion: v1
kind: ServiceAccount
metadata:
  name: e2e-test-sa
EOF

# 2. Create ClusterRole with required permissions (same as before)
kubectl apply -f - <<EOF
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: e2e-test-role
rules:
- apiGroups: [""]
  resources: ["pods", "pods/exec"]
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

# 3. Create ClusterRoleBinding pointing to kube-system SA
kubectl apply -f - <<EOF
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: e2e-test-binding
subjects:
- kind: ServiceAccount
  name: e2e-test-sa
  namespace: kube-system
roleRef:
  kind: ClusterRole
  name: e2e-test-role
  apiGroup: rbac.authorization.k8s.io
EOF

# 4. Create Secret in kube-system namespace
kubectl apply -n kube-system -f - <<EOF
apiVersion: v1
kind: Secret
metadata:
  name: e2e-test-token
  annotations:
    kubernetes.io/service-account.name: e2e-test-sa
type: kubernetes.io/service-account-token
EOF

# 5. Get Secret UID from kube-system
SECRET_UID=$(kubectl get secret e2e-test-token -n kube-system -o jsonpath='{.metadata.uid}')

# 6. Create token with proper binding (now using kube-system)
kubectl create token e2e-test-sa -n kube-system \
  --duration=1h \
  --bound-object-kind Secret \
  --bound-object-name e2e-test-token \
  --bound-object-uid $SECRET_UID \
  --audience=kubernetes.default.svc

# 7. Export to env variables (updated namespace):
export K8S_API_URL=$(kubectl config view --minify -o jsonpath='{.clusters[0].cluster.server}')
export K8S_TOKEN=$(kubectl create token e2e-test-sa -n kube-system --duration=1h)
export K8S_CA_CERT=$(kubectl get secret e2e-test-token -n kube-system -o jsonpath='{.data.ca\.crt}')

# 8. Cleanup commands (DON'T DELETE kube-system! Only cluster-scoped resources):
kubectl delete clusterrolebinding e2e-test-binding
kubectl delete clusterrole e2e-test-role
# Note: The SA and secret in kube-system will persist unless explicitly deleted

# 9. Test connection (using default namespace example):
curl -X GET "$K8S_API_URL/apis/apps/v1/namespaces/default/deployments" \
  -H "Authorization: Bearer $K8S_TOKEN" \
  --cacert <(echo "$K8S_CA_CERT" | base64 -d)