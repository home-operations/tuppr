# Upgrade coordination

tuppr runs **one upgrade at a time across the whole cluster**. This is the
safety property that lets you apply many upgrade resources at once and trust
that they won't fight each other.

|                    | `TalosUpgrade`                                  | `KubernetesUpgrade`      |
| ------------------ | ----------------------------------------------- | ------------------------ |
| Scope              | Talos on selected nodes                         | Kubernetes cluster-wide  |
| Resources allowed  | Many (queued, first-come-first-served)          | Exactly one              |
| Within a plan      | Sequential, or parallel via `spec.parallelism`  | Single controller node   |
| Reboot             | Yes                                             | No                       |
| Health checks      | Before each node/batch                          | Before the upgrade       |
| Blocked by         | Any other in-progress upgrade                   | Any other in-progress upgrade |

## Multiple TalosUpgrade plans queue

You may create several `TalosUpgrade` resources targeting different node groups
(for example one per zone). Only **one plan executes at a time**, first-come,
first-served; the rest stay `Pending` until the active plan finishes. Within a
single plan, use [`spec.parallelism`](talos-upgrades.md#parallel-upgrades) to
upgrade several nodes concurrently.

```yaml
apiVersion: tuppr.home-operations.com/v1alpha1
kind: TalosUpgrade
metadata:
  name: workers-west
spec:
  talos:
    version: v1.13.7
  nodeSelector:
    matchLabels:
      topology.kubernetes.io/zone: west
---
apiVersion: tuppr.home-operations.com/v1alpha1
kind: TalosUpgrade
metadata:
  name: workers-east
spec:
  talos:
    version: v1.13.7
  nodeSelector:
    matchLabels:
      topology.kubernetes.io/zone: east
```

/// warning | Overlapping node selectors
If two plans select the same node, the admission webhook issues a warning at
creation. It is allowed but discouraged: alternating plans can repeatedly
upgrade the same node.
///

## Only one KubernetesUpgrade

A Kubernetes upgrade affects the entire cluster, so tuppr allows **exactly one**
`KubernetesUpgrade` resource - the admission webhook rejects a second. To
upgrade again, edit `spec.kubernetes.version` on the existing resource.

## The two kinds never run concurrently

A `TalosUpgrade` and a `KubernetesUpgrade` cannot run at the same time -
changing Talos on nodes while changing the Kubernetes version could destabilize
the cluster. Whichever reaches `InProgress` first runs to completion; the other
waits in `Pending` with a message like:

> Waiting for Talos upgrade 'cluster' to complete before starting Kubernetes upgrade

and starts **automatically** once the active upgrade reaches `Completed`. If you
only ever apply one kind, it starts immediately with no blocking.
