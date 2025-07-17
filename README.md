# kat - Real-time Log Streaming for Kubernetes

`kat` (Kubernetes Attach & Tail Pod Logs) streams logs from all containers across namespaces in real-time. It supports glob patterns for namespace matching and automatically discovers new pods.

```sh
# Stream logs from your frontend namespace
kat frontend

# Stream from multiple namespaces with patterns
kat frontend backend go-*

# Stream from all namespaces (requires cluster-wide permissions)
kat -A

# Exclude development namespaces
kat -A --exclude "*-dev"
```

## Namespace Patterns

`kat` supports glob patterns for namespace matching:

- `*` matches any sequence of characters
- `?` matches any single character
- `[abc]` matches any character in the set

Examples:
- `kat go-*` matches `go-service`, `go-backend`
- `kat test-?` matches `test-1`, `test-a` but not `test-10`
- `kat app-[123]` matches `app-1`, `app-2`, `app-3`

The `--exclude` flag uses the same patterns and can be repeated or comma-separated.

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

   # Stream with patterns
   kat go-* frontend-*

   # Stream and save logs to disk
   kat -d frontend  # Creates timestamped directory in /tmp
   ```

## Basic Usage

### Stream logs to console
```sh
# Single namespace
kat frontend

# Multiple namespaces
kat frontend backend

# Glob patterns
kat go-* frontend-*

# All namespaces
kat -A

# Exclude patterns
kat -A --exclude "*-dev" --exclude "kube-*"
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
`-A` | Watch all namespaces | false
`--exclude` | Exclude namespace patterns (repeatable) | -
`--since duration` | Show logs from last N minutes | 1m
`-d` | Auto-create temporary directory in /tmp | -
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

`kat` uses Kubernetes informers to watch for pod lifecycle events, automatically attaching to new pods and detaching from terminated ones. When using glob patterns or the `-A` flag, it watches for namespace changes and starts streaming from matching namespaces as they appear.

## License

MIT License - see [LICENSE](LICENSE) for details.
