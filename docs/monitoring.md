# Monitoring

tuppr exposes Prometheus metrics under the `tuppr_` prefix. Hit `/metrics` on
the controller pod for the authoritative list (keeping a copy here would drift):

```bash
kubectl port-forward -n system-upgrade deployment/tuppr 8081:8081
curl -s http://localhost:8081/metrics | grep tuppr_
```

The chart wires up the standard observability stack on demand:

- **`monitoring.serviceMonitor.enabled: true`** - a `ServiceMonitor` for
  Prometheus Operator scraping.
- **`monitoring.prometheusRule.enabled: true`** - a bundled `PrometheusRule`
  with stuck-upgrade, failed-upgrade, and operator-absent alerts.
- **`monitoring.dashboards.enabled: true`** - a Grafana dashboard `ConfigMap`
  (sidecar-discoverable). Set `monitoring.dashboards.grafanaOperator.enabled:
  true` with `matchLabels` to also render a `GrafanaDashboard` CR for
  grafana-operator.

All three are off by default. See the [Helm chart values](configuration.md) for
the full set.

To silence *your own* alerts (Ceph, node-down, scrape failures) during upgrade
windows, see [Alertmanager
silences](talos-upgrades.md#alertmanager-silences).
