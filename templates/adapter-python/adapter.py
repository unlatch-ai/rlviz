#!/usr/bin/env python3
"""Dependency-free RolloutViz adapter template."""
import argparse
import json
import sys


def emit(record):
    print(json.dumps(record, separators=(",", ":"), ensure_ascii=False))


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("operation", choices=("probe", "stream"))
    parser.add_argument("--request", required=True)
    args = parser.parse_args()
    with open(args.request, "r", encoding="utf-8") as handle:
        request = json.load(handle)
    if request["operation"] != args.operation:
        raise ValueError("request operation mismatch")
    if args.operation == "probe":
        print(json.dumps({"supported": False, "confidence": 0, "reason": "implement format detection"}))
    else:
        emit({"record_type": "complete", "records": 0, "warnings": 0})


if __name__ == "__main__":
    try:
        main()
    except Exception as error:
        print(str(error), file=sys.stderr)
        raise SystemExit(1)
