"""Executable pseudocode for 003 Agent invariants; no production adapters are mocked here.

Run: python3 harness/tools/agent_runtime_model/verify.py
This explores business ordering, not broker/Kubernetes transport correctness.
"""
from __future__ import annotations

from dataclasses import dataclass, field
from itertools import permutations


class Conflict(Exception):
    pass


@dataclass
class Turn:
    key: str
    sequence: int
    context: int
    status: str = "queued"


@dataclass
class Session:
    context: int
    owner: str
    epoch: int = 1
    paused: bool = False
    draining: set[str] = field(default_factory=set)
    turns: list[Turn] = field(default_factory=list)
    events: list[int] = field(default_factory=list)

    def submit(self, key: str) -> Turn:
        existing = next((turn for turn in self.turns if turn.key == key), None)
        if existing is not None:
            return existing
        turn = Turn(key, len(self.turns) + 1, self.context)
        self.turns.append(turn)
        return turn

    def claim(self, owner: str, epoch: int) -> Turn | None:
        if (owner, epoch) != (self.owner, self.epoch):
            raise Conflict("stale owner")
        if self.paused or owner in self.draining or any(t.status == "running" for t in self.turns):
            return None
        turn = next((t for t in self.turns if t.status == "queued"), None)
        if turn is not None:
            turn.status = "running"
        return turn

    def finish(self, owner: str, epoch: int, key: str, status: str) -> None:
        if (owner, epoch) != (self.owner, self.epoch):
            raise Conflict("late writer")
        turn = next(t for t in self.turns if t.key == key)
        if turn.status != "running":
            raise Conflict("terminal turn is immutable")
        turn.status = status
        self.paused |= status != "completed"
        self.events.append(len(self.events) + 1)

    def transfer(self, expected_epoch: int, target: str, old_terminated: bool = False) -> int:
        if expected_epoch != self.epoch:
            raise Conflict("concurrent transfer")
        running = next((t for t in self.turns if t.status == "running"), None)
        if running is not None:
            if not old_terminated:
                raise Conflict("running is not proof of termination")
            self.finish(self.owner, self.epoch, running.key, "failed")
        self.owner, self.epoch = target, self.epoch + 1
        return self.epoch


@dataclass
class Metadata:
    selected: int = 0
    releases: dict[int, dict[str, int]] = field(default_factory=dict)
    applied: dict[str, int] = field(default_factory=dict)
    session_refs: dict[str, int] = field(default_factory=dict)

    def confirm(self, resources: dict[str, int]) -> int:
        self.selected += 1
        self.releases[self.selected] = dict(resources)
        return self.selected

    def apply(self, instance: str, notice: int, fail: bool = False) -> bool:
        # Pull only a confirmed release, never mutable Catalog latest.
        if notice not in self.releases:
            raise Conflict("unconfirmed target")
        target = self.selected if notice != self.selected else notice
        if fail:
            return False
        if target < self.applied.get(instance, 0):
            raise Conflict("decreasing selection")
        self.applied[instance] = target
        return True


@dataclass
class Bus:
    queues: dict[str, set[str]] = field(default_factory=dict)
    local: dict[tuple[str, str], list[list[int]]] = field(default_factory=dict)

    def open(self, gateway: str, session: str) -> list[int]:
        receiver: list[int] = []
        self.queues.setdefault(gateway, set()).add(session)
        self.local.setdefault((gateway, session), []).append(receiver)
        return receiver

    def publish(self, session: str, sequence: int) -> None:
        for gateway, subjects in self.queues.items():
            if session in subjects:
                for receiver in self.local.get((gateway, session), []):
                    receiver.append(sequence)


def expect_conflict(operation) -> None:
    try:
        operation()
    except Conflict:
        return
    raise AssertionError("expected conflict")


def verify() -> list[str]:
    passed: list[str] = []
    def check(name: str, condition: bool) -> None:
        assert condition, name
        passed.append(name)

    session = Session(7, "pod0/boot1")
    session.submit("a")
    session.submit("a")
    session.submit("b")
    check("idempotent submission and FIFO", len(session.turns) == 2 and session.turns[1].sequence == 2)
    first = session.claim(session.owner, 1)
    check("single running", first is not None and session.claim(session.owner, 1) is None)
    session.draining.add(session.owner)
    session.finish(session.owner, 1, "a", "completed")
    check("drain preserves queued without claiming", session.claim(session.owner, 1) is None and session.turns[1].status == "queued")
    epoch = session.transfer(1, "pod1/boot1")
    check("rebinding keeps context and FIFO", session.claim(session.owner, epoch) is session.turns[1] and session.context == 7)
    expect_conflict(lambda: session.finish("pod0/boot1", 1, "b", "completed"))
    passed.append("late old-owner write rejected")
    expect_conflict(lambda: session.transfer(1, "pod2/boot1"))
    passed.append("concurrent transfer CAS rejected")

    unknown = Session(3, "old")
    unknown.submit("external-write")
    unknown.submit("later")
    unknown.claim("old", 1)
    expect_conflict(lambda: unknown.transfer(1, "new"))
    passed.append("NotReady alone cannot authorize running transfer")
    unknown.transfer(1, "new", old_terminated=True)
    check("unknown outcome fails without replay and pauses queue", unknown.paused and unknown.turns[0].status == "failed" and unknown.claim("new", 2) is None)
    unknown.paused = False  # Explicit user Resume; never an implicit migration effect.
    check("explicit resume executes only previously queued", unknown.claim("new", 2) is unknown.turns[1])

    metadata = Metadata()
    revision1 = metadata.confirm({"skill": 7})
    metadata.session_refs["old-session"] = revision1
    metadata.apply("pod0", revision1)
    revision2 = metadata.confirm({"skill": 5})
    metadata.apply("pod0", revision2)
    check("explicit resource rollback is a newer selection", metadata.releases[metadata.applied["pod0"]]["skill"] == 5)
    metadata.apply("pod0", revision1)
    check("out-of-order notice cannot undo latest confirmed choice", metadata.applied["pod0"] == revision2)
    check("old session remains pinned", metadata.session_refs["old-session"] == revision1)
    revision3 = metadata.confirm({"skill": 9})
    check("failed installation leaves last good version", not metadata.apply("pod0", revision3, fail=True) and metadata.applied["pod0"] == revision2)
    expect_conflict(lambda: metadata.apply("pod0", 999))
    passed.append("unconfirmed release cannot be loaded")
    for ordering in permutations([revision1, revision2]):
        for notice in ordering:
            metadata.apply("pod1", notice)
        assert metadata.applied["pod1"] == revision3
    passed.append("all notification permutations converge")

    bus = Bus()
    a = bus.open("gateway-a", "session")
    b = bus.open("gateway-a", "session")
    c = bus.open("gateway-b", "session")
    bus.publish("session", 1)
    check("one broker queue per gateway, not per SSE", len(bus.queues) == 2 and a == b == c == [1])
    committed = [1, 2, 3]
    delivered = [3, 1, 3]  # Lost publish of sequence 2 plus out-of-order duplicates.
    cursor, emitted = 0, []
    for incoming in delivered:
        if incoming > cursor:
            for event in committed:
                if event == cursor + 1:
                    emitted.append(event)
                    cursor = event
    check("replay fills lost publication without duplicates", emitted == committed)

    rows: dict[tuple[str, int, str], str] = {}
    current_boot = {"pod0": "boot2"}
    def report(replica: str, boot: str, revision: int, state: str) -> None:
        key = (f"{replica}/{boot}", revision, "context")
        if rows.get(key) != "applied":
            rows[key] = state
    report("pod0", "boot1", revision2, "applied")
    check("old incarnation report does not mark new instance successful", rows.get((f"pod0/{current_boot['pod0']}", revision2, "context")) is None)
    report("pod0", "boot2", revision2, "applied")
    report("pod0", "boot2", revision2, "failed")
    check("late failure cannot revoke an application success", rows[("pod0/boot2", revision2, "context")] == "applied")

    definitions = {"alpha": {"search": "a"}, "beta": {"search": "b"}}
    enabled = {"alpha", "beta"}
    def discover() -> list[str]:
        return sorted(f"{definition}__{tool}" for definition in enabled for tool in definitions[definition])
    def invoke(exposed: str) -> str:
        definition, tool = exposed.split("__", 1)
        if definition not in enabled:
            raise Conflict("revoked tool")
        return definitions[definition][tool]
    check("aggregated tool names preserve origins", discover() == ["alpha__search", "beta__search"] and invoke("beta__search") == "b")
    enabled.remove("beta")
    expect_conflict(lambda: invoke("beta__search"))
    passed.append("MCP invocation rechecks revoked binding")

    # Two-step transfer: response may be lost after Agent commits epoch 2.
    handoff = Session(1, "old")
    gateway_binding = {"phase": "transferring", "expected_epoch": 1, "target": "new"}
    handoff.transfer(gateway_binding["expected_epoch"], gateway_binding["target"])
    # Reconcile queries Agent instead of issuing an unrelated transfer again.
    if handoff.owner == gateway_binding["target"] and handoff.epoch == 2:
        gateway_binding["phase"] = "active"
    check("lost transfer response reconciles committed ownership", gateway_binding["phase"] == "active" and handoff.epoch == 2)
    return passed


if __name__ == "__main__":
    checks = verify()
    for check_name in checks:
        print(f"PASS {check_name}")
    print(f"{len(checks)} invariant scenarios passed (model validation only)")
