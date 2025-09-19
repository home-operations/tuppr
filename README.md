# tuppr - Talos Linux Upgrade Controller

A Kubernetes controller for managing automated upgrades of Talos Linux nodes and Kubernetes control plane components.

## ✨ Features

- 🚀 **Automated Talos node upgrades** with safe orchestration
- 🎯 **Kubernetes control plane upgrades** via Talos *(coming soon)*
- 🔒 **Safe upgrade execution** - upgrades run from healthy nodes (never self-upgrade)
- 📊 **Built-in health checks** - validates cluster health before and during upgrades
- 🎛️ **Flexible node targeting** with label selectors
- 🔄 **Configurable reboot modes** - default or powercycle
- 📋 **Comprehensive status tracking** with detailed progress reporting
- ⚡ **Resilient job execution** with automatic retry and pod replacement

## 🧪 Testing Guide

> **⚠️ Important**: Pause System Upgrade Controller (SUC) before testing to avoid conflicts.

### 1. Preparation

Create the namespace:

```bash
kubectl create namespace system-upgrade
```

Create Talos configuration secret by applying Talos with this config:

```yaml
machine:
  # ...
  features:
    # ...
    kubernetesTalosAPIAccess:
      allowedKubernetesNamespaces:
        - system-upgrade
      allowedRoles:
        - os:admin
      enabled: true
# ...
```

Create your `values.yaml`:

```yaml
image:
  repository: ghcr.io/home-operations/tuppr
  tag: main-382f108 # Use latest sha from packages
```

### 2. Installation

```bash
helm install tuppr oci://ghcr.io/home-operations/charts/tuppr \
  --version 0.0.0 \
  --values values.yaml \
  --namespace system-upgrade
```

### 3. Initial State Check

Create a TalosUpgrade matching your **current** cluster state:

```yaml
apiVersion: tuppr.home-operations.com/v1alpha1
kind: TalosUpgrade
metadata:
  name: cluster
spec:
  target:
    image:
      repository: factory.talos.dev/metal-installer/YOUR_CURRENT_SCHEMATIC_WITHOUT_TAG
      tag: v1.11.1
    options: # Optional
      debug: false # Optional, default: false
      force: false # Optional, default: false
      rebootMode: default # Optional, default: default
    # You can create a TalosUpgrade per node
    #   Just make sure update the TalosUpgrade name to the node name (or whatever)
    #   and set the nodeSelector to the node name
    nodeSelector: {} # Optional
      # kubernetes.io/hostname: k8s-0
  talosctl: # Optional
    image: # Optional
      repository: ghcr.io/siderolabs/talosctl # Optional, default: ghcr.io/siderolabs/talosctl
      tag: v1.11.1 # Optional, default: current installed Talos version
      pullPolicy: IfNotPresent # Optional, default: IfNotPresent
```

Check that the controller recognizes the current state:

```bash
kubectl get talosupgrade cluster -o yaml
```

**Expected**: Status should show all nodes as already upgraded.

### 4. Test Downgrade

Modify the TalosUpgrade to downgrade to a previous version:

```yaml
spec:
  target:
    image:
      tag: v1.11.0 # Previous version
```

### 5. Monitor the Upgrade

Watch the upgrade progress:

```bash
# Terminal 1: Watch TalosUpgrade status
watch kubectl get talosupgrade cluster

# Terminal 2: Watch jobs and pods
watch kubectl get jobs,pods -n system-upgrade

# Terminal 3: Stream logs
stern -n system-upgrade cluster
```

### 6. Verify and Test Back

Once downgrade completes:

- All nodes should be running v1.11.0
- TalosUpgrade status should show `phase: Completed`
- Jobs are cleaned up automatically

**Test upgrade**: Change the TalosUpgrade back to v1.11.1 and repeat monitoring.

### 7. Cleanup

```bash
# Remove test resources
kubectl delete talosupgrade cluster

# Remove controller
helm uninstall tuppr --namespace system-upgrade

# Remove CRDs
kubectl delete crd talosupgrades.tuppr.home-operations.com
```

## 📖 How It Works

1. **Safety First**: Upgrade jobs always run on nodes different from the target node
2. **Health Checks**: Pre-upgrade validation ensures cluster health
3. **Sequential Upgrades**: Nodes are upgraded one at a time to maintain availability
4. **Status Tracking**: Real-time progress updates via Kubernetes status fields
5. **Automatic Cleanup**: Completed jobs are automatically cleaned up after 15 minutes

### Known Limitations

- **Single node clusters**: Not supported (upgrades require running from other nodes)
- **Network policies**: May interfere with Talos API access
- **Resource constraints**: Upgrade jobs need sufficient CPU/memory

## 🚧 Current Status

This project is in active development. Current roadmap:

- [x] Talos node upgrade controller
- [ ] Kubernetes control plane upgrade controller
- [ ] Comprehensive test suite
- [ ] Production-ready Helm chart
- [ ] Advanced metrics and alerting
- [ ] CI/CD workflows

## 🤝 Contributing

1. Fork the repository
2. Create a feature branch: `git checkout -b feature/amazing-feature`
3. Make your changes and add tests
4. Commit your changes: `git commit -m 'Add amazing feature'`
5. Push to the branch: `git push origin feature/amazing-feature`
6. Open a Pull Request

## 📄 License

This project is licensed under the GNU Affero General Public License - see the [LICENSE](LICENSE) file for details.

## 🙏 Acknowledgments

- [Talos Linux](https://www.talos.dev/) - The modern OS for Kubernetes
- [System Upgrade Controller](https://github.com/rancher/system-upgrade-controller) - Inspiration for upgrade orchestration
- [Kubebuilder](https://book.kubebuilder.io/) - Framework for building Kubernetes controllers
