# kat - Kubernetes Attach & Tail

`kat` follows and streams logs from every container in every pod across specified Kubernetes namespaces in real-time.

## Installation
```bash
go install github.com/frobware/kat@latest
```

## Example

```bash
% go run kat.go --since=3s openshift-ingress openshift-ingress-operator | grep -v Proxy
2025/01/03 17:57:06 Using kubeconfig: /home/aim/.kube/config
2025/01/03 17:57:06 Streaming logs for pod: openshift-ingress-operator/idle-close-on-response-rr5xj-external-verify:echo
2025/01/03 17:57:06 Streaming logs for pod: openshift-ingress-operator/idle-close-on-response-jpgfr-external-verify:echo
2025/01/03 17:57:06 Streaming logs for pod: openshift-ingress-operator/ingress-operator-7cd8fdc79c-6c2pd:kube-rbac-proxy
2025/01/03 17:57:06 Streaming logs for pod: openshift-ingress-operator/ingress-operator-7cd8fdc79c-6c2pd:ingress-operator
2025/01/03 17:57:06 Streaming logs for pod: openshift-ingress-operator/idle-close-on-response-n9k77-external-verify:echo
2025/01/03 17:57:06 Streaming logs for pod: openshift-ingress/router-default-8554d4c6c6-vhdk4:logs
2025/01/03 17:57:06 Streaming logs for pod: openshift-ingress/router-default-8554d4c6c6-vhdk4:router
```

