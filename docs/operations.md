# Operations

Day-to-day commands for watching and steering upgrades. The controller runs in
the `system-upgrade` namespace in these examples.

## Watch progress

```bash
kubectl get talosupgrade -w
kubectl get kubernetesupgrade -w

# Detailed status and events
kubectl describe talosupgrade cluster
kubectl describe kubernetesupgrade kubernetes

# Controller logs
kubectl logs -f deployment/tuppr -n system-upgrade

# Which nodes carry a version override
kubectl get nodes -o custom-columns=NAME:.metadata.name,VERSION-OVERRIDE:.metadata.annotations."tuppr\.home-operations\.com/version"
```

## Suspend and resume

Suspend a resource to upgrade manually without the controller interfering:

```bash
kubectl annotate talosupgrade cluster tuppr.home-operations.com/suspend="true"
kubectl annotate kubernetesupgrade kubernetes tuppr.home-operations.com/suspend="true"

# Resume (remove the annotation)
kubectl annotate talosupgrade cluster tuppr.home-operations.com/suspend-
kubectl annotate kubernetesupgrade kubernetes tuppr.home-operations.com/suspend-
```

## Retry a failed upgrade

`Failed` is terminal - the controller stops reconciling until you act. Three
ways to retry:

```bash
# Reset: wipe runtime state (phase, completed/failed nodes, hook progress),
# keep the spec, restart from scratch.
kubectl annotate talosupgrade cluster tuppr.home-operations.com/reset="$(date)"

# Spec edit: any change to .spec bumps generation and restarts.
kubectl edit talosupgrade cluster

# Delete + recreate: loses history. Only if the CR itself is corrupt.
kubectl delete talosupgrade cluster && kubectl apply -f talos-upgrade.yaml
```

/// warning | Node never converges
If a run keeps reaching `Completed` but a node never catches up to the target
version, tuppr marks it `Failed` after 5 completion cycles
(_"Node(s) never converged to … after 5 completion cycles"_). Investigate the
lagging node before retrying.
///

## Troubleshoot

```bash
# Upgrade Job logs
kubectl logs job/tuppr-xyz -n system-upgrade

# Controller pod health
kubectl get pods -n system-upgrade -l app.kubernetes.io/name=tuppr

# Phase / duration metrics
kubectl port-forward -n system-upgrade deployment/tuppr 8081:8081
curl -s http://localhost:8081/metrics | grep -E 'tuppr_.*_(phase|duration)'
```

## Emergency stop

```bash
# Pause everything (scale the controller to zero)
kubectl scale deployment tuppr --replicas=0 -n system-upgrade

# Clean up upgrades and their Jobs
kubectl delete talosupgrade --all
kubectl delete kubernetesupgrade --all
kubectl delete jobs -l app.kubernetes.io/name=talos-upgrade -n system-upgrade
kubectl delete jobs -l app.kubernetes.io/name=kubernetes-upgrade -n system-upgrade

# Resume
kubectl scale deployment tuppr --replicas=1 -n system-upgrade
```
