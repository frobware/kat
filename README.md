# kat - Kubernetes Attach & Tail

`kat` streams logs from all containers in specified Kubernetes namespaces in real-time.

## Installation

```sh
go install github.com/frobware/kat/cmd/kat@latest
```

## Usage

```sh
% go run cmd/kat.go --help
  -allow-existing
        Allow logging to an existing directory (default: false)
  -burst int
        Kubernetes client burst (default 1000)
  -d
        Automatically create a temporary directory for logs
  -kubeconfig string
        Path to kubeconfig
  -qps float
        Kubernetes client QPS (default 500)
  -silent
        Disable console output for log lines
  -since duration
        Show logs since duration (e.g., 5m) (default 1m0s)
  -tee string
        Directory to write logs to (optional)
```

### Flags
- `-allow-existing`: Allow kat to log to an existing directory without aborting. Use this flag with -tee to continue logging into an existing directory.
- `-burst`: The maximum burst size for the Kubernetes client rate limiter. Increase this for higher throughput.
- `-d`: Automatically create a temporary directory for logs in the format /tmp/kat-<timestamp>.
- `-kubeconfig`: Path to the Kubernetes kubeconfig file. Defaults to ~/.kube/config if not specified.
- `-qps`: Queries per second (QPS) for the Kubernetes client. Increase this value for higher throughput.
- `-silent`: Suppress console output for log lines. Progress messages (e.g., log file creation) will still be displayed.
- `-since`: Show logs from the specified duration (e.g., 5m for logs from the last 5 minutes). Defaults to 1m.
- `-tee`: Write logs to the specified directory. If combined with -allow-existing, logs will be appended to an existing directory.

## Examples

Stream logs to the console for multiple namespaces:
```sh
$ kat openshift-ingress openshift-ingress-operator
```

Stream logs and write to a specific directory:
```sh
$ kat --tee /tmp/logs openshift-ingress openshift-ingress-operator
```

Stream logs and automatically create a temporary directory:
```sh
kat -d openshift-ingress
Output: Using temporary log directory: /tmp/kat-2025-01-06T15:30:00
```

Stream logs to an existing directory:
```sh
kat --tee /tmp/logs --allow-existing openshift-ingress
```

Suppress console output and write logs to a directory:
```sh
kat --tee /tmp/logs --silent openshift-ingress
```

### Notes
- By default, if no namespace is specified, kat streams logs from the current Kubernetes namespace.
- If `--silent` is used, container logs are not printed to the console, but progress messages (e.g., “Started streaming logs”) are still displayed.
- Logs are written lazily, meaning directories and files are only created when log entries are received.
