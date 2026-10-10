#!/usr/bin/env python3
"""Small JSON-lines curl_cffi transport used by the protocol runner.

The process owns one curl_cffi Session and therefore keeps its cookie jar and
browser TLS profile across the complete OAuth/OTP flow. It is deliberately a
request transport only; it does not solve or bypass provider challenges.
"""

import base64
import json
import os
import sys
from typing import Any

from curl_cffi import requests


def emit(value: dict[str, Any]) -> None:
    sys.stdout.write(json.dumps(value, separators=(",", ":")) + "\n")
    sys.stdout.flush()


def main() -> None:
    session = requests.Session(impersonate="chrome")
    for line in sys.stdin:
        try:
            item = json.loads(line)
            proxy = item.get("proxy") or ""
            proxies = {"http": proxy, "https": proxy} if proxy else None
            headers = item.get("headers") or {}
            body = item.get("body")
            response = session.request(
                method=str(item.get("method") or "GET"),
                url=str(item["url"]),
                headers=headers,
                data=body if body is not None else None,
                proxies=proxies,
                timeout=float(item.get("timeout", 30)),
                allow_redirects=False,
                verify=True,
            )
            try:
                set_cookie = response.headers.get_list("set-cookie")
            except AttributeError:
                set_cookie = []
            regular_headers = {
                str(key).lower(): str(value)
                for key, value in response.headers.items()
                if str(key).lower() != "set-cookie"
            }
            body64 = base64.b64encode(response.content).decode("ascii")
            emit({
                "id": item.get("id"),
                "status": response.status_code,
                "headers": regular_headers,
                "setCookie": [str(value) for value in set_cookie],
                "body": body64,
            })
        except Exception as exc:
            emit({"id": item.get("id"), "error": f"{type(exc).__name__}: {exc}"})


if __name__ == "__main__":
    main()
