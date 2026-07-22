"""日志配置：应用 dictConfig。

对标 Go hertz_infra，但暂不引入服务治理（限流 / 熔断 / Consul 配置中心）。
仅 main.py 在启动期 import；业务层禁止 import。
"""

from __future__ import annotations

import logging.config


def setup_logging() -> None:
    """配置应用日志。简化版：默认 INFO + text 格式。"""
    level = logging.INFO
    logging.config.dictConfig(
        {
            "version": 1,
            "disable_existing_loggers": False,
            "formatters": {
                "default": {
                    "format": "%(asctime)s [%(levelname)s] %(name)s: %(message)s",
                }
            },
            "handlers": {
                "console": {
                    "class": "logging.StreamHandler",
                    "formatter": "default",
                    "level": level,
                }
            },
            "root": {"level": level, "handlers": ["console"]},
            "loggers": {
                "uvicorn": {"level": "INFO", "handlers": ["console"], "propagate": False},
                "uvicorn.access": {"level": "INFO", "handlers": ["console"], "propagate": False},
            },
        }
    )
