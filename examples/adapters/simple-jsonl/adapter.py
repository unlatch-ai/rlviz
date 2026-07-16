#!/usr/bin/env python3
"""Example adapter for a deliberately small agent-event JSONL format."""

import argparse
import hashlib
import json
import sys
from pathlib import Path


API_VERSION = "rolloutviz.dev/v1alpha1"


def load_request(path):
    with open(path, "r", encoding="utf-8") as handle:
        request = json.load(handle)
    if request.get("api_version") != API_VERSION:
        raise ValueError("unsupported api_version")
    return request


def source_records(path):
    with open(path, "rb") as handle:
        raw = handle.read()
    records = []
    offset = 0
    for line_number, line in enumerate(raw.splitlines(keepends=True), start=1):
        payload = line.rstrip(b"\r\n")
        if payload:
            records.append((line_number, offset, len(line), json.loads(payload)))
        offset += len(line)
    return raw, records


def emit(record):
    print(json.dumps(record, separators=(",", ":"), ensure_ascii=False, sort_keys=True))


def probe(request):
    _, records = source_records(request["source"]["path"])
    supported = bool(records) and all(
        isinstance(record, dict) and record.get("type") in {"message", "tool", "reward"}
        for _, _, _, record in records
    )
    print(json.dumps({
        "supported": supported,
        "confidence": 0.95 if supported else 0,
        "format": "simple-agent-jsonl" if supported else "",
        "reason": "recognized simple message/tool/reward records" if supported else "unknown record shape",
    }, separators=(",", ":"), sort_keys=True))


def stream(request):
    path = request["source"]["path"]
    raw, records = source_records(path)
    digest = hashlib.sha256(raw).hexdigest()[:16]
    run_id = f"run-{digest}"
    case_id = f"case-{digest}"
    group_id = f"group-{digest}"
    trajectory_id = f"trajectory-{digest}"

    output = [
        {"record_type": "run", "id": run_id, "name": "simple JSONL example", "metadata": {"adapter": "simple-jsonl"}},
        {"record_type": "case", "id": case_id, "run_id": run_id, "name": Path(path).name},
        {"record_type": "group", "id": group_id, "case_id": case_id},
        {"record_type": "trajectory", "id": trajectory_id, "group_id": group_id, "status": "completed"},
    ]
    parent_id = None
    for index, (line_number, offset, length, record) in enumerate(records):
        event_id = f"event-{digest}-{index}"
        event = {
            "record_type": "event",
            "id": event_id,
            "trajectory_id": trajectory_id,
            "sequence": index,
            "kind": "tool" if record["type"] == "tool" else record["type"],
            "source": {"path": path, "line": line_number, "byte_offset": offset, "byte_length": length},
            "raw": record,
        }
        if parent_id:
            event["parent_id"] = parent_id
        if record["type"] == "message":
            event["input" if record.get("role") == "user" else "output"] = {
                "role": record.get("role"), "content": record.get("content")
            }
        elif record["type"] == "tool":
            event["alignment_key"] = f"tool:{record.get('name', 'unknown')}"
            event["input"] = {"name": record.get("name"), "arguments": record.get("arguments", {})}
            event["output"] = record.get("result")
        else:
            event["data"] = {"reward": record.get("value"), "component": record.get("component")}
        output.append(event)
        parent_id = event_id

    for record in output:
        emit(record)
    emit({"record_type": "complete", "records": len(output), "warnings": 0})


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
