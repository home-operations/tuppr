# Helm chart values

The **tuppr** chart is documented value-by-value in the generated
[chart README](https://github.com/home-operations/tuppr/blob/main/charts/tuppr/README.md)
(kept in sync with `values.yaml` by helm-docs; CI fails if it goes stale). The
chart also ships a
[`values.schema.json`](https://github.com/home-operations/tuppr/blob/main/charts/tuppr/values.schema.json)
for editor autocompletion and `helm install`-time validation.

This page is the orientation layer - which groups of values exist and where
their behavior is explained - followed by the full `values.yaml` for reference.

- **Controller** (image, replicas, resources, `securityContext` /
  `podSecurityContext`, `nodeSelector`, `priorityClassName`, extra `env`): the
  deployment itself.
- **`notification`** (`enabled`, `secretName`, `secretKey`, `titleTemplate`,
  `messageTemplate`): upgrade notifications - see [Notifications](notifications.md).
- **`silences`** (`enabled`, `alertmanager.address`, `alertmanager.secretName`):
  the operator-level Alertmanager connection for upgrade-run silences
  (`spec.silences` on a TalosUpgrade) - see
  [Alertmanager silences](talos-upgrades.md#alertmanager-silences).
- **`monitoring`** (`serviceMonitor`, `prometheusRule`, `dashboards`): see
  [Monitoring](monitoring.md).
- **`rbac`** and **`webhook`** (`certManager`): the ClusterRole the controller
  needs and the admission webhook's certificate wiring.

## values.yaml

```yaml
--8<-- "charts/tuppr/values.yaml"
```
