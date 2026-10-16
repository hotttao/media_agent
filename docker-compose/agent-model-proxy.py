"""Local Docker bridge to a host loopback HTTP proxy; no credentials are read."""
import os
import select
import socket
import threading


def relay(client):
    upstream = None
    try:
        upstream = socket.create_connection(("127.0.0.1", int(os.getenv("MODEL_PROXY_PORT", "7897"))), timeout=10)
        upstream.settimeout(None)
        while True:
            ready, _, _ = select.select([client, upstream], [], [], 60)
            for source in ready:
                data = source.recv(65536)
                if not data:
                    return
                (upstream if source is client else client).sendall(data)
    except OSError:
        pass
    finally:
        client.close()
        if upstream:
            upstream.close()


with socket.socket() as listener:
    listener.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
    listener.bind((os.getenv("MODEL_PROXY_BIND", "172.27.0.1"), 17897))
    listener.listen(32)
    while True:
        client, _ = listener.accept()
        threading.Thread(target=relay, args=(client,), daemon=True).start()
