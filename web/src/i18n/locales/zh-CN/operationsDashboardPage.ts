const operationsDashboardPage = {
  description: '查看管理员配置的 Grafana 运营大盘。',
  configure: '配置运营面板',
  emptyTitle: '还没有配置运营面板',
  emptyDescription: '在全局设置中填写 Grafana dashboard 或 panel 的嵌入地址后，这里会展示运营大盘。',
  invalidTitle: '运营面板地址无效',
  invalidDescription: '请在全局设置中填写 http 或 https 开头的 Grafana iframe 地址。',
  loadFailedTitle: '运营面板加载失败',
  loadFailedDescription: '请确认当前账号具有平台管理员权限，或稍后重试。',
}

export default operationsDashboardPage
