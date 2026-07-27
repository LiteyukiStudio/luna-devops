# 账单

## 机器目录

先执行
`luna help catalog category=billing limit=100 output=json interactive=false agent=true`
读取当前 CLI 暴露的账单命令。执行具体工具前，再使用
`luna help command path=<category.tool> output=json interactive=false agent=true`
确认参数、Scope、风险和服务端支持状态。只使用目录实际返回的工具。

## 工作流

1. 明确目标账户、周期、项目空间、应用和部署配置。
2. 按目录能力读取余额、账单、用量、费率或账本，显式限制时间范围和分页。
3. 对照 meter、单位、单价与账本记录解释构建、运行、存储、请求和流量费用。
4. 诊断异常计费时保留 usage、rate、ledger、时间窗口和稳定资源 ID。
5. 充值、补偿、扣费、费率或计费规则变更前读取当前状态，确认后只执行一次并复查。

## 风险与验证

- 不通过 `api request` 查询或写入账单。
- 金额变更必须明确账户、金额、单位、生效时间和影响范围。
- 涉及资金或全局费率的操作按机器 Help 风险处理，不自动确认。
- 历史账单不因项目删除或后续调价而重写。
- 没有账本或余额后置读结果时，不报告金额变更成功。
