# talup

## TODO

- [ ] Implement Kubernetes Upgrade Controller
- [ ] Fix up and Test Talos Upgrade Controller
- [ ] Fix up and Test Helm Chart
- [ ] Write a better README
- [ ] Implement Metrics
- [ ] Fix up Workflows
- [ ] Determine best way for **single node clusters** to use this (maybe eschew?)
- [ ] Tests (unit/e2e)

## Testing

_To start off it might be wise to pause SUC._

1. Create `values.yaml`:

  ```yaml
  image:
    repository: ghcr.io/home-operations/talup
    tag: main-443f170 # grab most recent from packages
  ```

1. Install Helm Chart

  ```sh
  helm install talup oci://ghcr.io/home-operations/talup/charts/talup --version 0.0.0 --values values.yaml --namespace=system-upgrade
  ```

1. Observe the rollout, logs of the controller

1. Apply the `TalosPlan` CR _(make sure the schematic and versions match you **current** state)_

```yaml
---
apiVersion: talup.home-operations.com/v1alpha1
kind: TalosPlan
metadata:
  name: cluster
  namespace: system-upgrade
spec:
  force: false
  image:
    repository: factory.talos.dev/metal-installer/05b4a47a70bc97786ed83d200567dcc8a13f731b164537ba59d5397d668851fa
    tag: v1.11.1
  nodeSelector: {}
    # kubernetes.io/hostname: k8s-0
  rebootMode: default # or; powercycle
  talosctl:
    image:
      repository: ghcr.io/siderolabs/talosctl
      tag: v1.11.1
      pullPolicy: IfNotPresent
```

1. Check the status field of the CR, it should say all nodes are upgraded.

```sh
kubectl get talosplans.talup.home-operations.com -n system-upgrade cluster -oyaml
```

1. Okay, so it recognized the state of the cluster (hopefully). Let's downgrade it to test.

1. Change the `TalosPlan` CR and downgrade to the previous patch release (v1.11.0) and apply it

1. Observe the following while the nodes are being downgraded.

- `watch kubectl get talosplans.talup.home-operations.com -n system-upgrade cluster -oyaml`
- `watch kubectl get job -n system-upgrade`
- `watch kubectl get po -n system-upgrade`
- `stern -n system-upgrade talup-talos-cluster`

1. Report findings

1. Once all nodes are downgraded, apply the plan again to upgrade them to their previous version (v1.11.1)

1. Cleanup

- `helm uninstall talup --namespace=system-upgrade`
- `kubectl delete crd kubernetesplans.talup.home-operations.com talosplans.talup.home-operations.com`
