# xhs_data domain 首版建立

- v1 只实现小红书（xhs）数据查询：QueryXhsNotes / GetXhsNote / QueryXhsComments。
- DTO 直接对齐 xhs_note / xhs_note_comment 真实字段。
- 处理 varchar 排序（CAST AS UNSIGNED）与保留字（`desc`/`type` 反引号）。
- 各平台独立 domain（如后续 `dy_data`），互不依赖，加新平台不动现有平台。
# 2026-08-06

- xhs_data 降为内部只读平台适配器；业务查询必须先通过 Crawl Result 的 Workspace 映射授权。
