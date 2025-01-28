# kat - Real-time Log Streaming for Kubernetes

`kat` (Kubernetes Attach & Tail Pod Logs) lets you stream logs from all containers across any number of namespaces in real-time. Think of it as `tail -f` for your entire Kubernetes cluster.

```sh
# Stream logs from your frontend namespace
kat frontend

# Stream from multiple namespaces and save to disk
kat --tee /tmp/logs frontend backend monitoring
```

## Why kat?

Managing logs across a Kubernetes cluster is challenging:
- `kubectl logs` requires separate commands for each pod
- You have to manually reattach when pods restart
- New pods are easily missed
- Multiple terminal windows needed for multiple namespaces

`kat` solves these problems by:
- Automatically discovering and attaching to all pods
- Handling pod restarts and new pod creation
- Streaming logs from all containers simultaneously
- Supporting multiple namespaces in a single command
- Saving logs to disk with automatic directory organization

## Quick Start

1. Install:
   ```sh
   go install github.com/frobware/kat/cmd/kat@latest
   ```

2. Run:
   ```sh
   # Stream all logs from current namespace
   kat
   
   # Stream from specific namespaces
   kat frontend backend
   
   # Stream and save logs to disk
   kat -d frontend  # Creates timestamped directory in /tmp
   ```

## Basic Usage

### Stream logs to console
```sh
# Single namespace
kat frontend

# Multiple namespaces
kat frontend backend monitoring
```

### Save logs to disk
```sh
# Auto-create timestamped directory
kat -d frontend
→ Using temporary log directory: /tmp/kat-2025-01-06T15:30:00

# Specify output directory
kat --tee /tmp/my-logs frontend

# Save logs but hide console output
kat --tee /tmp/logs --silent frontend
```

## Directory Structure

When saving logs (using `-d` or `--tee`), `kat` creates this structure:
```
/output-dir/
  └── namespace/
      └── pod-name/
          └── container-name.txt
```

## Common Options

Flag | Description | Default
---|---|---
`--since duration` | Show logs from last N minutes | 1m
`-d` | Auto-create temp directory in /tmp | -
`--tee string` | Write logs to specified directory | -
`--silent` | Disable console output | false
`--allow-existing` | Allow writing to existing directory | false

## Advanced Configuration

For high-throughput clusters or specific requirements:

Flag | Description | Default
---|---|---
`--qps float` | Kubernetes client QPS | 500
`--burst int` | Kubernetes client burst rate | 1000
`--kubeconfig string` | Path to kubeconfig | ~/.kube/config

## How It Works

```
┌──────────┐    ┌───────────┐    ┌──────────┐
│  Pod 1   │    │           │    │ Console  │
│ Container├────┤           ├────┤          │
│ Logs     │    │    kat    │    │          │
├──────────┤    │           │    ├──────────┤
│  Pod 2   │    │ Automatic │    │          │
│ Container├────┤ Discovery ├────┤  Files   │
│ Logs     │    │    &      │    │          │
├──────────┤    │ Streaming │    │          │
│   ...    │    │           │    │          │
└──────────┘    └───────────┘    └──────────┘
```

`kat` uses Kubernetes informers to watch for pod lifecycle events, automatically attaching to new pods and detaching from terminated ones. All container logs are streamed in real-time and can be displayed on the console and/or saved to disk.

## License

MIT License - see [LICENSE](LICENSE) for details.
