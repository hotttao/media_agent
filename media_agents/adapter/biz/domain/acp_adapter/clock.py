"""Clock 抽象：可注入的时钟，便于测试。"""

from __future__ import annotations

import time
from typing import Protocol, runtime_checkable


@runtime_checkable
class Clock(Protocol):
    def now(self) -> float: ...


class SystemClock:
    def now(self) -> float:
        return time.time()
