#!/usr/bin/env python3
import hashlib
import json
import urllib.request

BASE = "http://127.0.0.1:8080"

def request(method, path, body=None):
    data = None if body is None else json.dumps(body).encode()
    req = urllib.request.Request(BASE + path, data=data, method=method)
    req.add_header("Accept", "application/json")
    if data is not None:
        req.add_header("Content-Type", "application/json")
    with urllib.request.urlopen(req, timeout=3) as response:
        raw = response.read()
        return None if not raw else json.loads(raw)

def main():
    iphone = request("POST", "/v1/devices", {"name": "E2E iPhone", "platform": "ios"})
    pixel = request("POST", "/v1/devices", {"name": "E2E Pixel", "platform": "android"})

    payload = b"pixel-go-e2e"
    checksum = hashlib.sha256(payload).hexdigest()
    transfer = request("POST", "/v1/transfers", {
        "sourceDeviceId": iphone["id"],
        "destinationDeviceId": pixel["id"],
        "kind": "file",
        "displayName": "e2e.txt",
        "contentType": "text/plain",
        "sizeBytes": len(payload),
        "sha256": checksum
    })
    assert transfer["status"] == "uploading", transfer

    ready = request("POST", f"/v1/transfers/{transfer['id']}/uploaded")
    assert ready["status"] == "ready", ready

    delivered = request("POST", f"/v1/transfers/{transfer['id']}/complete")
    assert delivered["status"] == "completed", delivered
    assert delivered["sha256"] == checksum

    fetched = request("GET", f"/v1/transfers/{transfer['id']}")
    assert fetched["status"] == "completed"

    print("PASS: iOS device -> Android device transfer lifecycle")

if __name__ == "__main__":
    main()
