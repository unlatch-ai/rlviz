#!/usr/bin/env python3
"""Dependency-free adapter for Inspect AI EvalLog JSON documents."""

import argparse
import hashlib
import json
import math
import sys
from pathlib import Path


API_VERSION = "rlviz.dev/v1alpha1"
FORMAT = "inspect-ai-eval-log-json-v2"


def load_request(path):
    with open(path, "r", encoding="utf-8") as handle:
        request = json.load(handle)
    if request.get("api_version") != API_VERSION:
        raise ValueError("unsupported api_version")
    return request


def stable_id(prefix, *parts):
    material = json.dumps(parts, ensure_ascii=False, separators=(",", ":"), sort_keys=True)
    digest = hashlib.sha256(material.encode("utf-8")).hexdigest()[:24]
    return f"inspect-{prefix}-{digest}"


def emit(record):
    print(json.dumps(record, separators=(",", ":"), ensure_ascii=False, sort_keys=True))


def bounded_probe(request):
    source = request["source"]
    if source.get("kind") == "directory" or Path(source["path"]).suffix == ".eval":
        return False, "Inspect .eval archives and directories are not supported; export EvalLog JSON"

    limit = int(request.get("limits", {}).get("probe_bytes", 262144))
    with open(source["path"], "rb") as handle:
        payload = handle.read(limit)
        truncated = bool(handle.read(1))

    if not payload.lstrip().startswith(b"{"):
        return False, "source is not a JSON object"

    if not truncated:
        try:
            document = json.loads(payload)
        except json.JSONDecodeError:
            return False, "source is not valid JSON"
        supported = (
            isinstance(document, dict)
            and document.get("version") == 2
            and isinstance(document.get("eval"), dict)
            and isinstance(document.get("samples"), list)
        )
        return supported, "recognized Inspect EvalLog JSON version 2" if supported else "JSON does not match Inspect EvalLog version 2"

    markers = (b'"version"', b'"eval"')
    supported = all(marker in payload for marker in markers)
    return supported, "recognized bounded Inspect EvalLog JSON header" if supported else "bounded JSON prefix lacks Inspect EvalLog markers"


def probe(request):
    supported, reason = bounded_probe(request)
    emit({
        "supported": supported,
        "confidence": 0.98 if supported else 0,
        "format": FORMAT if supported else "",
        "reason": reason,
    })


def load_eval_log(path):
    with open(path, "r", encoding="utf-8") as handle:
        document = json.load(handle)
    if not isinstance(document, dict):
        raise ValueError("Inspect EvalLog JSON must be an object")
    if document.get("version") != 2:
        raise ValueError("only Inspect EvalLog JSON version 2 is supported")
    if not isinstance(document.get("eval"), dict):
        raise ValueError("Inspect EvalLog JSON requires an eval object")
    if not isinstance(document.get("samples"), list):
        raise ValueError("Inspect EvalLog JSON requires a samples array")
    return document


def source_location(path):
    return {"path": path}


def event_metadata(event, extra=None):
    metadata = {
        "source_format": FORMAT,
        "provenance": "source_native",
    }
    for key in ("span_id", "working_start", "pending", "completed", "working_time"):
        if event.get(key) is not None:
            metadata[key] = event[key]
    if isinstance(event.get("metadata"), dict):
        metadata["inspect_metadata"] = event["metadata"]
    if extra:
        metadata.update({key: value for key, value in extra.items() if value is not None})
    return metadata


def base_event(event, trajectory_id, event_id, sequence, path, parent_id):
    record = {
        "record_type": "event",
        "id": event_id,
        "trajectory_id": trajectory_id,
        "sequence": sequence,
        "kind": "log",
        "source": source_location(path),
        "raw": event,
    }
    if parent_id:
        record["parent_id"] = parent_id
    if event.get("timestamp"):
        record["timestamp"] = event["timestamp"]
    return record


def normalize_event(event, trajectory_id, sequence, path, parent_id):
    if not isinstance(event, dict):
        raise ValueError(f"sample event {sequence} must be an object")
    event_type = str(event.get("event", "unknown"))
    identity = event.get("uuid") or f"ordinal-{sequence}"
    event_id = stable_id("event", trajectory_id, identity)
    record = base_event(event, trajectory_id, event_id, sequence, path, parent_id)

    if event_type == "model":
        record["kind"] = "generation"
        record["input"] = event.get("input", [])
        record["output"] = event.get("output")
        record["metadata"] = event_metadata(event, {
            "inspect_event": event_type,
            "model": event.get("model"),
            "role": event.get("role"),
            "retries": event.get("retries"),
            "cache": event.get("cache"),
            "error": event.get("error"),
        })
    elif event_type == "tool":
        record["kind"] = "tool"
        function = event.get("function") or "unknown"
        record["alignment_key"] = f"tool:{function}"
        record["input"] = {
            "id": event.get("id"),
            "name": function,
            "arguments": event.get("arguments", {}),
        }
        record["output"] = {
            "result": event.get("result"),
            "error": event.get("error"),
            "truncated": event.get("truncated"),
        }
        record["metadata"] = event_metadata(event, {"inspect_event": event_type, "tool_type": event.get("type")})
    elif event_type == "compaction":
        record["kind"] = "state"
        operation = "truncation" if event.get("type") == "trim" else "compaction"
        record["alignment_key"] = f"context:{operation}"
        record["data"] = {
            "operation": operation,
            "type": event.get("type"),
            "role": event.get("role"),
            "before_tokens": event.get("tokens_before"),
            "after_tokens": event.get("tokens_after"),
            "source": event.get("source"),
        }
        record["metadata"] = event_metadata(event, {"inspect_event": event_type})
    elif event_type == "score":
        record["kind"] = "grader"
        scorer = event.get("scorer")
        if scorer:
            record["alignment_key"] = f"grader:{scorer}"
        record["input"] = {"target": event.get("target")}
        record["output"] = event.get("score")
        record["metadata"] = event_metadata(event, {
            "inspect_event": event_type,
            "scorer": scorer,
            "intermediate": event.get("intermediate"),
            "scorer_args": event.get("scorer_args"),
        })
    else:
        kind_by_type = {
            "error": "error",
            "message": "message",
            "state": "state",
            "store": "state",
            "sample_limit": "error",
        }
        record["kind"] = kind_by_type.get(event_type, "log")
        record["data"] = event
        record["metadata"] = event_metadata(event, {"inspect_event": event_type})

    return record


def scalar_score(score):
    value = score.get("value") if isinstance(score, dict) else score
    if isinstance(value, bool) or isinstance(value, str):
        return value
    if isinstance(value, (int, float)) and not isinstance(value, bool) and math.isfinite(value):
        return value
    return None


def sample_status(sample):
    if sample.get("error"):
        return "failed"
    return "completed"


def sample_termination(sample):
    error = sample.get("error")
    if isinstance(error, dict) and isinstance(error.get("message"), str):
        return error["message"]
    output = sample.get("output")
    if isinstance(output, dict):
        for key in ("stop_reason", "finish_reason"):
            if isinstance(output.get(key), str):
                return output[key]
    return ""


def stream(request):
    path = request["source"]["path"]
    document = load_eval_log(path)
    eval_spec = document["eval"]
    samples = document["samples"]
    run_identity = eval_spec.get("task_id") or eval_spec.get("task") or eval_spec
    run_id = stable_id("run", run_identity, eval_spec.get("created_at") or eval_spec.get("created"))
    run = {
        "record_type": "run",
        "id": run_id,
        "name": str(eval_spec.get("task") or "Inspect AI evaluation"),
        "metadata": {
            "adapter": "inspect-ai",
            "format_version": document["version"],
            "eval_status": document.get("status"),
            "task_id": eval_spec.get("task_id"),
            "model": eval_spec.get("model"),
        },
    }
    records = [run]
    warnings = 0
    groups = {}

    for sample_index, sample in enumerate(samples):
        if not isinstance(sample, dict):
            raise ValueError(f"sample {sample_index} must be an object")
        if "id" not in sample:
            raise ValueError(f"sample {sample_index} requires id")
        sample_key = json.dumps(sample["id"], ensure_ascii=False, sort_keys=True)
        if sample_key not in groups:
            case_id = stable_id("case", run_id, sample["id"])
            group_id = stable_id("group", run_id, sample["id"])
            case = {
                "record_type": "case",
                "id": case_id,
                "run_id": run_id,
                "name": str(sample["id"]),
                "input": sample.get("input"),
                "metadata": {"target": sample.get("target")},
            }
            group = {
                "record_type": "group",
                "id": group_id,
                "case_id": case_id,
                "name": f"Inspect sample {sample['id']}",
            }
            records.extend((case, group))
            groups[sample_key] = group_id

        group_id = groups[sample_key]
        epoch = sample.get("epoch", 1)
        trajectory_identity = sample.get("uuid") or [sample["id"], epoch, sample_index]
        trajectory_id = stable_id("trajectory", run_id, trajectory_identity)
        trajectory = {
            "record_type": "trajectory",
            "id": trajectory_id,
            "group_id": group_id,
            "status": sample_status(sample),
            "metadata": {
                "sample_id": sample["id"],
                "sample_uuid": sample.get("uuid"),
                "epoch": epoch,
                "started_at": sample.get("started_at"),
                "completed_at": sample.get("completed_at"),
            },
        }
        termination = sample_termination(sample)
        if termination:
            trajectory["termination"] = termination
        records.append(trajectory)

        parent_id = None
        score_events = {}
        events = sample.get("events", [])
        if not isinstance(events, list):
            raise ValueError(f"sample {sample['id']} events must be an array")
        for sequence, event in enumerate(events):
            canonical = normalize_event(event, trajectory_id, sequence, path, parent_id)
            records.append(canonical)
            parent_id = canonical["id"]
            if event.get("event") == "score" and event.get("scorer"):
                score_events[event["scorer"]] = canonical["id"]

        scores = sample.get("scores") or {}
        if not isinstance(scores, dict):
            raise ValueError(f"sample {sample['id']} scores must be an object")
        for scorer, score in scores.items():
            value = scalar_score(score)
            if value is None:
                warnings += 1
                continue
            signal = {
                "record_type": "signal",
                "id": stable_id("signal", trajectory_id, scorer),
                "trajectory_id": trajectory_id,
                "name": str(scorer),
                "value": value,
                "metadata": {"source_format": FORMAT, "provenance": "source_native"},
            }
            if scorer in score_events:
                signal["event_id"] = score_events[scorer]
            records.append(signal)

    for record in records:
        emit(record)
    emit({"record_type": "complete", "records": len(records), "warnings": warnings})


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
