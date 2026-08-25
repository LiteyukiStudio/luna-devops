export const cardToolOperationIds = new Set<string>([
  "present_card",
  "request_input",
  "request_choice",
])

/** 模型协议工具不代表平台业务领域，不能参与工作流 reference 选择。 */
export const internalToolOperationIds = new Set<string>([
  "rename_conversation",
  "navigate_to_route",
  "get_tool_details",
  "search_tools",
  ...cardToolOperationIds,
])
