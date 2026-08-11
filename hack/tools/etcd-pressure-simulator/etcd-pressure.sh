#!/bin/bash
#
# etcd-pressure-simulator — Real etcd pressure simulator with genuine gradual drain.
#
# Uses predict-before-defrag: reads dbSizeInUse to forecast post-defrag size
# before actually defragmenting, and rolls back if the prediction would cross
# the HOLD safety floor.
#
# Usage:
#   ./etcd-pressure.sh status
#   ./etcd-pressure.sh fill <target_percent>
#   ./etcd-pressure.sh drain <target_percent>
#   ./etcd-pressure.sh cleanup
#

set -euo pipefail

# ---------------------------------------------------------------------------
# Configuration
# ---------------------------------------------------------------------------

QUOTA_BYTES="${QUOTA_BYTES:-268435456}"
NAMESPACE="etcd-pressure-simulator"
LABEL="app=etcd-pressure-simulator"
HOLD_SAFETY_FLOOR="${HOLD_SAFETY_FLOOR:-81.0}"
PROMETHEUS_SVC="svc/prometheus-kube-prometheus-prometheus"
PROMETHEUS_NS="prometheus"
LOCAL_PORT="${PROMETHEUS_LOCAL_PORT:-19091}"
PROMETHEUS_URL="http://localhost:${LOCAL_PORT}"
SCRAPE_INTERVAL=15
DEFRAG_INTERVAL=200
CLUSTER_NAME="${CLUSTER_NAME:-etcd-shield-test}"

PF_PID=""
BACKUP_DIR=""
PAYLOAD_FILES=()

cleanup() {
    [[ -n "$PF_PID" ]] && kill "$PF_PID" 2>/dev/null || true
    [[ -n "$BACKUP_DIR" ]] && rm -rf "$BACKUP_DIR" 2>/dev/null || true
    for f in "${PAYLOAD_FILES[@]}"; do
        rm -f "$f" 2>/dev/null || true
    done
}
trap cleanup EXIT

# ---------------------------------------------------------------------------
# Helpers: port-forward & Prometheus
# ---------------------------------------------------------------------------

start_port_forward() {
    if [[ -n "$PF_PID" ]] && kill -0 "$PF_PID" 2>/dev/null; then
        return 0
    fi

    kubectl port-forward -n "$PROMETHEUS_NS" "$PROMETHEUS_SVC" "${LOCAL_PORT}:9090" >/dev/null 2>&1 &
    PF_PID=$!

    local i
    for i in $(seq 1 30); do
        if curl -sf "${PROMETHEUS_URL}/-/ready" >/dev/null 2>&1; then
            return 0
        fi
        sleep 1
    done
    echo "ERROR: Prometheus port-forward failed to become ready." >&2
    exit 1
}

get_etcd_size() {
    local result
    result=$(curl -sf "${PROMETHEUS_URL}/api/v1/query" \
        --data-urlencode 'query=max(etcd_mvcc_db_total_size_in_bytes)' \
        | jq -r '.data.result[0].value[1] // "0"')
    echo "${result%%.*}"
}

get_sample_timestamp() {
    curl -sf "${PROMETHEUS_URL}/api/v1/query" \
        --data-urlencode 'query=max(timestamp(etcd_mvcc_db_total_size_in_bytes))' \
        | jq -r '.data.result[0].value[1] // "0"'
}

wait_for_fresh_sample() {
    local old_ts=$1
    local i new_ts
    for i in $(seq 1 40); do
        new_ts=$(get_sample_timestamp)
        if [[ "$new_ts" != "$old_ts" ]]; then
            return 0
        fi
        sleep 1
    done
    echo "ERROR: Prometheus did not receive a fresh etcd sample." >&2
    return 1
}

calc_usage() {
    local size=$1
    awk "BEGIN { printf \"%.2f\", ${size} / ${QUOTA_BYTES} * 100 }"
}

usage_ge() {
    local usage=$1 target=$2
    awk "BEGIN { exit !($usage >= $target) }"
}

usage_le() {
    local usage=$1 target=$2
    awk "BEGIN { exit !($usage <= $target) }"
}

float_min() {
    local a=$1 b=$2
    awk "BEGIN { print ($a < $b) ? $a : $b }"
}

check_cluster() {
    if ! kubectl cluster-info >/dev/null 2>&1; then
        echo "ERROR: Cannot reach Kubernetes cluster." >&2
        exit 1
    fi
}

check_kind_context() {
    local ctx
    ctx=$(kubectl config current-context 2>/dev/null || true)
    if [[ "$ctx" != "kind-${CLUSTER_NAME}" ]]; then
        echo "ERROR: Current context '${ctx}' does not match CLUSTER_NAME '${CLUSTER_NAME}'." >&2
        echo "Expected context: kind-${CLUSTER_NAME}" >&2
        echo "This tool is designed for local KinD clusters only." >&2
        exit 1
    fi
}

check_tools() {
    local missing=()
    for tool in kubectl curl jq awk; do
        command -v "$tool" &>/dev/null || missing+=("$tool")
    done
    if ! command -v docker &>/dev/null && ! command -v podman &>/dev/null; then
        missing+=("docker or podman")
    fi
    if [[ ${#missing[@]} -gt 0 ]]; then
        echo "ERROR: Missing required tools: ${missing[*]}" >&2
        exit 1
    fi
}

check_metric() {
    local size i
    for i in $(seq 1 20); do
        size=$(get_etcd_size)
        if [[ "$size" -gt 0 ]] 2>/dev/null; then
            return 0
        fi
        if [[ "$i" -eq 1 ]]; then
            echo "Waiting for etcd metric to appear in Prometheus..."
        fi
        sleep "$SCRAPE_INTERVAL"
    done
    echo "ERROR: Prometheus does not expose etcd_mvcc_db_total_size_in_bytes." >&2
    echo "Verify that kubeEtcd scraping is enabled." >&2
    exit 1
}

validate_target() {
    local target=$1 cmd=$2
    if [[ -z "$target" ]]; then
        echo "Usage: $0 ${cmd} <target_percent>" >&2
        return 1
    fi
    if ! [[ "$target" =~ ^[0-9]+$ ]]; then
        echo "ERROR: Target must be a positive integer." >&2
        return 1
    fi
    if [[ "$target" -ge 100 ]]; then
        echo "ERROR: Target must be less than 100%." >&2
        return 1
    fi
    if [[ "$target" -le 0 ]]; then
        echo "ERROR: Target must be greater than 0%." >&2
        return 1
    fi
}

refresh_usage() {
    local ts_before
    ts_before=$(get_sample_timestamp)
    wait_for_fresh_sample "$ts_before"
    CURRENT_SIZE=$(get_etcd_size)
    CURRENT_USAGE=$(calc_usage "$CURRENT_SIZE")
}

# ---------------------------------------------------------------------------
# Helpers: etcdctl
# ---------------------------------------------------------------------------

container_runtime() {
    if command -v docker &>/dev/null; then
        echo docker
    elif command -v podman &>/dev/null; then
        echo podman
    else
        echo "ERROR: neither docker nor podman found." >&2
        exit 1
    fi
}

run_etcdctl() {
    local rt
    rt=$(container_runtime)
    local container_id
    container_id=$("$rt" exec "${CLUSTER_NAME}-control-plane" \
        crictl ps --name='^etcd$' -o json | jq -r '.containers[0].id')
    "$rt" exec "${CLUSTER_NAME}-control-plane" \
        crictl exec "$container_id" \
        etcdctl \
        --endpoints=https://127.0.0.1:2379 \
        --cacert=/etc/kubernetes/pki/etcd/ca.crt \
        --cert=/etc/kubernetes/pki/etcd/healthcheck-client.crt \
        --key=/etc/kubernetes/pki/etcd/healthcheck-client.key \
        "$@" 2>&1
}

get_etcd_sizes() {
    local json
    json=$(run_etcdctl endpoint status --write-out=json)
    local db_size db_size_in_use
    db_size=$(echo "$json" | jq -r '.[0].Status.dbSize')
    db_size_in_use=$(echo "$json" | jq -r '.[0].Status.dbSizeInUse')
    echo "${db_size} ${db_size_in_use}"
}

get_etcd_revision() {
    run_etcdctl endpoint status --write-out=json \
        | jq -r '.[0].Status.header.revision'
}

etcd_compact() {
    local revision
    revision=$(get_etcd_revision)
    echo "Compacting etcd to revision ${revision}..."
    run_etcdctl compact "$revision" >/dev/null 2>&1 || true
}

etcd_defrag() {
    echo "Defragmenting etcd..."
    run_etcdctl defrag >/dev/null 2>&1
}

etcd_disarm_alarm() {
    local output
    output=$(run_etcdctl alarm list 2>&1)
    if echo "$output" | grep -qi "alarm"; then
        echo "Clearing etcd alarms..."
        run_etcdctl alarm disarm >/dev/null 2>&1
    fi
}

# ---------------------------------------------------------------------------
# Helpers: payload generation
# ---------------------------------------------------------------------------

generate_payload() {
    local size_kb=$1
    local payload_file
    payload_file=$(mktemp)
    PAYLOAD_FILES+=("$payload_file")
    dd if=/dev/urandom of="$payload_file" bs=1024 count="$size_kb" 2>/dev/null
    echo "$payload_file"
}

PAYLOAD_LARGE=""
PAYLOAD_MEDIUM=""
PAYLOAD_SMALL=""
PAYLOAD_LARGE_B64=""
PAYLOAD_MEDIUM_B64=""
PAYLOAD_SMALL_B64=""

ensure_payloads() {
    if [[ -z "$PAYLOAD_LARGE" ]]; then
        PAYLOAD_LARGE=$(generate_payload 512)
        PAYLOAD_LARGE_B64=$(base64 < "$PAYLOAD_LARGE" | tr -d '\n')
    fi
    if [[ -z "$PAYLOAD_MEDIUM" ]]; then
        PAYLOAD_MEDIUM=$(generate_payload 128)
        PAYLOAD_MEDIUM_B64=$(base64 < "$PAYLOAD_MEDIUM" | tr -d '\n')
    fi
    if [[ -z "$PAYLOAD_SMALL" ]]; then
        PAYLOAD_SMALL=$(generate_payload 32)
        PAYLOAD_SMALL_B64=$(base64 < "$PAYLOAD_SMALL" | tr -d '\n')
    fi
}

# ---------------------------------------------------------------------------
# Helpers: ConfigMap management
# ---------------------------------------------------------------------------

next_sequence() {
    local max_seq
    max_seq=$(kubectl get configmaps -n "$NAMESPACE" -l "$LABEL" \
        -o jsonpath='{range .items[*]}{.metadata.annotations.etcd-pressure/sequence}{"\n"}{end}' \
        2>/dev/null | sort -n | tail -1)
    if [[ -z "$max_seq" ]]; then
        echo 0
    else
        echo $(( 10#$max_seq + 1 ))
    fi
}

create_configmap() {
    local seq=$1 size_class=$2
    local payload_b64 size_kb
    case "$size_class" in
        large)  payload_b64="$PAYLOAD_LARGE_B64"; size_kb=512 ;;
        medium) payload_b64="$PAYLOAD_MEDIUM_B64"; size_kb=128 ;;
        small)  payload_b64="$PAYLOAD_SMALL_B64"; size_kb=32 ;;
    esac

    local name="pressure-${size_class}-$(printf '%05d' "$seq")"

    cat <<EOF | kubectl create -f - >/dev/null 2>&1
{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"${name}","namespace":"${NAMESPACE}","labels":{"app":"etcd-pressure-simulator","pressure-size-class":"${size_class}"},"annotations":{"etcd-pressure/size-kb":"${size_kb}","etcd-pressure/sequence":"${seq}"}},"binaryData":{"payload":"${payload_b64}"}}
EOF
}

save_clean_manifest() {
    local cm_name=$1 output_dir=$2
    kubectl get configmap "$cm_name" -n "$NAMESPACE" -o json \
        | jq '{
            apiVersion: .apiVersion,
            kind: .kind,
            metadata: {
                name: .metadata.name,
                namespace: .metadata.namespace,
                labels: .metadata.labels,
                annotations: (
                    .metadata.annotations
                    | with_entries(
                        select(.key | startswith("etcd-pressure/") or startswith("pressure-"))
                    )
                )
            },
            data: .data,
            binaryData: .binaryData,
            immutable: .immutable
        } | with_entries(select(.value != null))' \
        > "${output_dir}/${cm_name}.json"
}

restore_configmaps() {
    local manifest_dir=$1
    local failed=0

    for manifest in "${manifest_dir}"/*.json; do
        [[ -f "$manifest" ]] || continue
        local name
        name=$(jq -r '.metadata.name' "$manifest")

        if ! kubectl apply -f "$manifest" -n "$NAMESPACE" >/dev/null 2>&1; then
            echo "ERROR: Failed to restore ConfigMap ${name}" >&2
            failed=1
            continue
        fi

        if ! kubectl get configmap "$name" -n "$NAMESPACE" >/dev/null 2>&1; then
            echo "ERROR: ConfigMap ${name} not found after restore" >&2
            failed=1
        fi
    done

    return $failed
}

list_configmaps_by_size() {
    local size_class=$1 order=$2
    local sort_flag="-n"
    if [[ "$order" == "desc" ]]; then
        sort_flag="-rn"
    fi
    kubectl get configmaps -n "$NAMESPACE" \
        -l "${LABEL},pressure-size-class=${size_class}" \
        -o jsonpath='{range .items[*]}{.metadata.name}{"\n"}{end}' 2>/dev/null \
        | sort $sort_flag
}

select_cms_to_delete() {
    local distance=$1 is_hold=$2 max_cms=${3:-0}
    local cms=()

    local large_red medium_red small_red
    large_red=$(awk "BEGIN { printf \"%.4f\", 512 * 1024 / $QUOTA_BYTES * 100 }")
    medium_red=$(awk "BEGIN { printf \"%.4f\", 128 * 1024 / $QUOTA_BYTES * 100 }")
    small_red=$(awk "BEGIN { printf \"%.4f\", 32 * 1024 / $QUOTA_BYTES * 100 }")

    local target_red
    if [[ "$is_hold" == "true" ]]; then
        if awk "BEGIN { exit !($distance > 7) }"; then
            target_red=0.6
        elif awk "BEGIN { exit !($distance > 3) }"; then
            target_red=0.3
        elif awk "BEGIN { exit !($distance > 1) }"; then
            target_red=0.15
        else
            target_red=0.05
        fi
    else
        if awk "BEGIN { exit !($distance > 7) }"; then
            target_red=1.0
        elif awk "BEGIN { exit !($distance > 3) }"; then
            target_red=0.5
        elif awk "BEGIN { exit !($distance > 1) }"; then
            target_red=0.3
        else
            target_red=0.1
        fi
    fi

    local large_list medium_list small_list
    large_list=$(list_configmaps_by_size "large" "desc")
    medium_list=$(list_configmaps_by_size "medium" "desc")
    small_list=$(list_configmaps_by_size "small" "desc")

    local n_large n_medium n_small
    n_large=$(echo "$large_list" | grep -c . 2>/dev/null || echo 0)
    n_medium=$(echo "$medium_list" | grep -c . 2>/dev/null || echo 0)
    n_small=$(echo "$small_list" | grep -c . 2>/dev/null || echo 0)

    local remaining="$target_red"
    local take_large=0 take_medium=0 take_small=0

    if [[ "$n_large" -gt 0 ]]; then
        take_large=$(awk "BEGIN {
            v = int($remaining / $large_red)
            if (v > $n_large) v = $n_large
            print v
        }")
        remaining=$(awk "BEGIN { printf \"%.4f\", $remaining - ($take_large * $large_red) }")
    fi

    if [[ "$n_medium" -gt 0 ]] && awk "BEGIN { exit !($remaining > $medium_red * 0.5) }"; then
        take_medium=$(awk "BEGIN {
            v = int($remaining / $medium_red + 0.5)
            if (v > $n_medium) v = $n_medium
            if (v < 1) v = 1
            print v
        }")
    fi

    if [[ "$take_large" -eq 0 ]] && [[ "$take_medium" -eq 0 ]] && [[ "$n_small" -gt 0 ]]; then
        take_small=$(awk "BEGIN {
            v = int($remaining / $small_red + 0.5)
            if (v > $n_small) v = $n_small
            if (v < 1) v = 1
            print v
        }")
    fi

    if [[ "$take_large" -eq 0 ]] && [[ "$take_medium" -eq 0 ]] && [[ "$take_small" -eq 0 ]]; then
        if [[ "$n_medium" -gt 0 ]]; then
            take_medium=1
        elif [[ "$n_large" -gt 0 ]]; then
            take_large=1
        elif [[ "$n_small" -gt 0 ]]; then
            take_small=1
        fi
    fi

    if [[ "$max_cms" -gt 0 ]]; then
        local total=$((take_large + take_medium + take_small))
        while [[ "$total" -gt "$max_cms" ]]; do
            if [[ "$take_large" -gt 0 ]]; then
                take_large=$((take_large - 1))
            elif [[ "$take_medium" -gt 0 ]]; then
                take_medium=$((take_medium - 1))
            elif [[ "$take_small" -gt 0 ]]; then
                take_small=$((take_small - 1))
            else
                break
            fi
            total=$((take_large + take_medium + take_small))
        done
        if [[ "$total" -eq 0 ]]; then
            if [[ "$n_small" -gt 0 ]]; then
                take_small=1
            elif [[ "$n_medium" -gt 0 ]]; then
                take_medium=1
            elif [[ "$n_large" -gt 0 ]]; then
                take_large=1
            fi
        fi
    fi

    if [[ "$take_large" -gt 0 ]]; then
        while IFS= read -r cm; do
            [[ -z "$cm" ]] && continue
            cms+=("$cm")
        done <<< "$(echo "$large_list" | head -"$take_large")"
    fi
    if [[ "$take_medium" -gt 0 ]]; then
        while IFS= read -r cm; do
            [[ -z "$cm" ]] && continue
            cms+=("$cm")
        done <<< "$(echo "$medium_list" | head -"$take_medium")"
    fi
    if [[ "$take_small" -gt 0 ]]; then
        while IFS= read -r cm; do
            [[ -z "$cm" ]] && continue
            cms+=("$cm")
        done <<< "$(echo "$small_list" | head -"$take_small")"
    fi

    if [[ ${#cms[@]} -eq 0 ]]; then
        local any_cm
        any_cm=$(kubectl get configmaps -n "$NAMESPACE" -l "$LABEL" \
            -o jsonpath='{.items[0].metadata.name}' 2>/dev/null)
        if [[ -n "$any_cm" ]]; then
            cms+=("$any_cm")
        fi
    fi

    printf '%s\n' "${cms[@]}"
}

# ---------------------------------------------------------------------------
# Commands
# ---------------------------------------------------------------------------

cmd_status() {
    check_tools
    check_kind_context
    check_cluster
    start_port_forward
    check_metric

    local size usage
    size=$(get_etcd_size)
    usage=$(calc_usage "$size")

    printf "etcd physical size: %'d bytes\n" "$size"
    printf "quota:              %'d bytes\n" "$QUOTA_BYTES"
    printf "physical usage:     %s%%\n" "$usage"

    local sizes db_size_in_use in_use_usage
    sizes=$(get_etcd_sizes 2>/dev/null || true)
    if [[ -n "$sizes" ]]; then
        db_size_in_use=$(echo "$sizes" | awk '{print $2}')
        if [[ -n "$db_size_in_use" ]] && [[ "$db_size_in_use" != "null" ]]; then
            in_use_usage=$(calc_usage "$db_size_in_use")
            printf "etcd in-use size:   %'d bytes (%s%%)\n" "$db_size_in_use" "$in_use_usage"
        fi
    fi

    local cm_count
    cm_count=$(kubectl get configmaps -n "$NAMESPACE" -l "$LABEL" \
        --no-headers 2>/dev/null | wc -l)
    printf "pressure CMs:       %d\n" "$cm_count"
}

cmd_fill() {
    local target=$1
    validate_target "$target" "fill" || return 1

    check_tools
    check_kind_context
    check_cluster
    start_port_forward
    check_metric

    local size usage
    size=$(get_etcd_size)
    usage=$(calc_usage "$size")

    local sizes_info db_size_physical db_size_in_use in_use_usage
    sizes_info=$(get_etcd_sizes 2>/dev/null || true)
    if [[ -n "$sizes_info" ]]; then
        db_size_physical=$(echo "$sizes_info" | awk '{print $1}')
        db_size_in_use=$(echo "$sizes_info" | awk '{print $2}')
        in_use_usage=$(calc_usage "$db_size_in_use")
        local gap
        gap=$(awk "BEGIN { printf \"%.0f\", $usage - $in_use_usage }")
        if [[ "$gap" -gt 5 ]] || usage_ge "$usage" "100"; then
            printf "Current size: %'d bytes\n" "$size"
            printf "Physical (%s%%) >> in-use (%s%%). Defragmenting first...\n" "$usage" "$in_use_usage"
            etcd_compact
            etcd_defrag
            etcd_disarm_alarm
            refresh_usage
            size=$CURRENT_SIZE
            usage=$CURRENT_USAGE
            printf "After defrag: %s%%\n" "$usage"
        fi
    fi

    printf "Current size: %'d bytes\n" "$size"
    echo "Current usage: ${usage}%"
    echo "Target usage: ${target}%"
    echo ""

    if usage_ge "$usage" "$target"; then
        echo "Already at or above target. Nothing to do."
        return 0
    fi

    kubectl create namespace "$NAMESPACE" --dry-run=client -o yaml | kubectl apply -f - >/dev/null 2>&1
    ensure_payloads

    local round=0
    while [[ "$round" -lt 20 ]]; do
        round=$((round + 1))
        local seq
        seq=$(next_sequence)
        local cms_at_last_defrag=$seq

        if [[ "$round" -eq 1 ]]; then
            echo "Creating etcd pressure resources..."
        else
            echo "Continuing fill (round ${round})..."
        fi
        echo ""

        while true; do
            local ts_before
            ts_before=$(get_sample_timestamp)

            size=$(get_etcd_size)
            usage=$(calc_usage "$size")

            if usage_ge "$usage" "$target"; then
                break
            fi

            local distance
            distance=$(awk "BEGIN { printf \"%.0f\", $target - $usage }")

            local size_class batch_size
            if [[ "$distance" -gt 15 ]]; then
                size_class="large"; batch_size=10
            elif [[ "$distance" -gt 5 ]]; then
                size_class="medium"; batch_size=5
            else
                size_class="medium"; batch_size=10
            fi

            local i
            for i in $(seq 1 "$batch_size"); do
                if ! create_configmap "$seq" "$size_class"; then
                    echo ""
                    echo "WARNING: ConfigMap creation failed (etcd may have hit quota)."
                    echo "Running compaction and defragmentation..."
                    etcd_compact
                    etcd_defrag
                    etcd_disarm_alarm
                    cms_at_last_defrag=$seq
                    refresh_usage
                    size=$CURRENT_SIZE
                    usage=$CURRENT_USAGE
                    break
                fi
                seq=$((seq + 1))
            done

            if [[ $((seq - cms_at_last_defrag)) -ge $DEFRAG_INTERVAL ]]; then
                printf "Periodic defrag (%d CMs since last)...\n" $((seq - cms_at_last_defrag))
                etcd_compact
                etcd_defrag
                etcd_disarm_alarm
                cms_at_last_defrag=$seq
            fi

            wait_for_fresh_sample "$ts_before"

            size=$(get_etcd_size)
            usage=$(calc_usage "$size")
            local cm_count
            cm_count=$(kubectl get configmaps -n "$NAMESPACE" -l "$LABEL" \
                --no-headers 2>/dev/null | wc -l)
            printf "ConfigMaps: %-6d  usage: %s%% [%s batch]\n" "$cm_count" "$usage" "$size_class"
        done

        echo ""
        echo "Consolidation defrag..."
        etcd_compact
        etcd_defrag
        etcd_disarm_alarm
        refresh_usage
        size=$CURRENT_SIZE
        usage=$CURRENT_USAGE
        printf "After consolidation: %s%%\n" "$usage"

        if usage_ge "$usage" "$target"; then
            break
        fi
    done

    if ! usage_ge "$usage" "$target"; then
        echo "ERROR: Could not reach target after ${round} rounds." >&2
        printf "Current usage: %s%%\n" "$usage" >&2
        return 1
    fi

    echo ""
    echo "Fill completed successfully."
    printf "Prometheus reports etcd usage at %s%%.\n" "$usage"
    return 0
}

cmd_drain() {
    local target=$1
    validate_target "$target" "drain" || return 1

    check_tools
    check_kind_context
    check_cluster
    start_port_forward
    check_metric

    local size usage
    size=$(get_etcd_size)
    usage=$(calc_usage "$size")

    local is_hold="false"
    local safety_floor="0"
    if [[ "$target" -ge 80 ]]; then
        is_hold="true"
        safety_floor="$HOLD_SAFETY_FLOOR"
    fi

    printf "Current size: %'d bytes\n" "$size"
    echo "Current usage: ${usage}%"
    echo "Target usage: <=${target}%"
    if [[ "$is_hold" == "true" ]]; then
        echo "Mode: HOLD (safety floor: ${safety_floor}%)"
    else
        echo "Mode: RESET (no safety floor)"
    fi
    echo ""

    if usage_le "$usage" "$target"; then
        echo "Already at or below target. Nothing to do."
        return 0
    fi

    local drain_sizes drain_in_use drain_in_use_pct
    drain_sizes=$(get_etcd_sizes 2>/dev/null || true)
    if [[ -n "$drain_sizes" ]]; then
        drain_in_use=$(echo "$drain_sizes" | awk '{print $2}')
        drain_in_use_pct=$(calc_usage "$drain_in_use")
        local drain_gap
        drain_gap=$(awk "BEGIN { printf \"%.0f\", $usage - $drain_in_use_pct }")
        if [[ "$drain_gap" -gt 5 ]] || usage_ge "$usage" "100"; then
            printf "Physical (%s%%) >> in-use (%s%%).\n" "$usage" "$drain_in_use_pct"

            if [[ "$is_hold" == "true" ]] && ! usage_ge "$drain_in_use_pct" "$safety_floor"; then
                echo "ERROR: In-use ${drain_in_use_pct}% is below safety floor ${safety_floor}%." >&2
                echo "Defrag would drop physical to ~${drain_in_use_pct}%, violating the floor." >&2
                echo "The etcd state does not have enough live data for a HOLD drain." >&2
                echo "Hint: re-run 'fill' — it now uses periodic defrag to keep physical ≈ in-use." >&2
                return 1
            fi

            echo "Compacting and defragmenting first..."
            etcd_compact
            etcd_defrag
            etcd_disarm_alarm
            refresh_usage
            size=$CURRENT_SIZE
            usage=$CURRENT_USAGE
            printf "After defrag: %s%%\n" "$usage"
            echo ""

            if usage_le "$usage" "$target"; then
                echo "Already at or below target after defrag."
                return 0
            fi
        fi
    fi

    local cm_count
    cm_count=$(kubectl get configmaps -n "$NAMESPACE" -l "$LABEL" \
        --no-headers 2>/dev/null | wc -l)
    if [[ "$cm_count" -eq 0 ]]; then
        echo "ERROR: No pressure ConfigMaps found to drain." >&2
        echo "Current etcd usage is ${usage}% but there are no CMs to delete." >&2
        return 1
    fi

    local min_observed="$usage"
    local iteration=0
    local retry_max=0

    while ! usage_le "$usage" "$target"; do
        iteration=$((iteration + 1))
        local distance
        distance=$(awk "BEGIN { printf \"%.2f\", $usage - $target }")
        local prev_usage="$usage"

        local cms_to_delete
        cms_to_delete=$(select_cms_to_delete "$distance" "$is_hold" "$retry_max")
        if [[ -z "$cms_to_delete" ]]; then
            echo "ERROR: No ConfigMaps available to delete." >&2
            printf "Current usage: %s%%, target: %s%%\n" "$usage" "$target" >&2
            return 1
        fi

        local delete_count=0
        local deleted_names=()

        BACKUP_DIR=$(mktemp -d)

        while IFS= read -r cm_name; do
            [[ -z "$cm_name" ]] && continue
            save_clean_manifest "$cm_name" "$BACKUP_DIR"
            deleted_names+=("$cm_name")
            delete_count=$((delete_count + 1))
        done <<< "$cms_to_delete"

        local size_class_info
        if [[ ${#deleted_names[@]} -gt 0 ]]; then
            size_class_info=$(kubectl get configmap "${deleted_names[0]}" -n "$NAMESPACE" \
                -o jsonpath='{.metadata.labels.pressure-size-class}' 2>/dev/null || echo "unknown")
        fi

        for cm_name in "${deleted_names[@]}"; do
            kubectl delete configmap "$cm_name" -n "$NAMESPACE" --wait=true >/dev/null 2>&1
        done

        for cm_name in "${deleted_names[@]}"; do
            if kubectl get configmap "$cm_name" -n "$NAMESPACE" >/dev/null 2>&1; then
                echo "ERROR: ConfigMap ${cm_name} still exists after delete." >&2
                return 1
            fi
        done

        etcd_compact

        local sizes db_size_in_use predicted_usage
        sizes=$(get_etcd_sizes)
        db_size_in_use=$(echo "$sizes" | awk '{print $2}')
        predicted_usage=$(calc_usage "$db_size_in_use")

        if [[ "$is_hold" == "true" ]] && ! usage_ge "$predicted_usage" "$safety_floor"; then
            echo ""
            printf "SAFETY: Predicted post-defrag usage %s%% would cross floor %s%%.\n" \
                "$predicted_usage" "$safety_floor"
            echo "Restoring ${delete_count} ConfigMap(s)..."

            etcd_disarm_alarm

            if ! restore_configmaps "$BACKUP_DIR"; then
                echo "CRITICAL: Restore failed. Manual intervention required." >&2
                return 1
            fi

            local post_restore_sizes post_restore_in_use
            post_restore_sizes=$(get_etcd_sizes)
            post_restore_in_use=$(echo "$post_restore_sizes" | awk '{print $2}')
            printf "Post-restore dbSizeInUse: %'d bytes\n" "$post_restore_in_use"

            rm -rf "$BACKUP_DIR"
            BACKUP_DIR=""

            if [[ "$delete_count" -eq 1 ]]; then
                echo ""
                echo "ERROR: Even deleting 1 smallest ConfigMap would cross the safety floor." >&2
                printf "  Current usage:    %s%%\n" "$usage" >&2
                printf "  Predicted:        %s%%\n" "$predicted_usage" >&2
                printf "  Safety floor:     %s%%\n" "$safety_floor" >&2
                printf "  Target:           %s%%\n" "$target" >&2
                echo "" >&2
                echo "The gap between the safety floor and current usage is too small" >&2
                echo "for the smallest available ConfigMap. Consider lowering HOLD_SAFETY_FLOOR." >&2
                return 1
            fi

            retry_max=$((delete_count / 2))
            if [[ "$retry_max" -lt 1 ]]; then retry_max=1; fi
            echo "Retrying with smaller batch (max ${retry_max})..."
            continue
        fi

        retry_max=0

        etcd_defrag
        etcd_disarm_alarm

        rm -rf "$BACKUP_DIR"
        BACKUP_DIR=""

        refresh_usage
        size=$CURRENT_SIZE
        usage=$CURRENT_USAGE

        min_observed=$(float_min "$min_observed" "$usage")

        if [[ "$is_hold" == "true" ]] && ! usage_ge "$usage" "$safety_floor"; then
            echo ""
            echo "CRITICAL: HOLD safety floor was crossed after defrag." >&2
            echo "" >&2
            printf "  Predicted usage:  %s%%\n" "$predicted_usage" >&2
            printf "  Observed usage:   %s%%\n" "$usage" >&2
            printf "  Safety floor:     %s%%\n" "$safety_floor" >&2
            printf "  Previous usage:   %s%%\n" "$prev_usage" >&2
            printf "  Deleted CMs:      %d (%s)\n" "$delete_count" "$size_class_info" >&2
            echo "" >&2
            echo "The observed post-defrag usage is lower than predicted." >&2
            echo "This should not happen. Do NOT attempt to fix by refilling." >&2
            return 1
        fi

        printf "Step %d: deleted %d (%s) → %s%% [predicted: %s%%]\n" \
            "$iteration" "$delete_count" "$size_class_info" "$usage" "$predicted_usage"
    done

    echo ""

    if [[ "$is_hold" == "true" ]]; then
        if ! usage_le "$usage" "$target"; then
            echo "ERROR: Final usage ${usage}% exceeds target ${target}%." >&2
            return 1
        fi
        if ! usage_ge "$usage" "$safety_floor"; then
            echo "ERROR: Final usage ${usage}% is below safety floor ${safety_floor}%." >&2
            return 1
        fi
        if ! usage_ge "$min_observed" "$safety_floor"; then
            echo "ERROR: Minimum observed ${min_observed}% crossed safety floor ${safety_floor}%." >&2
            return 1
        fi
    else
        if ! usage_le "$usage" "$target"; then
            echo "ERROR: Final usage ${usage}% exceeds target ${target}%." >&2
            return 1
        fi
    fi

    echo "Drain completed successfully."
    printf "Final usage:         %s%%\n" "$usage"
    printf "Minimum observed:    %s%%\n" "$min_observed"
    if [[ "$is_hold" == "true" ]]; then
        printf "Safety floor:        %s%%\n" "$safety_floor"
    else
        echo "Safety floor:        none"
    fi
}

cmd_cleanup() {
    check_tools
    check_kind_context
    check_cluster
    start_port_forward

    echo "Deleting all pressure ConfigMaps..."
    kubectl delete configmaps -n "$NAMESPACE" -l "$LABEL" --wait=true 2>/dev/null || true

    echo "Running compaction and defragmentation..."
    etcd_compact
    etcd_defrag
    etcd_disarm_alarm

    echo "Deleting namespace..."
    kubectl delete namespace "$NAMESPACE" --wait=true 2>/dev/null || true

    check_metric
    refresh_usage
    printf "After cleanup: %s%%\n" "$CURRENT_USAGE"

    echo ""
    echo "Cleanup completed."
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

case "${1:-}" in
    status)
        cmd_status
        ;;
    fill)
        cmd_fill "${2:-}"
        ;;
    drain)
        cmd_drain "${2:-}"
        ;;
    cleanup)
        cmd_cleanup
        ;;
    *)
        echo "Usage: $0 {status|fill <percent>|drain <percent>|cleanup}" >&2
        echo "" >&2
        echo "Commands:" >&2
        echo "  status              Show current etcd usage" >&2
        echo "  fill <percent>      Fill etcd to the target percentage" >&2
        echo "  drain <percent>     Gradually drain etcd to the target percentage" >&2
        echo "  cleanup             Remove all pressure data and namespace" >&2
        echo "" >&2
        echo "Environment:" >&2
        echo "  HOLD_SAFETY_FLOOR   Min usage during HOLD drain (default: 81.0)" >&2
        echo "  CLUSTER_NAME        KinD cluster name (default: etcd-shield-test)" >&2
        echo "  QUOTA_BYTES         etcd quota in bytes (default: 268435456)" >&2
        exit 1
        ;;
esac
