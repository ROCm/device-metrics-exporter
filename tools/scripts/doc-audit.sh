#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(git rev-parse --show-toplevel 2>/dev/null || true)"
if [[ -z "$REPO_ROOT" ]]; then
  REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
fi

YAML="$REPO_ROOT/docs/configuration/metrics-support-matrix.yaml"
MD="$REPO_ROOT/docs/configuration/metricslist.md"

for f in "$YAML" "$MD"; do
  if [[ ! -f "$f" ]]; then
    echo "ERROR: file not found: $f" >&2
    exit 2
  fi
done

errors=0
warnings=0
checked=0

# Parse YAML: extract name, baremetal, hypervisor_host per metric.
# Structural sanity checks per entry (no yq/python dependency).
declare -A yaml_bm yaml_hv
yaml_names=()
current_name=""
struct_errors=0
lineno=0

# Required fields we track per entry
declare -A seen_bm seen_hv seen_class

while IFS= read -r line; do
  lineno=$((lineno + 1))
  stripped="${line#"${line%%[![:space:]]*}"}"

  # Detect unbalanced brackets on any value line
  if [[ "$stripped" == *":"* ]]; then
    val_part="${stripped#*: }"
    opens="${val_part//[^\[]/}"
    closes="${val_part//[^\]]/}"
    if [[ ${#opens} -ne ${#closes} ]]; then
      echo "ERROR: malformed YAML entry near ${current_name:-line $lineno}: unbalanced brackets in '$stripped'" >&2
      struct_errors=$((struct_errors + 1))
    fi
  fi

  if [[ "$stripped" == "- name:"* ]]; then
    # Validate previous entry before starting new one
    if [[ -n "$current_name" ]]; then
      [[ -z "${seen_bm[$current_name]+x}" ]] && { echo "ERROR: malformed YAML entry near $current_name: missing 'baremetal' field" >&2; struct_errors=$((struct_errors + 1)); }
      [[ -z "${seen_hv[$current_name]+x}" ]] && { echo "ERROR: malformed YAML entry near $current_name: missing 'hypervisor_host' field" >&2; struct_errors=$((struct_errors + 1)); }
      [[ -z "${seen_class[$current_name]+x}" ]] && { echo "ERROR: malformed YAML entry near $current_name: missing 'class' field" >&2; struct_errors=$((struct_errors + 1)); }
    fi

    current_name="${stripped#*: }"
    current_name="${current_name%\"}"
    current_name="${current_name#\"}"

    if ! [[ "$current_name" =~ ^(GPU|PCIE)_[A-Z0-9_]+$ ]]; then
      echo "ERROR: malformed YAML entry near line $lineno: name '$current_name' is not a valid metric identifier" >&2
      struct_errors=$((struct_errors + 1))
    fi
    yaml_names+=("$current_name")
  elif [[ -n "$current_name" ]]; then
    if [[ "$stripped" == "baremetal:"* ]]; then
      val="${stripped#*: }"
      if [[ "$val" != "true" && "$val" != "false" ]]; then
        echo "ERROR: malformed YAML entry near $current_name: baremetal='$val' (expected true/false)" >&2
        struct_errors=$((struct_errors + 1))
      fi
      yaml_bm["$current_name"]="$val"
      seen_bm["$current_name"]=1
    elif [[ "$stripped" == "hypervisor_host:"* ]]; then
      val="${stripped#*: }"
      if [[ "$val" != "true" && "$val" != "false" ]]; then
        echo "ERROR: malformed YAML entry near $current_name: hypervisor_host='$val' (expected true/false)" >&2
        struct_errors=$((struct_errors + 1))
      fi
      yaml_hv["$current_name"]="$val"
      seen_hv["$current_name"]=1
    elif [[ "$stripped" == "class:"* ]]; then
      val="${stripped#*: }"
      if [[ "$val" != "C" && "$val" != "D" && "$val" != "N" ]]; then
        echo "ERROR: malformed YAML entry near $current_name: class='$val' (expected C/D/N)" >&2
        struct_errors=$((struct_errors + 1))
      fi
      seen_class["$current_name"]=1
    fi
  fi
done < "$YAML"

# Validate last entry
if [[ -n "$current_name" ]]; then
  [[ -z "${seen_bm[$current_name]+x}" ]] && { echo "ERROR: malformed YAML entry near $current_name: missing 'baremetal' field" >&2; struct_errors=$((struct_errors + 1)); }
  [[ -z "${seen_hv[$current_name]+x}" ]] && { echo "ERROR: malformed YAML entry near $current_name: missing 'hypervisor_host' field" >&2; struct_errors=$((struct_errors + 1)); }
  [[ -z "${seen_class[$current_name]+x}" ]] && { echo "ERROR: malformed YAML entry near $current_name: missing 'class' field" >&2; struct_errors=$((struct_errors + 1)); }
fi

if [[ $struct_errors -gt 0 ]]; then
  echo "ERROR: $struct_errors structural error(s) in $YAML" >&2
  exit 3
fi

if [[ ${#yaml_names[@]} -eq 0 ]]; then
  echo "ERROR: failed to parse any metrics from $YAML" >&2
  exit 2
fi

# Parse metricslist.md table rows.
# Table format: | Hypervisor | Baremetal | Metric | Description |
# Also handle profiler-only tables (no Hypervisor/Baremetal columns) -- skip those.
declare -A md_bm md_hv
md_names=()

while IFS= read -r line; do
  # Only process lines that look like table rows with pipes
  [[ "$line" != *"|"* ]] && continue
  # Skip separator rows
  [[ "$line" =~ ^[[:space:]]*\|[[:space:]]*[-:]+[[:space:]]*\| ]] && continue
  # Skip header rows (contain "Metric" as a header word)
  [[ "$line" =~ \|[[:space:]]*Metric[[:space:]]*\| ]] && continue

  # Count columns (number of pipe-delimited cells)
  col_count=$(echo "$line" | awk -F'|' '{print NF - 1}')
  # We only care about 4-column tables (Hypervisor | Baremetal | Metric | Description)
  [[ "$col_count" -lt 4 ]] && continue

  # Extract cells
  hyp_cell=$(echo "$line" | awk -F'|' '{print $2}')
  bm_cell=$(echo "$line" | awk -F'|' '{print $3}')
  metric_cell=$(echo "$line" | awk -F'|' '{print $4}')

  # Extract metric name: strip backtick annotations, whitespace, and trailing [platform] tags
  metric_name=$(echo "$metric_cell" | sed 's/`//g' | sed 's/\[.*\]//g' | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')
  [[ -z "$metric_name" ]] && continue

  # Convert &check;/&cross; (and &#x2713;/&#x2717; variants) to true/false
  hyp_val="false"
  if echo "$hyp_cell" | grep -qE '&check;|&#x2713;|✓'; then
    hyp_val="true"
  fi
  bm_val="false"
  if echo "$bm_cell" | grep -qE '&check;|&#x2713;|✓'; then
    bm_val="true"
  fi

  md_names+=("$metric_name")
  md_bm["$metric_name"]="$bm_val"
  md_hv["$metric_name"]="$hyp_val"
done < "$MD"

# Compare metrics present in both
for name in "${yaml_names[@]}"; do
  if [[ -z "${md_bm[$name]+x}" ]]; then
    echo "WARNING: $name in YAML but not in metricslist.md"
    warnings=$((warnings + 1))
    continue
  fi
  checked=$((checked + 1))

  y_bm="${yaml_bm[$name]}"
  y_hv="${yaml_hv[$name]}"
  m_bm="${md_bm[$name]}"
  m_hv="${md_hv[$name]}"

  if [[ "$y_bm" != "$m_bm" ]]; then
    echo "ERROR: $name baremetal mismatch: YAML=$y_bm md=$m_bm"
    errors=$((errors + 1))
  fi
  if [[ "$y_hv" != "$m_hv" ]]; then
    echo "ERROR: $name hypervisor mismatch: YAML=$y_hv md=$m_hv"
    errors=$((errors + 1))
  fi
done

# Check for metrics in md but not in YAML
declare -A yaml_set
for name in "${yaml_names[@]}"; do yaml_set["$name"]=1; done
for name in "${md_names[@]}"; do
  if [[ -z "${yaml_set[$name]+x}" ]]; then
    echo "WARNING: $name in metricslist.md but not in YAML"
    warnings=$((warnings + 1))
  fi
done

echo ""
echo "doc-audit: checked=$checked errors=$errors warnings=$warnings"
if [[ $errors -gt 0 ]]; then
  exit 1
fi
exit 0
