# kat - Kubernetes Attach & Tail

`kat` follows and streams logs from every container in every pod across specified Kubernetes namespaces in real-time.

## Installation
```sh
go install github.com/frobware/kat/cmd@latest
```

## Usage

```sh
% go run cmd/kat.go -help
Usage of /tmp/go-build1160906041/b001/exe/kat:
  -burst int
        Kubernetes client burst (default 1000)
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

## Example

```sh
% go run cmd/kat.go --tee /tmp/logs openshift-ingress openshift-ingress-operator
Started streaming logs: openshift-ingress/router-default-85fd9fff84-9dls7:router
Started streaming logs: openshift-ingress/router-default-85fd9fff84-fdlm2:router
Started streaming logs: openshift-ingress-operator/ingress-operator-ccc98f7f9-sgl5f:kube-rbac-proxy
Started streaming logs: openshift-ingress-operator/ingress-operator-ccc98f7f9-sgl5f:ingress-operator
Created log file: /tmp/logs/openshift-ingress/router-default-85fd9fff84-9dls7/router.log
Created log file: /tmp/logs/openshift-ingress-operator/ingress-operator-ccc98f7f9-sgl5f/kube-rbac-proxy.log
Created log file: /tmp/logs/openshift-ingress/router-default-85fd9fff84-fdlm2/router.log
Created log file: /tmp/logs/openshift-ingress-operator/ingress-operator-ccc98f7f9-sgl5f/ingress-operator.log
```

