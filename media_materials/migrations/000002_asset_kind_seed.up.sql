-- =====================================================================
-- media_materials — asset_kind seed（SYSTEM 预置类别）
-- =====================================================================
-- SYSTEM 状态：不可删，name/resource_type/media_type/source_table 锁列不可改。
-- 这些是研发固定的核心类别；业务线若要扩展，用 RegisterAssetKind 注册 ENABLED。
-- =====================================================================

BEGIN;

-- MATERIAL 类（curated 原料，落 materials 表）
INSERT INTO media_materials.asset_kinds (name, resource_type, media_type, source_table, description, status) VALUES
    ('scene',          'MATERIAL', 'IMAGE', 'materials', '场景图',                 'SYSTEM'),
    ('pose',           'MATERIAL', 'IMAGE', 'materials', '姿势图',                 'SYSTEM'),
    ('movement',       'MATERIAL', 'VIDEO', 'materials', '动作视频',               'SYSTEM'),
    ('music',          'MATERIAL', 'VIDEO', 'materials', '背景音乐',               'SYSTEM'),
    ('caption',        'MATERIAL', 'TEXT',  'materials', '文案文本',               'SYSTEM'),
    ('makeup',         'MATERIAL', 'IMAGE', 'materials', '妆容图',                 'SYSTEM'),
    ('product_image',  'MATERIAL', 'IMAGE', 'materials', '产品图（绑到 product）',  'SYSTEM'),
    ('product_brief',  'MATERIAL', 'TEXT',  'materials', '产品简介文本',           'SYSTEM'),
    ('illustration',   'MATERIAL', 'IMAGE', 'materials', '插画（国学等）',          'SYSTEM'),
    ('voice',          'MATERIAL', 'VIDEO', 'materials', '配音音频',               'SYSTEM')
ON CONFLICT (name) DO NOTHING;

-- ASSET 类（workflow 产物，落 generated_assets 表）
INSERT INTO media_materials.asset_kinds (name, resource_type, media_type, source_table, description, status) VALUES
    ('model_image',   'ASSET', 'IMAGE', 'generated_assets', '模特定妆图（生成）',   'SYSTEM'),
    ('style_image',   'ASSET', 'IMAGE', 'generated_assets', '定妆图（生成）',       'SYSTEM'),
    ('first_frame',   'ASSET', 'IMAGE', 'generated_assets', '首帧图（生成）',       'SYSTEM'),
    ('ai_video',      'ASSET', 'VIDEO', 'generated_assets', 'AI 视频（生成）',      'SYSTEM'),
    ('final_video',   'ASSET', 'VIDEO', 'generated_assets', '成片视频（生成）',     'SYSTEM')
ON CONFLICT (name) DO NOTHING;

-- PRODUCT / VIRTUAL_IP 类（容器，作 workflow_input 用）
INSERT INTO media_materials.asset_kinds (name, resource_type, media_type, source_table, description, status) VALUES
    ('product',    'PRODUCT',    'IMAGE', 'products',    '业务产品（容器）', 'SYSTEM'),
    ('virtual_ip', 'VIRTUAL_IP', 'IMAGE', 'virtual_ips', '虚拟形象（容器）', 'SYSTEM')
ON CONFLICT (name) DO NOTHING;

COMMIT;
