#!/usr/bin/env python3
import hashlib
import json
import urllib.error
import urllib.request

BASE = "http://127.0.0.1:8080"
PRIMARY_EMAIL = "e2e-primary@pixelgo.local"
PRIMARY_PASSWORD = "correct-horse-battery-staple-e2e"
SECONDARY_EMAIL = "e2e-secondary@pixelgo.local"
SECONDARY_PASSWORD = "another-correct-horse-battery-e2e"


def request(method, path, body=None, headers=None):
    status, payload, _ = request_status(method, path, body, headers)
    if status < 200 or status >= 300:
        raise AssertionError(f"{method} {path} returned HTTP {status}: {payload}")
    return payload


def request_status(method, path, body=None, headers=None):
    data = None if body is None else json.dumps(body).encode()
    req = urllib.request.Request(BASE + path, data=data, method=method)
    req.add_header("Accept", "application/json")
    if data is not None:
        req.add_header("Content-Type", "application/json")
    for key, value in (headers or {}).items():
        req.add_header(key, value)

    try:
        with urllib.request.urlopen(req, timeout=3) as response:
            raw = response.read()
            payload = None if not raw else json.loads(raw)
            return response.status, payload, dict(response.headers)
    except urllib.error.HTTPError as error:
        raw = error.read()
        payload = None if not raw else json.loads(raw)
        return error.code, payload, dict(error.headers)


def bearer(access_token, extra=None):
    headers = {"Authorization": f"Bearer {access_token}"}
    headers.update(extra or {})
    return headers


def upload(url, payload, content_type, checksum):
    req = urllib.request.Request(url, data=payload, method="PUT")
    req.add_header("Content-Type", content_type)
    req.add_header("Content-Length", str(len(payload)))
    req.add_header("X-Amz-Meta-Sha256", checksum)
    with urllib.request.urlopen(req, timeout=3) as response:
        assert 200 <= response.status < 300, response.status


def download(url):
    req = urllib.request.Request(url, method="GET")
    with urllib.request.urlopen(req, timeout=3) as response:
        assert response.status == 200, response.status
        return response.read()


def register(email, password):
    return request(
        "POST",
        "/v1/auth/register",
        {"email": email, "password": password},
    )


def main():
    primary = register(PRIMARY_EMAIL, PRIMARY_PASSWORD)
    access = primary["accessToken"]
    original_refresh = primary["refreshToken"]

    unauthorized_status, _, _ = request_status("GET", "/v1/devices")
    assert unauthorized_status == 401, unauthorized_status

    iphone = request(
        "POST",
        "/v1/devices",
        {"name": "E2E iPhone", "platform": "ios"},
        bearer(access, {"Idempotency-Key": "e2e-device-ios"}),
    )
    pixel = request(
        "POST",
        "/v1/devices",
        {"name": "E2E Pixel", "platform": "android"},
        bearer(access, {"Idempotency-Key": "e2e-device-android"}),
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
        bearer(access, {"Idempotency-Key": "e2e-transfer-create"}),
    )
    retry = request(
        "POST",
        "/v1/transfers",
        create_body,
        bearer(access, {"Idempotency-Key": "e2e-transfer-create"}),
    )

    assert transfer["status"] == "uploading", transfer
    assert retry["id"] == transfer["id"], (transfer, retry)
    assert transfer["id"] in transfer["uploadUrl"]

    upload(transfer["uploadUrl"], payload, "text/plain", checksum)

    ready = request(
        "POST",
        f"/v1/transfers/{transfer['id']}/uploaded",
        headers=bearer(access, {"Idempotency-Key": "e2e-transfer-uploaded"}),
    )
    assert ready["status"] == "ready", ready
    assert transfer["id"] in ready["downloadUrl"]

    received = download(ready["downloadUrl"])
    assert received == payload
    assert hashlib.sha256(received).hexdigest() == checksum

    delivered = request(
        "POST",
        f"/v1/transfers/{transfer['id']}/complete",
        headers=bearer(access, {"Idempotency-Key": "e2e-transfer-complete"}),
    )
    assert delivered["status"] == "completed", delivered
    assert delivered["sha256"] == checksum

    fetched = request(
        "GET",
        f"/v1/transfers/{transfer['id']}",
        headers=bearer(access),
    )
    assert fetched["status"] == "completed"

    secondary = register(SECONDARY_EMAIL, SECONDARY_PASSWORD)
    secondary_access = secondary["accessToken"]

    secondary_devices = request(
        "GET",
        "/v1/devices",
        headers=bearer(secondary_access),
    )
    assert secondary_devices == [], secondary_devices

    cross_user_status, _, _ = request_status(
        "GET",
        f"/v1/transfers/{transfer['id']}",
        headers=bearer(secondary_access),
    )
    assert cross_user_status == 404, cross_user_status

    rotated = request(
        "POST",
        "/v1/auth/refresh",
        {"refreshToken": original_refresh},
    )
    assert rotated["refreshToken"] != original_refresh

    reuse_status, reuse_body, _ = request_status(
        "POST",
        "/v1/auth/refresh",
        {"refreshToken": original_refresh},
    )
    assert reuse_status == 401, (reuse_status, reuse_body)
    assert reuse_body["code"] == "refresh_reuse_detected", reuse_body

    family_status, family_body, _ = request_status(
        "POST",
        "/v1/auth/refresh",
        {"refreshToken": rotated["refreshToken"]},
    )
    assert family_status == 401, (family_status, family_body)
    assert family_body["code"] == "invalid_refresh", family_body

    print(
        "PASS: auth + user isolation + signed bytes + SHA-256 + "
        "idempotency + refresh reuse revocation"
    )


if __name__ == "__main__":
    main()
