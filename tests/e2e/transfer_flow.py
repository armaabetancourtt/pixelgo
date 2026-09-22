#!/usr/bin/env python3
import hashlib
import json
import urllib.request

BASE = "http://127.0.0.1:8080"


def request(method, path, body=None, headers=None):
    data = None if body is None else json.dumps(body).encode()
    req = urllib.request.Request(BASE + path, data=data, method=method)
    req.add_header("Accept", "application/json")
    if data is not None:
        req.add_header("Content-Type", "application/json")
    for key, value in (headers or {}).items():
        req.add_header(key, value)

    with urllib.request.urlopen(req, timeout=3) as response:
        raw = response.read()
        return None if not raw else json.loads(raw)


def main():
    iphone = request(
        "POST",
        "/v1/devices",
        {"name": "E2E iPhone", "platform": "ios"},
        {"Idempotency-Key": "e2e-device-ios"},
    )
    pixel = request(
        "POST",
        "/v1/devices",
        {"name": "E2E Pixel", "platform": "android"},
        {"Idempotency-Key": "e2e-device-android"},
    )

    payload = b"pixel-go-e2e"
    checksum = hashlib.sha256(payload).hexdigest()
    create_body = {
        "sourceDeviceId": iphone["id"],
        "destinationDeviceId": pixel["id"],
        "kind": "file",
        "displayName": "e2e.txt",
        "contentType": "text/plain",
        "sizeBytes": len(payload),
        "sha256": checksum,
    }

    transfer = request(
        "POST",
        "/v1/transfers",
        create_body,
        {"Idempotency-Key": "e2e-transfer-create"},
    )
    retry = request(
        "POST",
        "/v1/transfers",
        create_body,
        {"Idempotency-Key": "e2e-transfer-create"},
    )

    assert transfer["status"] == "uploading", transfer
    assert retry["id"] == transfer["id"], (transfer, retry)

    ready = request(
        "POST",
        f"/v1/transfers/{transfer['id']}/uploaded",
        headers={"Idempotency-Key": "e2e-transfer-uploaded"},
    )
    assert ready["status"] == "ready", ready

    delivered = request(
        "POST",
        f"/v1/transfers/{transfer['id']}/complete",
        headers={"Idempotency-Key": "e2e-transfer-complete"},
    )
    assert delivered["status"] == "completed", delivered
    assert delivered["sha256"] == checksum

    fetched = request("GET", f"/v1/transfers/{transfer['id']}")
    assert fetched["status"] == "completed"

    print("PASS: iOS -> Android lifecycle + retry-safe idempotency")


if __name__ == "__main__":
    main()
