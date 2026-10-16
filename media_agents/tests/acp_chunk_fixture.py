"""Emit all native chunks and the terminal RPC response in one burst."""

import json
import sys


def send(value: object) -> None:
    print(json.dumps(value), flush=True)


for line in sys.stdin:
    request = json.loads(line)
    method = request.get("method")
    if method == "initialize":
        result = {"protocolVersion": 1, "agentCapabilities": {}}
    elif method == "session/new":
        result = {"sessionId": "native-chunks"}
    elif method == "session/prompt":
        for text in ["8c7189b0", "-7cd3", "-4c8d", "-927f", "-4ace400a2ebc"]:
            send(
                {
                    "jsonrpc": "2.0",
                    "method": "session/update",
                    "params": {
                        "sessionId": "native-chunks",
                        "update": {
                            "sessionUpdate": "agent_message_chunk",
                            "content": {
                                "type": "text",
                                "text": text,
                            },
                        },
                    },
                }
            )
        if request["params"]["prompt"][0]["text"] == "fail":
            send(
                {
                    "jsonrpc": "2.0",
                    "id": request["id"],
                    "error": {
                        "code": -32603,
                        "message": "Provider failed after partial output",
                    },
                }
            )
            continue
        result = {"stopReason": "end_turn"}
    else:
        continue
    send({"jsonrpc": "2.0", "id": request["id"], "result": result})
