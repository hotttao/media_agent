# 2026-10-04

- 明确逐条claim、30秒lease/5秒publish、单项失败继续和1至256秒退避；本地JSONL须写入并Sync才确认，取消及未知确认保留event ID重投，不依赖观测后端。
