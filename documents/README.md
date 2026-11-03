# Norns 文档

## 协议

- [Norns protocol 正式规范](protocol/protocol.md)
- [统一版本清单](../versions.json)（`protocol` 声明协议规范版本）

正式协议只定义跨实现的实体、状态机、Graph、View、Domain、协议动作和角色协作语义。

## 设计

- [Norns CLI 设计](design/norns.md)
- [nornsd 设计与 HTTP API](design/nornsd.md)
- [Well 设计](design/well.md)

每个模块设计文档分为“原始设计”和“实现设计”两部分。安装、构建、快速使用、数据位置和操作安全须知保存在对应模块的 README。

内部产品、协议及兼容性版本统一引用根目录 `versions.json` 的字段，不在文档中维护另一份数值。已发布产物的实际版本以 CLI 版本输出及 `/api/` 服务信息为准；文档中的外部依赖版本和历史格式说明不属于此清单。
