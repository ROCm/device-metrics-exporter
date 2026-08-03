# CLAUDE.md — AMD Device Metrics Exporter

Workflow rules and non-obvious gotchas for this repo. Architecture, component deep-dives, and troubleshooting walkthroughs live in [`docs-internal/knowledge/exporter/`](docs-internal/knowledge/exporter/) — load on demand, not every session.

## Build & test

All builds happen inside the build container. Never `go build` against the host toolchain.

```bash
make docker-shell   # enter build container — required shell for all builds below
make gen            # regenerate protobuf
make all            # build amdexporter binary
make unit-test      # run unit tests
make docker         # build deployment container image
make pkg            # build Debian + RPM packages
```

Custom skills wrap the multi-step builds — prefer them over invoking make manually.
All project skills live in [`.claude/skills/`](.claude/skills/) and are auto-discovered.

**PR creation/update:** always use `/pr-create` — enforces `[JIRA-ID]` title, six required sections, ≤300 words.

> Skills removed (superseded by codie): `prd-dev-workflow`, `prd-metric-add`, `prd-metric-implementation`.
> Use `/codie:autodev` / `/codie:task-implement` instead. Invariants preserved in
> [`docs-internal/knowledge/exporter/gpu-metric-authoring.md`](docs-internal/knowledge/exporter/gpu-metric-authoring.md).

## Repo etiquette

- **Branch off `main`** for general work. The `collab-*` branches (e.g. `collab-2.0.0`) are integration branches for specific epics — only base on them if your work is explicitly part of that epic.
- **Config schema:** all runtime config goes through [`pkg/exporter/proto/exporterconfig.proto`](pkg/exporter/proto/exporterconfig.proto). Edit the `.proto`, then `make gen`. Never hand-edit generated `*.pb.go`.
- **Runtime config auto-reloads every 3 seconds** — no restart needed when changing `/etc/metrics/config.json`.

## Don't touch

- **Root `entrypoint.sh`** — this is the jobd CI dev-container helper, NOT the runtime container entrypoint. The runtime entrypoint is [`docker/entrypoint.sh`](docker/entrypoint.sh). Editing the root file silently breaks CI.
- **Anything generated** — `*.pb.go`, `pkg/amdgpu/gen/`, vendored amdsmi/gpuagent assets. Regenerate via `make gen` or the relevant `/amdsmi-update` / `/rocm-update` skill.

## Project conventions

- **Metric name prefix is `amd_*`**, derived from `MetricsFieldPrefix` in `config.json`. When grepping `/metrics` output, search `^amd_ifoe_`, never `^ifoe_` or `^gpu_ual_ifoe_` — those won't match and a test asserting on them will silently pass.
- **Test port discovery:** any test that hits `/metrics` on a shared lab host must read `ServerPort` from the target's `/etc/metrics/config.json` first. The hourly build owns port 5000 — never hardcode it.

## Build gotchas

- `make debpkg-ual` and `make rpmpkg-ual` leave three transient dirty files in `debian/` from sed scaffolding (see `Makefile.package:266-275`) — these are normal mid-build, not real edits. Don't commit them.
- All `docker run` targets in `Makefile.package` need a TTY. Running them via a non-TTY harness will hang.
- `make rpmpkg-ual` requires `libcopy-assets-RHEL9` to have populated `build/assets/RHEL9/profilerlibs/` first. If `libcopy` failed silently (TTY trap), `rpmpkg-ual` produces a misleading error.

## Troubleshooting (top hits)

| Symptom | Likely cause | First check |
|---|---|---|
| `ErrZeroGPUs`, all GPUs unhealthy at startup | amdgpu driver loads after gpuagent | `-exit-on-agent-down` flag + K8s `restartPolicy: Always` |
| `GPU_PROF_*` metrics missing | ROCProfiler init failed (3-failure auto-disable kicked in) | Disable in config: `"ProfilerMetrics": {"all": false}` |
| Config changes not taking effect | Invalid JSON | `jq . /etc/metrics/config.json` |

Deeper troubleshooting tree: [`docs-internal/knowledge/exporter/troubleshooting.md`](docs-internal/knowledge/exporter/troubleshooting.md).

## Where to look

- **Entry point:** [`cmd/exporter/main.go`](cmd/exporter/main.go)
- **GPU client:** [`pkg/amdgpu/gpuagent/`](pkg/amdgpu/gpuagent/)
- **NIC client:** [`pkg/amdnic/nicagent/`](pkg/amdnic/nicagent/)
- **Architecture / deep dives:** [`docs-internal/knowledge/exporter/`](docs-internal/knowledge/exporter/)
- **User-facing docs (Sphinx):** [`docs/`](docs/)
- **PRDs:** [`docs-internal/knowledge/prds/`](docs-internal/knowledge/prds/)

## Per-PR plan file requirement

Every PR to `main` requires a plan file. See [`docs-internal/knowledge/CONTRIBUTING.md`](docs-internal/knowledge/CONTRIBUTING.md).

---

# Pensando Repo — Claude Code Context

> This file is loaded automatically by Claude Code on every session.
> It instructs Claude how to assist developers with the branch workflow.

---

## Your Role

When a developer asks about branches, PRs, or git workflow, proactively guide them toward
the branch naming convention. Do not wait to be asked — if you see someone about to create
a branch with the wrong format, correct them before they push.

**Whenever you create or suggest a branch name, you MUST:**

**Step 1 — Resolve the GitHub username:**
```bash
if command -v gh &>/dev/null; then
  LOGIN=$(gh api user --jq .login 2>/dev/null)
else
  # gh not installed — fall back to a cached value
  LOGIN=$(git config github.user 2>/dev/null)
fi
if [[ -z "$LOGIN" ]]; then
  echo "ERROR: Cannot resolve GitHub username."
  echo "  Option A (recommended): install gh CLI and run: gh auth login"
  echo "  Option B: set it manually: git config --global github.user <your-github-login>"
  exit 1
fi
```
`gh api user` returns the exact GitHub org username (e.g. `jsmith-amd`, not `jsmith`).
If `gh` is not installed, cache your username once with `git config --global github.user <your-github-login>`.
**Never infer the username from git config `user.name` or email — the org login may differ.**

**Step 2 — Construct the branch name:**
```
user/${LOGIN}/short-description
```
Include a ticket ID if one exists (e.g. `user/${LOGIN}/fix-rdma-timeout` or `user/${LOGIN}/JIRA-1234-fix-rdma`).
Description must be **ASCII only** — no Unicode, emoji, or special characters. Use hyphens, underscores, or dots as separators. Do not start the description with a dot (`.`) — Git rejects branch names with a leading dot.

**Step 3 — Validate the name matches one of the allowed prefixes:**
- Personal: `^user/[a-zA-Z0-9_.-]+/[a-zA-Z0-9_.-]+$`
- Team: `^team/[a-zA-Z0-9_.-]+/[a-zA-Z0-9_.-]+$`
- Bot: `^bot/[a-zA-Z0-9_.-]+/[a-zA-Z0-9_.-]+$`
- Collab: `^collab/` or `^collab-`
- Auto: `^revert-`, `^copilot/`, `^dependabot/`

Never skip Step 1. A missing or wrong login creates an invalid branch name silently.

---

## Branch Naming Convention

See `AGENTS.md` for the full branch naming reference — format, allowed prefixes, create/push steps.
`AGENTS.md` is the shared baseline read by all AI tools (Claude Code, Copilot, Codex, Gemini).

The short version: `user/<github-login>/<description>` — always resolve login via:
```bash
LOGIN=$(gh api user --jq .login)
```
Never infer from git config or email. A wrong login creates an invalid branch name silently.

---

## Fetch Setup (run once per clone)

With many branches in the repo, `git fetch` can be slow. Scope it to only what you need:

```bash
# Resolve GitHub username (see Step 1 above)
if command -v gh &>/dev/null; then
  LOGIN=$(gh api user --jq .login 2>/dev/null)
else
  LOGIN=$(git config github.user 2>/dev/null)
fi

# Resolve default (trunk) branch
if command -v gh &>/dev/null; then
  DEFAULT=$(gh api repos/pensando/$(basename $(git rev-parse --show-toplevel)) --jq .default_branch 2>/dev/null)
fi
if [[ -z "$DEFAULT" ]]; then
  # Fallback: detect from remote HEAD or ask git
  DEFAULT=$(git remote show origin 2>/dev/null | awk '/HEAD branch/{print $NF}')
fi
DEFAULT=${DEFAULT:-master}  # last resort default

git config --unset-all remote.origin.fetch
git config --add remote.origin.fetch "+refs/heads/${DEFAULT}:refs/remotes/origin/${DEFAULT}"
git config --add remote.origin.fetch '+refs/heads/team/*:refs/remotes/origin/team/*'
git config --add remote.origin.fetch "+refs/heads/user/${LOGIN}/*:refs/remotes/origin/user/${LOGIN}/*"
git config --add remote.origin.fetch '^refs/heads/user/*'
```

This pulls only the default branch, `team/*`, and your own `user/<login>/*` branches.

To fetch a colleague's branch explicitly:
```bash
git fetch origin user/<colleague-login>/branch-name
```

---

## Opening a PR

```bash
# After pushing your branch:
gh pr create \
  --head "user/${LOGIN}/short-description" \
  --title "Short description of change" \
  --body "Description of change"
```

For code review: anyone with write access can push directly to your `user/<login>/*`
branch — you don't need to create a new PR if someone wants to fix a CI failure or
add a small change.

---

## Common Scenarios — How to Help

**Scenario: Developer tries to create `feat/fix-something`**
→ Say: "That branch name will be blocked. Use `user/<login>/fix-something` instead.
  Run: `git checkout -b user/$(gh api user --jq .login)/fix-something origin/<default-branch>`"

**Scenario: Developer asks how to collaborate on someone's branch**
→ Say: "User branches are open — anyone with write access can push.
  Fetch it: `git fetch origin user/<login>/branch-name`
  Check out: `git checkout -b local-fix origin/user/<login>/branch-name`
  Push back: `git push origin HEAD:user/<login>/branch-name`"

**Scenario: Developer's branch was auto-deleted after PR merged**
→ Say: "Branches are auto-deleted when PRs merge. For follow-up work:
  `git checkout -b user/<login>/followup-work origin/<default-branch>`"

**Scenario: Developer asks about deleting their own branch**
→ Say: "You can delete your own branches via the mergers team or the self-service
  deletion tool (coming soon). After PR merge, branches are auto-deleted."

**Scenario: Developer asks why `git fetch` is slow**
→ Say: "Configure scoped fetch using the commands in the Fetch Setup section above.
  This reduces fetch to only the branches you need."

**Scenario: Developer is writing CI automation / a bot**
→ Say: "Use the `bot/<your-bot-account>/<description>` prefix.
  Example: `bot/ci-bot/fix-pr4521`
  This is the designated namespace for automated branches."

**Scenario: Developer's branch was accidentally deleted (not via merge)**
→ Say: "GitHub keeps deleted branch data — go to the repo's branch list or a related PR
  and click 'Restore branch'. There is no official time limit for restoration."

## Further Reading

For the full ruleset design, bypass policies, and org-wide enforcement details, refer to the internal Confluence page:
[GitHub Branch Protection Rulesets](https://amd.atlassian.net/wiki/spaces/EN/pages/1810205147/GitHub+Branch+Protection+Rulesets)