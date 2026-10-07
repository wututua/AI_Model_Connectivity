# 原生模型协议

[文档索引](README.md) · [检测参数](monitoring-features.md) · [HTTP API](api.md)

自 beta.6 起支持原生协议。现有 Provider 的 `type` 仍用于名称与图标，**不决定协议**；旧配置继续使用 OpenAI 兼容接口。启用原生协议需要在 Provider 编辑页明确选择「探测协议」，或设置 `probe.protocol`。

## 协议与地址

| 协议 | `probe.protocol` | Base URL 示例 | 文本接口 |
| --- | --- | --- | --- |
| OpenAI 兼容 Chat | 空 / `chat` | `https://api.openai.com/v1` | `/chat/completions` |
| OpenAI Responses | `responses` | `https://api.openai.com/v1` | `/responses` |
| Anthropic Messages | `anthropic` | `https://api.anthropic.com/v1` | `/messages` |
| Gemini generateContent | `gemini` | `https://generativelanguage.googleapis.com/v1beta` | `/models/{id}:generateContent` |

地址需要包含上游要求的版本路径。切换协议不会自动修改已有地址或密钥；修改地址仍须重新填写或明确清除密钥。原生接口与 OpenAI 兼容代理的地址及密钥格式可能不同，不能仅凭模型或 Provider 名称判断。

- Anthropic 使用 `x-api-key` 和 `anthropic-version: 2023-06-01`。
- Gemini 使用 `x-goog-api-key`，不会把密钥写进 URL。
- 两者共用已有出站地址校验、超时和禁止重定向策略。
- Gemini 模型可写裸 ID 或 `models/` 加 ID；发现列表统一返回裸 ID。路径分隔符、查询参数和方法名不能写入 ID。

## 参数与响应

原生协议支持普通文本、SSE 流式探测、自定义用户/系统提示词、输出上限、温度省略、超时和文本断言。

`max_tokens` 在 Anthropic 映射为 `max_tokens`，在 Gemini 映射为 `generationConfig.maxOutputTokens`。两者均拒绝 `max_completion_tokens`。Anthropic 发送温度时仅接受 0–1；Gemini 沿用 0–2。不同模型可能进一步限制参数，预设不代表该模型一定支持。

新建原生预设默认省略温度；Gemini 预设使用 1024 的输出上限，为思考输出保留空间。直接使用 API 而省略上限时仍为 16；需要推理的模型可能因此截断，建议根据模型实际要求调整。上限不是实际消耗承诺。

只有带有效文本且明确正常结束才算成功：

- Anthropic 接受 `end_turn` / `stop_sequence`；流式还要求 `message_start` 和 `message_stop`。
- Gemini 接受 `STOP`；流式读到正常 EOF，并允许最终候选后的独立用量事件。
- 截断、拦截、仅思考内容、错误事件及缺少完成证据均失败；失败响应中明确上报的有效用量仍保存。
- 单个 JSON 响应 / SSE 事件上限 1 MiB，完整流上限 8 MiB；超限失败，不接受部分文本作为成功结果。

工具调用结构和 Embedding 探测仍仅支持非流式 OpenAI 兼容 Chat 配置，不支持原生工具执行、原生向量、图片或音频请求。

## 模型发现与预算

原生发现调用 `/models`，自动遍历 Anthropic `after_id` 或 Gemini `pageToken`。Gemini 仅保留声明支持 `generateContent` 的模型。每页均单独预留一次每日请求预算，包括编辑页的手动同步。

完整发现最多 100 页、10,000 个去重模型、8 MiB 数据，每页上限 1 MiB。原有模型发现超时、确认阈值和后续清单批准继续生效。任一页失败、预算耗尽或分页游标重复时整次失败，不返回部分列表，不用部分结果覆盖已知模型清单。

页面切换协议、地址或凭据会取消未完成的模型同步，防止旧响应更改新配置的已选模型。改变协议会更新连接版本，使当前状态回到未检测，但不删除历史。

## 用量与费用

- Anthropic 输入总量包含常规输入、缓存创建和缓存读取 Token；输出使用上游 `output_tokens`。
- Gemini 输出总量包含候选文本与 `thoughtsTokenCount`；已缓存输入包含在上游输入计数中，不重复相加。
- 缺失、null、负数、溢出或不一致的用量不会作为完整费用证据；不推算未上报的消耗。
- 缓存和特殊工具用量无法由当前两项输入/输出单价准确表示，因此保留 Token 计数，但将费用标为未知。流式缺少最终用量时同样不估价。

费用仅为估算，不能替代供应商账单。回归测试使用隔离假上游，不会自动调用真实付费模型；特定账户、模型版本和代理兼容性仍需部署者核实。

## 实现依据

实现核对供应商维护的 SDK 源码，避免把地区限制页面当作 API 文档：

- [Anthropic Go SDK](https://github.com/anthropics/anthropic-sdk-go/tree/c9ebe447ac92c91748af817c265398e5d81ca49f)：Messages、流事件、模型分页和缓存用量字段。
- [Google Gen AI Go SDK](https://github.com/googleapis/go-genai/tree/3a7595eccddf2530209ac9eb044802c71c246eb0)：generateContent、SSE、模型发现、思考用量与 API Key 请求头。
