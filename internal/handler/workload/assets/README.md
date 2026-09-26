# Pinned Gateway API installer manifest

`envoy-gateway-v1.9.1-install.yaml.gz` is the unmodified
[`install.yaml` from Envoy Gateway v1.9.1](https://github.com/envoyproxy/gateway/releases/download/v1.9.1/install.yaml),
compressed with `gzip -n -9` so KubePilot can install into clusters without
outbound GitHub access. The SHA-256 of the **decompressed** manifest is
`72b3971364f172eb0b9636c7142cc84ff695467bc065897958bde85a3c06cfd5`.

Update the release URL, digest, compatibility check, and manifest together.
The installer verifies this digest before invoking `kubectl apply --server-side`.
See the upstream project for the manifest's license and provenance.
