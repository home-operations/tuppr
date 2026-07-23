# Talos upgrades

A `TalosUpgrade` declares a target Talos version for a set of nodes. tuppr
health-checks the cluster, then rolls the version out node-by-node (or in
parallel batches), draining and rebooting each node and verifying it returns
`Ready` on the new version before moving on.

The only required field is the target version:

```yaml
apiVersion: tuppr.home-operations.com/v1alpha1
kind: TalosUpgrade
metadata:
  name: cluster
spec:
  talos:
    # renovate: datasource=docker depName=ghcr.io/siderolabs/installer
    version: v1.13.7
```

Everything below is optional and layers onto that. The full set of fields and
defaults lives in the CRD schema; this page covers the ones you'll reach for.

## Health checks

Health checks are [CEL](https://cel.dev/) expressions evaluated against live
cluster objects before an upgrade proceeds. They run concurrently, and (for a
`TalosUpgrade`) before each node or batch.

```yaml
spec:
  healthChecks:
    # All nodes Ready
    - apiVersion: v1
      kind: Node
      expr: status.conditions.filter(c, c.type == "Ready").all(c, c.status == "True")
      timeout: 10m

    # A named Deployment fully rolled out
    - apiVersion: apps/v1
      kind: Deployment
      name: critical-app
      namespace: production
      expr: status.readyReplicas == status.replicas

    # Objects selected by labels
    - apiVersion: apps/v1
      kind: Deployment
      namespace: production
      labelSelector:
        matchLabels:
          app.kubernetes.io/part-of: critical-platform
      expr: status.readyReplicas == status.replicas

    # A custom resource
    - apiVersion: ceph.rook.io/v1
      kind: CephCluster
      name: rook-ceph
      namespace: rook-ceph
      expr: status.ceph.health in ["HEALTH_OK"]
```

Health checks are also available on [`KubernetesUpgrade`](kubernetes-upgrades.md).

## Policy

`spec.policy` tunes how the upgrade runs:

| Field                 | Default   | Effect                                                                             |
| --------------------- | --------- | ---------------------------------------------------------------------------------- |
| `debug`               | `false`   | Verbose upgrade logging.                                                            |
| `force`               | `false`   | Skip etcd health checks. Dangerous - can break quorum.                             |
| `placement`           | `hard`    | How strictly the upgrade Job avoids the target node (`hard` or `soft`).            |
| `rebootMode`          | `default` | `default`, or `powercycle` for nodes that don't reboot cleanly.                    |
| `stage`               | `false`   | Stage the upgrade and apply it on the next reboot (two reboots total).             |
| `waitForVolumeDetach` | `false`   | Drain and wait for CSI volumes to detach before the reboot (see below).            |
| `timeout`             | `30m`     | Per-node upgrade timeout, including how long a node may take to become ready after its reboot before being marked failed. |

/// tip | waitForVolumeDetach
Set this when a fast reboot could orphan a mount and pin a volume to the node -
which surfaces as a `Multi-Attach` error when the pod reschedules. tuppr drains
the node and waits for its CSI volumes to detach before rebooting. No effect on
single-node clusters.
///

## Node selection

Without a selector, a plan targets all nodes. Use `spec.nodeSelector` (a
standard label selector) to scope it - useful for opt-in labels or splitting a
cluster into plans:

```yaml
spec:
  nodeSelector:
    matchExpressions:
      - { key: tuppr.home-operations.com/upgrade, operator: In, values: ["enabled"] }
      - { key: node-role.kubernetes.io/control-plane, operator: DoesNotExist }
```

Multiple plans with different selectors queue; see [Upgrade
coordination](coordination.md).

## Parallel upgrades

By default nodes upgrade one at a time. `spec.parallelism` raises the batch
size:

```yaml
spec:
  talos:
    version: v1.13.7
  parallelism: 3 # up to 3 nodes at once
```

The webhook requires `parallelism >= 1` and no greater than the number of nodes
the selector matches. When `parallelism > 1`:

- Health checks run once **before each batch**, not per node.
- Drain (if configured) runs on all batch nodes before any upgrade Job starts.
- The batch waits for every node's Job to finish before the next batch begins.
- Any failure in a batch stops further batches.
- `status.currentNodes` lists the active batch.

## Draining

```yaml
spec:
  drain:
    enabled: true
    # disableEviction: false  # force delete instead of evicting
```

tuppr cordons and drains a node before its reboot and uncordons it after a
verified upgrade. Draining is skipped on single-node clusters (there is nowhere
to drain to, and evicting the upgrade pod would strand the node).

## Pre/post-upgrade hooks

Run side-effecting Jobs around the whole run - for example, set and clear Ceph
`noout` so brief reboots don't trigger rebalancing:

```yaml
spec:
  talos:
    version: v1.13.7
  hooks:
    pre:
      - name: ceph-set-noout
        image: ghcr.io/rook/rook:v1.18.7
        command: ["sh", "-c"]
        args: ["ceph osd set noout"]
        envFrom:
          - secretRef:
              name: rook-ceph-mon
    post:
      - name: ceph-unset-noout
        image: ghcr.io/rook/rook:v1.18.7
        command: ["sh", "-c"]
        args: ["ceph osd unset noout"]
        envFrom:
          - secretRef:
              name: rook-ceph-mon
```

- **Pre-hooks** run sequentially after the initial health check, before any node
  is touched. A failed pre-hook skips the upgrade and marks the run `Failed`
  (post-hooks still run as cleanup).
- **Post-hooks** run sequentially once the upgrade reaches a terminal state
  (success or failure), if any pre-hook was attempted. Their failures are logged
  and recorded but don't change the upgrade outcome.
- While pre-hooks are configured, **inter-batch health checks are suppressed** -
  the contract is that pre-hooks own cluster state for the upgrade window.
- Each hook is a Job in the controller namespace with the same non-root,
  capabilities-dropped posture as the upgrade Job. Mount credentials via
  `volumes` / `envFrom`, and set `serviceAccountName` if you need cluster-API
  access.

Phase progression with hooks:

```text
Pending → HealthChecking → PreHook → (Draining → Upgrading → Rebooting per batch) → PostHook → Completed
```

## Maintenance windows

Restrict when upgrades may **start**. An in-progress upgrade always runs to
completion.

```yaml
spec:
  maintenance:
    windows:
      - start: "0 2 * * 0" # cron: Sunday 02:00
        duration: "4h" # max 168h; warns if < 1h
        timezone: "Europe/Paris" # IANA tz, default UTC
```

- Upgrades stay `Pending` outside every window.
- Multiple windows are a union (any open window allows a start).
- A `TalosUpgrade` re-checks the window between nodes.
- No windows configured → upgrades start immediately.

## Alertmanager silences

Every node reboot during an upgrade fires a burst of expected alerts
(`CephMonDown`, `KubeNodeUnreachable`, `TargetDown`, ...). tuppr knows exactly
when that window starts and ends, so it can hold an Alertmanager silence open
for the run.

The connection is operator-level (credentials stay out of CRs - see the
[chart values](configuration.md)); which alerts get silenced is per-resource
and user-authored, same philosophy as `healthChecks`:

```yaml
# Helm values
silences:
  enabled: true
  alertmanager: # where the silences are created
    address: http://alertmanager.monitoring.svc:9093
    secretName: tuppr-alertmanager # optional: Secret of HTTP headers (Authorization, X-Scope-OrgID)
```

```yaml
# TalosUpgrade
spec:
  silences: # requires silences.* configured in the Helm chart
    - matchers:
        - name: alertname
          value: ^(CephMonDown|CephOSDDown|KubeNodeUnreachable|TargetDown)$
          matchType: "=~" # "=", "!=", "=~", "!~"
      maxDuration: 2h # default 4h
```

Each list entry is one Alertmanager silence, held for the whole run.

/// warning | Matchers within one silence are ANDed
Alertmanager semantics: a silence matches an alert only when **all** its
matchers do. To cover alternatives, use a regex value (as above); to silence
independent scopes - say Ceph alerts only in `namespace=rook-ceph` plus a
cluster-wide `TargetDown` - use separate `silences` entries, one per scope.
///

Anything speaking the Alertmanager v2 silences API works: Prometheus
Alertmanager, VMAlertmanager, or Grafana-managed alerting behind its prefix.

### The silence is a lease, not a timer

tuppr never guesses how long the run will take. It creates a **short silence
(~25m) and re-extends it on every reconcile** while the run is active, then
expires it down to a ~5m tail at the terminal phase (so the last reboot's
alert tail stays covered). Every failure mode converges to "the silence lapses
within its TTL":

- Controller crash or CR deleted mid-run → lapses on its own.
- Alertmanager unreachable → logged and evented, **never gates the upgrade**.
- Run parked (`Pending`, `MaintenanceWindow`, coordination wait) → lapses;
  re-established when the run resumes. A cluster parked overnight should alert.
- Suspending the CR (the `suspend` annotation) expires the silence immediately.

Two details worth knowing:

- The silence is created only once the run **leaves its initial health check**,
  so alerts stay live while the cluster is still being verified - and it keeps
  being extended through inter-batch health checks, so there is no mid-run gap.
- A run that is active but stuck (health checks failing for hours because
  something is genuinely broken) stops extending at each entry's `maxDuration`
  and emits a `SilenceMaxDurationReached` event - you get paged again instead
  of masking a real failure.

The silence IDs are visible in `.status.alertSilenceIDs` (indexed like
`spec.silences`), and their lifecycle is emitted as Events (`SilenceCreated`,
`SilenceExpired`, ...) on the resource. Configuring `spec.silences` without the
operator-level Alertmanager connection warns at apply time (admission webhook)
and as a `SilencesNotConfigured` event.

/// tip | Alternative: Alertmanager inhibition
If you'd rather keep alert routing entirely in Alertmanager, you can wire an
inhibition rule keyed on tuppr's `tuppr_talos_upgrade_phase` metric (via an
always-firing "upgrade in progress" alert) instead - zero tuppr configuration,
at the cost of maintaining the alert + `inhibit_rules` by hand.
///

## Per-node overrides

Annotations on a **Node** override the plan for that node - handy for a canary
version or a different installer flavor.

| Annotation                            | Purpose                                                                     | Example                           |
| ------------------------------------- | --------------------------------------------------------------------------- | --------------------------------- |
| `tuppr.home-operations.com/version`   | Target Talos version for this node.                                         | `v1.12.1`                         |
| `tuppr.home-operations.com/factory-url` | Switch the node's installer flavor on the next upgrade.                   | `factory.talos.dev/aws-installer` |
| `tuppr.home-operations.com/schematic` | Companion to `factory-url`; the schematic to build the image with.          | `b55fbf...`                       |

```bash
# Pin one node to a different version
kubectl annotate node worker-01 tuppr.home-operations.com/version="v1.12.1"

# Switch a node to a factory flavor at the next upgrade
kubectl annotate node hcloud-01 \
  tuppr.home-operations.com/factory-url="factory.talos.dev/hcloud-installer" \
  tuppr.home-operations.com/schematic="314b18a3f89d..."
```

### How the upgrade image is resolved

tuppr derives the image from each node's runtime state and
`.machine.install.image` - no per-platform branching:

1. **`factory-url` override** → builds `<factory-url>/<schematic>:<version>`. The
   schematic comes from the `schematic` annotation if set, otherwise from the
   node's runtime `ExtensionStatus`.
2. **Default** → version-swaps the node's current `.machine.install.image`. A
   factory install stays on its factory base + schematic; a private-registry path
   is preserved; a vanilla generic install stays vanilla.
3. **Safety net** → refuses with a clear error (pointing at `factory-url`) when
   the runtime schematic isn't in the install-image path, or when reinstalling
   the canonical generic installer would silently wipe installed system
   extensions.

## talosctl image

The upgrade Job's `talosctl` version is auto-detected. Pin it if needed:

```yaml
spec:
  talosctl:
    image:
      repository: ghcr.io/siderolabs/talosctl
      tag: v1.11.0
      pullPolicy: IfNotPresent
```
