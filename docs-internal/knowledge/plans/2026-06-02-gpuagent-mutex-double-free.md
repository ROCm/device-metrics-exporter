# gpuagent: mutex fix for g_gpu_metrics double-free

- **Date:** 2026-06-02
- **Author:** Bhatturu
- **Related PR(s):** TBD
- **Related issue(s) / JIRA:** GPUOP-832

## Context

gpuagent crashes with SIGABRT (double-free) under concurrent gRPC stress.
Root cause: the global `std::unordered_map<aga_gpu_handle_t, amdsmi_gpu_metrics_t> g_gpu_metrics`
in `smi_api.cc` is accessed from 6 sites across multiple gRPC handler threads
without synchronization. Concurrent `.clear()` and `operator[]` calls race,
corrupting the internal hash table and triggering glibc's double-free detection.

Reproduced on Radeon W7900 (RDNA3, Navi 31) with 60 concurrent gpuctl calls.

## Approach

Add `std::mutex g_gpu_metrics_mutex` protecting all 6 access sites with
`std::lock_guard<std::mutex>`. Minimal, surgical fix — no architectural changes.

### Alternatives considered

- **Per-GPU mutex (sharded locks):** Lower contention but higher complexity.
  Rejected — contention is not a bottleneck; single mutex is sufficient.
- **Read-write lock (`shared_mutex`):** Allows concurrent reads. Rejected —
  the map is cleared+rebuilt every poll cycle, so writes are frequent.
- **Lock-free concurrent map:** Overkill for this use case.

## Scope

- **In scope:** `smi_api.cc` mutex addition (gpu-agent repo), updated
  `gpuagent_static.bin.gz` asset in DME repo.
- **Out of scope:** Other gpuagent thread-safety issues, profiler metrics,
  SR-IOV variant (same fix applies but separate asset).

## Validation

- **Crash reproduction (without fix):** 5 rounds × 60 concurrent gpuctl →
  SIGABRT at `smi_api.cc:146` (`g_gpu_metrics.clear()`), Thread 170/1028.
- **Fix verification (with fix):** 10 rounds × 60 concurrent gpuctl = 600
  calls, no crash, gpuagent healthy.
- **Test environment:** K8s GPU operator (4× W7900, k3s v1.35.5).
- **Mock test:** DME image serves 217 metrics, 200 concurrent requests pass.

## Risks and rollback

- **Known risks:** Mutex adds serialization to the hot path. Measured impact
  is negligible — gpuctl calls take ~100ms, lock hold time is <1ms.
- **Rollback plan:** Revert gpuagent_static.bin.gz to previous commit's asset.
