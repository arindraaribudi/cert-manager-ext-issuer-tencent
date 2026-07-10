# cert-manager-ext-issuer-tencent

A cert-manager companion controller that mirrors pre-uploaded Tencent Cloud SSL
certificates into Kubernetes Secrets. Tencent owns issuance and renewal; this
controller synchronizes the resulting certificate chain and private key into a
target Secret managed alongside your cert-manager `Certificate` resources.

This controller is **not** a cert-manager external issuer in the strict sense
(it does not use the `CertificateRequest` Sign interface). It watches
`Certificate` CRs directly and writes Secrets directly. The
`spec.issuerRef` field is honored for grouping, but no signing handshake with
cert-manager is performed.

## Requirements

- Kubernetes 1.27+
- cert-manager 1.13+ (only the `Certificate` CRD is required)
- Tencent Cloud account with SSL Certificates service access
- A CAM role with `ssl:Describe*` / `ssl:DownloadCertificate` permissions

### Authentication

Default mode is **TKE pod identity**. The controller's pod picks up the
CAM role bound to its ServiceAccount via the SDK credential chain — no
API keys stored in the cluster. Static `SecretId` / `SecretKey` are still
supported as a fallback (see [Static credentials](#static-credentials)).

## Install

Apply the latest release manifest (CRDs + RBAC + Deployment bundled):

```bash
kubectl apply -f https://github.com/arindraaribudi/cert-manager-ext-issuer-tencent/releases/latest/download/install.yaml
```

For a specific version:

```bash
kubectl apply -f https://github.com/arindraaribudi/cert-manager-ext-issuer-tencent/releases/download/v0.1.0/install.yaml
```

For local development (CRDs only, then point at your own image):

```bash
make install
make deploy IMG=ghcr.io/arindraaribudi/cert-manager-ext-issuer-tencent:dev
```

## Quick start (TKE pod identity — default)

### 1. Bind a CAM role to the controller ServiceAccount

The controller runs as ServiceAccount `cert-manager/controller-manager-tencent`.
Annotate it with the CAM role ARN that has `ssl:Describe*` /
`ssl:DownloadCertificate` — the TKE pod-identity webhook then injects
`TKE_REGION`, `TKE_PROVIDER_ID`, `TKE_WEB_IDENTITY_TOKEN_FILE`, and
`TKE_ROLE_ARN` into the pod and the controller picks them up automatically.

```bash
kubectl -n cert-manager annotate serviceaccount controller-manager-tencent \
  tke.cloud.tencent.com/audience=sts.cloud.tencent.com \
  tke.cloud.tencent.com/role-arn=qcs::cam::uin/1234567890:roleName/controller-manager-tencent-cert-manager-sa \
  tke.cloud.tencent.com/token-expiration=86400
```

### 2. Create Issuer

Omit `secretRef` to use the credential chain (pod identity → STS env → CVM
metadata):

```yaml
apiVersion: tencent.cert-manager.io/v1alpha1
kind: TencentIssuer
metadata:
  name: prod
  namespace: cert-manager
spec:
  region: ap-guangzhou
  resyncInterval: 24h
```

### 3. Annotate your Certificates

For each `Certificate`, add the Tencent cert ID:

```yaml
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: my-app-cert
  annotations:
    tencent.cert-manager.io/certificate-id: abc123def
spec:
  secretName: my-app-tls
  issuerRef:
    name: prod
    kind: TencentIssuer
    group: tencent.cert-manager.io
  duration: 2160h   # 90d
  renewBefore: 720h  # 30d
```

The controller will:

1. Find the Tencent certificate matching `abc123def`
2. Download the certificate chain + private key from Tencent
3. Write `Secret/my-app-tls` of type `kubernetes.io/tls` with both `tls.crt` and `tls.key`
4. Set the Certificate's `status.conditions[Ready]` to `True`

## Periodic re-sync

The controller's background ticker (interval `spec.resyncInterval`, default 24h)
re-downloads each managed cert from Tencent and compares the SHA-256 hash of
the remote `tls.crt` against the local Secret. If they differ:

- The Issuer's `status.conditions[Synced]` is set to `False` with
  reason `RemoteChanged`.
- Operators should investigate or re-annotate the Certificate.

For an automatic refresh, use the force-sync annotation (below) or trigger a
Certificate reconcile (delete the Secret, bump a label, etc.).

## Force sync

Force an immediate re-download:

```bash
kubectl annotate certificate my-app-cert tencent.cert-manager.io/force-sync=true
```

The resync ticker clears the annotation after handling.

## ClusterIssuer (cluster-scoped)

`TencentClusterIssuer` is fully supported. Use it when one set of Tencent
credentials should serve Certificates across all namespaces:

```yaml
apiVersion: tencent.cert-manager.io/v1alpha1
kind: TencentClusterIssuer
metadata:
  name: prod
spec:
  region: ap-guangzhou
  resyncInterval: 24h
```

Reference it from a Certificate the same way, with `kind: TencentClusterIssuer`:

```yaml
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: my-app-cert
  annotations:
    tencent.cert-manager.io/certificate-id: abc123def
spec:
  secretName: my-app-tls
  issuerRef:
    name: prod
    kind: TencentClusterIssuer
    group: tencent.cert-manager.io
```

## RBAC

The controller needs:

- `get/list/watch` on `certificates`, `tencentissuers`, `tencentclusterissuers`
- `get/list/watch/create/update` on `secrets` (in the namespaces it manages)
- `update/patch` on `certificates/status` and `tencentissuers/status`

## Configuration

| Flag | Default | Description |
| --- | --- | --- |
| `--metrics-addr` | `:8080` | Metrics bind address |
| `--health-addr` | `:8081` | Health probe bind address |
| `--leader-elect` | `true` | Enable leader election |

Issuer spec fields:

| Field | Required | Description |
| --- | --- | --- |
| `region` | yes | Tencent Cloud region (e.g. `ap-guangzhou`) |
| `secretRef.name` | yes | Name of the Secret holding `secret-id`/`secret-key` |
| `endpoint` | no | Override Tencent API endpoint (default: `ssl.tencentcloudapi.com`) |
| `resyncInterval` | no | Re-sync period (default: `24h`, minimum: `5m`) |

## Development

```bash
make manifests     # regenerate CRDs + RBAC
make generate      # regenerate deep-copy
make build         # build manager binary to bin/manager
make build-installer IMG=<image:tag>  # produce dist/install.yaml
make test          # unit tests
make lint          # golangci-lint
```

## Static credentials

Use static API keys only when pod identity isn't available (non-TKE clusters,
local dev). Create the Secret and reference it from the Issuer:

```bash
kubectl -n cert-manager create secret generic tencent-creds \
  --from-literal=secret-id=$TENCENT_SECRET_ID \
  --from-literal=secret-key=$TENCENT_SECRET_KEY
```

```yaml
apiVersion: tencent.cert-manager.io/v1alpha1
kind: TencentIssuer
metadata:
  name: prod
  namespace: cert-manager
spec:
  region: ap-guangzhou
  secretRef:
    name: tencent-creds   # any non-empty value triggers static mode
  resyncInterval: 24h
```

## License

Apache-2.0