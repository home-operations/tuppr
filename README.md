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
apiVersion: talup.home-operations.com/v1alpha1
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
    image:
      repository: ghcr.io/siderolabs/talosctl
      tag: v1.11.1
      pullPolicy: IfNotPresent
  rebootMode: powercycle

```

## Test with Helm

```yaml
---
# helm install talup oci://ghcr.io/home-operations/talup/charts/talup --version 0.0.0 --values values.yaml --namespace=system-upgrade
# helm uninstall talup --namespace=system-upgrade
# kubectl delete crd kubernetesplans.talup.home-operations.com talosplans.talup.home-operations.com
image:
  repository: ghcr.io/home-operations/talup
  tag: main-xxxxxx
```
