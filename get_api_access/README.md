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
  --duration=8760h \
  --bound-object-kind Secret \
  --bound-object-name e2e-test-token \
  --bound-object-uid $SECRET_UID