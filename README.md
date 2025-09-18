# talup

## TODO

- [ ] Implement Kubernetes Upgrade Controller
- [ ] Fix up and Test Talos Upgrade Controller
- [ ] Fix up and Test Helm Chart
- [ ] Write a better README
- [ ] Implement Metrics
- [ ] Fix up Workflows
- [ ] Determine best way for single node clusters to use this

## Example TalosPlan

```yaml
---
apiVersion: upgrade.home-operations.com/v1alpha1
kind: TalosPlan
metadata:
  name: talos
  namespace: system-upgrade
spec:
  force: false
  image:
    repository: factory.talos.dev/metal-installer/05b4a47a70bc97786ed83d200567dcc8a13f731b164537ba59d5397d668851fa
    tag: v1.11.1
  talosctl:
<<<<<<< HEAD
    image:
      repository: ghcr.io/siderolabs/talosctl
      tag: v1.11.1
      pullPolicy: IfNotPresent
=======
    repository: ghcr.io/siderolabs/talosctl
    tag: v1.11.1
    pullPolicy: IfNotPresent
>>>>>>> b9ae2b1 (chore: updates)
  rebootMode: powercycle

```

## Roles nto included in chart yet

```yaml
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: talup
  namespace: system-upgrade
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: cluster-admin
subjects:
  - kind: ServiceAccount
    name: talup
    namespace: system-upgrade
---
apiVersion: talos.dev/v1alpha1
kind: ServiceAccount
metadata:
  name: talup
  namespace: system-upgrade
spec:
  roles: ["os:admin"]
```

## Test with Helm

```yaml
---
# helm install talup oci://ghcr.io/home-operations/charts/talup --version 0.0.0 --values values.yaml --namespace=system-upgrade
# helm uninstall talup --namespace=system-upgrade
# kubectl delete crd kubernetesplans.upgrade.home-operations.com talosplans.upgrade.home-operations.com
image:
  repository: ghcr.io/home-operations/talup
  tag: main-6a0c6de
talosServiceAccount:
  secretName: talup
```
