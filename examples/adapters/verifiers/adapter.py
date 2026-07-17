#!/usr/bin/env python3
"""Map Prime Intellect Verifiers GenerateOutputs JSON to canonical RLViz."""

import argparse
import hashlib
import json
import sys
from pathlib import Path


API_VERSION = "rlviz.dev/v1alpha1"


def load_request(path):
    with open(path, "r", encoding="utf-8") as handle:
        request = json.load(handle)
    if request.get("api_version") != API_VERSION:
        raise ValueError("unsupported api_version")
    return request


def load_source(path):
    with open(path, "rb") as handle:
        raw = handle.read()
    value = json.loads(raw)
    return raw, value


def supported(value):
    if not isinstance(value, dict) or not isinstance(value.get("outputs"), list):
        return False
    if not isinstance(value.get("metadata"), dict):
        return False
    return all(
        isinstance(output, dict) and isinstance(output.get("trajectory", []), list)
        for output in value["outputs"]
    )


def emit(record):
    print(json.dumps(record, separators=(",", ":"), ensure_ascii=False, sort_keys=True))


def bounded_probe(request):
    source = request["source"]
    if source.get("kind") == "directory":
        return False, "Verifiers GenerateOutputs must be a JSON file"
    limit = int(request.get("limits", {}).get("probe_bytes", 262144))
    with open(source["path"], "rb") as handle:
        payload = handle.read(limit)
        truncated = bool(handle.read(1))
    if not payload.lstrip().startswith(b"{"):
        return False, "source is not a JSON object"
    if truncated:
        matches = b'"metadata"' in payload and b'"outputs"' in payload
        return matches, "recognized bounded GenerateOutputs header" if matches else "bounded JSON prefix lacks GenerateOutputs markers"
    try:
        value = json.loads(payload)
    except json.JSONDecodeError:
        return False, "source is not valid JSON"
    matches = supported(value)
    return matches, "recognized GenerateOutputs metadata and outputs" if matches else "unknown JSON shape"


def probe(request):
    matches, reason = bounded_probe(request)
    emit({
        "supported": matches,
        "confidence": 0.98 if matches else 0,
        "format": "prime-verifiers-generate-outputs" if matches else "",
        "reason": reason,
    })


def masked_count(mask):
    if not isinstance(mask, list) or not all(isinstance(value, int) for value in mask):
        return None
    return sum(1 for value in mask if value != 0)


def stream(request):
    path = request["source"]["path"]
    raw, value = load_source(path)
    if not supported(value):
        raise ValueError("source is not Verifiers GenerateOutputs JSON")
    digest = hashlib.sha256(raw).hexdigest()[:16]
    metadata = value["metadata"]
    run_id = f"run-{digest}"
    records = [{
        "record_type": "run",
        "id": run_id,
        "name": metadata.get("env_id") or Path(path).name,
        "started_at": metadata.get("date", ""),
        "metadata": {
            "adapter": "verifiers-generate",
            "model": metadata.get("model"),
            "rollouts_per_example": metadata.get("rollouts_per_example"),
            "version_info": metadata.get("version_info"),
        },
    }]

    seen_cases = set()
    for output_index, output in enumerate(value["outputs"]):
        case_key = output.get("example_id", output_index)
        case_id = f"case-{digest}-{case_key}"
        group_id = f"group-{digest}-{case_key}"
        trajectory_id = output.get("trajectory_id") or f"trajectory-{digest}-{output_index}"
        if case_key not in seen_cases:
            records.extend([{
                "record_type": "case",
                "id": case_id,
                "run_id": run_id,
                "name": str(output.get("task") or f"example {case_key}"),
                "input": output.get("prompt"),
                "metadata": {"answer": output.get("answer"), "info": output.get("info")},
            }, {"record_type": "group", "id": group_id, "case_id": case_id}])
            seen_cases.add(case_key)
        records.append({
            "record_type": "trajectory",
            "id": trajectory_id,
            "group_id": group_id,
            "status": "completed" if output.get("is_completed") else "incomplete",
            "termination": output.get("stop_condition") or ("error" if output.get("error") else ""),
            "metadata": {
                "error": output.get("error"),
                "is_truncated": output.get("is_truncated"),
                "timing": output.get("timing"),
            },
        })
        parent_id = None
        sequence = 0
        for step_index, step in enumerate(output.get("trajectory", [])):
            event_id = f"event-{digest}-{output_index}-{step_index}"
            tokens = step.get("tokens") if isinstance(step.get("tokens"), dict) else {}
            prompt_tokens = masked_count(tokens.get("prompt_mask"))
            completion_tokens = masked_count(tokens.get("completion_mask"))
            event = {
                "record_type": "event",
                "id": event_id,
                "trajectory_id": trajectory_id,
                "sequence": sequence,
                "kind": "generation",
                "alignment_key": f"generation:{step_index}",
                "input": step.get("prompt"),
                "output": step.get("completion"),
                "data": {
                    "tokens": tokens,
                    "prompt_tokens_from_mask": prompt_tokens,
                    "completion_tokens_from_mask": completion_tokens,
                    "is_truncated": step.get("is_truncated"),
                    "reward": step.get("reward"),
                    "advantage": step.get("advantage"),
                    "extras": step.get("extras"),
                },
                "source": {"path": path},
                "raw": step,
                "metadata": {"context_provenance": "adapter_derived_from_prompt_mask"},
            }
            if parent_id:
                event["parent_id"] = parent_id
            records.append(event)
            parent_id = event_id
            sequence += 1

        reward_event_id = f"reward-{digest}-{output_index}"
        reward_event = {
            "record_type": "event",
            "id": reward_event_id,
            "trajectory_id": trajectory_id,
            "sequence": sequence,
            "kind": "reward",
            "alignment_key": "reward:final",
            "data": {"reward": output.get("reward"), "metrics": output.get("metrics", {})},
            "source": {"path": path},
        }
        if parent_id:
            reward_event["parent_id"] = parent_id
        records.append(reward_event)
        if isinstance(output.get("reward"), (int, float)) and not isinstance(output.get("reward"), bool):
            records.append({
                "record_type": "signal",
                "id": f"signal-reward-{digest}-{output_index}",
                "trajectory_id": trajectory_id,
                "event_id": reward_event_id,
                "name": "reward",
                "value": output["reward"],
            })
        for name, metric in sorted((output.get("metrics") or {}).items()):
            if isinstance(metric, (bool, int, float, str)):
                records.append({
                    "record_type": "signal",
                    "id": f"signal-{digest}-{output_index}-{hashlib.sha256(name.encode()).hexdigest()[:8]}",
                    "trajectory_id": trajectory_id,
                    "event_id": reward_event_id,
                    "name": name,
                    "value": metric,
                })

    for record in records:
        emit(record)
    emit({"record_type": "complete", "records": len(records), "warnings": 0})


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("operation", choices=("probe", "stream"))
    parser.add_argument("--request", required=True)
    arguments = parser.parse_args()
    request = load_request(arguments.request)
    if request.get("operation") != arguments.operation:
        raise ValueError("request operation mismatch")
    (probe if arguments.operation == "probe" else stream)(request)


if __name__ == "__main__":
    try:
        main()
    except Exception as error:
        print(str(error), file=sys.stderr)
        raise SystemExit(1)
