# talup

## TODO

- [ ] Implement Kubernetes Upgrade Controller
- [ ] Fix up and Test Talos Upgrade Controller
- [ ] Fix up and Test Helm Chart
- [ ] Write a better README
- [ ] Implement Metrics
- [ ] Fix up Workflows

## Example TalosPlan

```yaml
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
  rebootMode: powercycle
```
