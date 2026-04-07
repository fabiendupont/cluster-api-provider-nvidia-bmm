# Cluster API Provider NVIDIA NCX Infrastructure Controller

Cluster API (CAPI) infrastructure provider for NICo (NVIDIA NCX
Infrastructure Controller). Manages cluster-level networking
(VPC, subnets, NSG, peering) and instance lifecycle for
Kubernetes clusters provisioned on NICo bare-metal infrastructure.

## Build and test

```bash
go build ./...
# Unit tests (currently disabled — see TESTING.md)
go test ./... -v
# Integration tests (require envtest)
go test ./test/integration/ -v
# E2E tests (require live NICo API)
NICO_API_ENDPOINT=https://... NICO_TOKEN=... go test ./test/e2e/ -v
```

## Key files

- `api/v1beta1/` — CRD types: NcxInfraCluster, NcxInfraMachine,
  NcxInfraClusterTemplate, NcxInfraMachineTemplate
- `internal/controller/ncxinfracluster_controller.go` — cluster
  reconciler (VPC, subnet, NSG, peering lifecycle)
- `internal/controller/ncxinframachine_controller.go` — machine
  reconciler (instance create/poll/delete)
- `pkg/scope/cluster.go` — NICo SDK client wrapper
- `pkg/providerid/providerid.go` — provider ID parsing
- `test/` — integration and e2e tests

## SDK

Uses `github.com/NVIDIA/ncx-infra-controller-rest v1.2.0`
(official SDK, no forks). Auth via JWT bearer token in context.

## Current status

v0.1.0, alpha. Unit tests disabled post-SDK migration (need HTTP
mock server — see TESTING.md). Integration and E2E tests functional.

---

## Work to do

The following changes align this CAPI provider with the NCP
reference architecture vision. Key reference documents:

- NEP-0007: `~/Code/github.com/NVIDIA/ncx-infra-controller-rest/docs/enhancements/0007-fault-management-provider.md`
- NEP-0001: `~/Code/github.com/NVIDIA/ncx-infra-controller-rest/docs/enhancements/0001-extensible-architecture.md`

### 1. Unify provider ID scheme

**Current:** `ncx-infra://org/tenant/site/instance-id`
**Target:** `nico://org/tenant/site/instance-id`

Changes:
- `pkg/providerid/providerid.go`: Change scheme from `ncx-infra`
  to `nico`
- Keep backward compatibility: `ParseProviderID()` must accept
  both `ncx-infra://` (legacy) and `nico://` (new) on read
- `FormatProviderID()` must always write `nico://`
- Update all test cases
- Update any hardcoded scheme references in controllers

The CCM at `~/Code/github.com/fabiendupont/cloud-provider-nvidia-ncx-infra-controller/`
already uses `nico://`. The MAPI provider is being updated
simultaneously. All three must agree for the CCM to correlate
nodes with instances.

### 2. Set FailureReason and FailureMessage from fault events

**Current:** `NcxInfraMachine.Status.FailureReason` and
`FailureMessage` exist in the CRD but the controller rarely sets
them. When an instance enters Error state, the controller just
logs it.

**Target:** When the machine controller detects instance state
`Error`:

1. Query `GET /v2/org/{org}/carbide/health/events?machine_id={id}&state=open&severity=critical`
2. If fault events found, set:
   ```go
   machine.Status.FailureReason = capierrors.UpdateMachineError
   machine.Status.FailureMessage = "GPU fault: gpu-xid-48 — automated remediation in progress"
   ```
3. If no fault events found (or health API unavailable), fall back
   to current behavior

This gives CAPI's MachineHealthCheck meaningful data to act on.

Guard behind `/v2/org/{org}/carbide/capabilities` check for
`fault-management` feature.

### 3. Add MachineHealthCheck support

**Current:** No MachineHealthCheck integration. No health
conditions on NcxInfraMachine.

**Target:** Add conditions to `NcxInfraMachine.Status.Conditions`:

```go
// Condition types to add
const (
    NcxInfraMachineHealthy         = "NicoHealthy"
    NcxInfraMachineFaultRemediation = "NicoFaultRemediation"
)
```

In the machine reconciler's `Update` path:
1. Query health events API for the machine
2. Set `NicoHealthy` condition (True/False)
3. Set `NicoFaultRemediation` condition if remediation in progress
4. CAPI's MachineHealthCheck can then watch these conditions

### 4. Pre-flight fault check before instance creation

**Current:** The machine controller creates instances without
checking if the target hardware is healthy.

**Target:** Before calling `CreateInstance()`:

1. If the spec targets a specific `machineID`, query
   `GET /health/events?machine_id={id}&state=open&severity=critical`
2. If open critical faults exist, do NOT create the instance.
   Instead:
   - Set `FailureReason` to explain why
   - Requeue with backoff (the fault may resolve via automated
     remediation)
   - Log a warning event

Note: NICo's `pre-create-instance` hook (NEP-0007) also blocks
this at the API level, but checking client-side gives a better
error message to the user.

### 5. Error classification and retry logic

**Current:** Fixed requeue intervals (10-30s), no distinction
between transient and terminal API errors.

**Target:**
- Wrap NICo API calls with error classification:
  - HTTP 429, 503, timeout, connection refused → transient
    (requeue with exponential backoff: 1s, 2s, 4s, 8s, max 60s)
  - HTTP 400 (bad request) → terminal (set FailureReason, stop)
  - HTTP 404 on GET → resource gone (handle gracefully)
  - HTTP 409 (conflict) → transient (resource being modified)
- Add a `retryableClient` wrapper in `pkg/scope/` that handles
  this for all API calls
- Record retry attempts in machine events

### 6. Re-enable unit tests

**Current:** Unit tests disabled post-SDK migration. TESTING.md
documents the HTTP mock server approach needed.

**Target:**
- Implement an HTTP mock server that returns canned responses
  for the NICo API endpoints used by the controllers
- The mock must handle the generated SDK client's response types
  (JSON201 for creates, JSON200 for reads)
- Re-enable all test files (remove `.disabled` extension)
- Add test cases for:
  - Health event query integration
  - Fault-aware instance creation (blocked by fault)
  - Error classification (transient vs terminal)
  - Provider ID parsing (both schemes)

### 7. Prometheus metrics

**Current:** Only `VPCCount` metric exists. Other metrics
registered but never recorded.

**Target:**
- `nico_capi_api_latency_seconds` (histogram) by endpoint
- `nico_capi_api_errors_total` (counter) by endpoint and type
- `nico_capi_machines_managed` (gauge)
- `nico_capi_machines_unhealthy` (gauge)
- `nico_capi_instance_provision_duration_seconds` (histogram)
  — time from create request to Ready state
- Wire up the existing histogram that was registered but never
  used

## Design constraints

- All new health features must be guarded behind capability
  checks (`fault-management` feature in `/capabilities`)
- Graceful degradation: if NICo doesn't support NEP-0007 yet,
  fall back to current behavior
- Follow CAPI conventions for conditions, failure reasons, and
  event types
- Provider ID change must be backward compatible on read
- All changes must pass `go build ./...`
- New features must have unit tests (with the mock server)
