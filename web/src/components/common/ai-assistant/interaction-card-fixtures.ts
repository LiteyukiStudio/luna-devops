import type { InteractionCardGroup } from './interaction-card-schema'

const sourceRefs: NonNullable<InteractionCardGroup['cards'][number]['sourceRefs']> = [{
  type: 'platform_resource',
  refId: 'platform-result',
  label: 'Luna DevOps 实时数据',
  trust: 'platform',
}]

export const interactionCardTemplateFixtures = {
  catalog: {
    schemaVersion: 1,
    generationId: 'catalog-fixture',
    title: '选择可用的镜像站',
    description: '根据当前项目空间权限与连接状态整理。',
    mode: 'interactive',
    template: 'catalog',
    cards: [
      {
        id: 'registry-harbor',
        presentation: {
          variant: 'resource',
          title: '生产 Harbor',
          subtitle: 'harbor.internal.example',
          description: '项目默认镜像站，支持镜像扫描与保留策略。',
          icon: { type: 'category', name: 'registry', alt: '镜像站' },
          badges: [{ label: '健康', tone: 'success' }],
        },
        sourceRefs,
        blocks: [{
          id: 'registry-facts',
          type: 'key_value',
          items: [
            { label: '类型', value: 'Harbor' },
            { label: '可见范围', value: '当前项目空间' },
          ],
        }],
        actions: [{
          id: 'choose-harbor',
          type: 'send_message',
          label: '选择生产 Harbor',
          message: '使用镜像站 registry-harbor 继续配置。',
          emphasis: 'primary',
        }],
      },
      {
        id: 'registry-dockerhub',
        presentation: {
          variant: 'resource',
          title: 'Docker Hub',
          subtitle: 'docker.io',
          description: '公共镜像来源，当前未配置推送凭据。',
          icon: { type: 'category', name: 'registry', alt: '镜像站' },
          badges: [{ label: '只读', tone: 'warning' }],
        },
        sourceRefs,
        actions: [{
          id: 'choose-dockerhub',
          type: 'send_message',
          label: '选择 Docker Hub',
          message: '使用镜像站 registry-dockerhub 继续配置。',
        }],
      },
    ],
  },
  comparison: {
    schemaVersion: 1,
    generationId: 'comparison-fixture',
    title: '发布方案对比',
    description: '对比停机时间、风险、耗时和回滚路径。',
    mode: 'interactive',
    template: 'comparison',
    cards: [{
      id: 'release-comparison',
      presentation: {
        variant: 'summary',
        title: '滚动发布与重新创建',
        subtitle: '同一应用、同一镜像版本',
        badges: [{ label: '滚动发布风险较低', tone: 'success' }],
      },
      sourceRefs,
      blocks: [
        {
          id: 'release-metrics',
          type: 'metrics',
          items: [
            { label: '预计停机', value: '0 秒', change: '保持可用', trend: 'flat', tone: 'success' },
            { label: '预计耗时', value: '3–5 分钟', change: '增加约 1 分钟', trend: 'up', tone: 'neutral' },
          ],
        },
        {
          id: 'release-table',
          type: 'data_table',
          columns: [
            { key: 'strategy', label: '方案' },
            { key: 'downtime', label: '停机' },
            { key: 'risk', label: '风险' },
            { key: 'rollback', label: '回滚方式' },
            { key: 'notes', label: '适用条件' },
          ],
          rows: [
            { id: 'rolling', cells: { strategy: '滚动发布', downtime: '无', risk: '低', rollback: '回滚到上一 Release', notes: '至少两个可用副本，并且 Readiness Probe 正常' } },
            { id: 'recreate', cells: { strategy: '重新创建', downtime: '有', risk: '中', rollback: '重新部署上一镜像', notes: '单副本或不允许两个版本并行运行' } },
          ],
        },
      ],
      actions: [{ id: 'choose-rolling', type: 'send_message', label: '选择滚动发布', message: '使用滚动发布方案继续。', emphasis: 'primary' }],
    }],
  },
  inspector: {
    schemaVersion: 1,
    generationId: 'inspector-fixture',
    title: '应用运行状态',
    mode: 'presentation',
    template: 'inspector',
    cards: [{
      id: 'application-api',
      presentation: {
        variant: 'resource',
        title: 'luna-api',
        subtitle: 'production · app_luna_api',
        description: '当前 Release 与集群工作负载状态。',
        icon: { type: 'category', name: 'application', alt: '应用' },
        badges: [{ label: '运行中', tone: 'success' }],
      },
      sourceRefs,
      blocks: [
        {
          id: 'application-facts',
          type: 'key_value',
          items: [
            { label: '当前版本', value: 'sha-8a3b7f2', format: 'code', copyable: true },
            { label: '副本', value: '3 / 3', format: 'status' },
            { label: '更新时间', value: '2026-07-31 00:20', format: 'date_time' },
          ],
        },
        {
          id: 'application-relations',
          type: 'relations',
          nodes: [
            { id: 'app', label: 'luna-api', category: 'application', status: 'success' },
            { id: 'cluster', label: 'production-k3s', category: 'cluster', status: 'success' },
            { id: 'route', label: 'api.example.com', category: 'gateway', status: 'success' },
          ],
          edges: [
            { source: 'app', target: 'cluster', label: '部署到' },
            { source: 'route', target: 'app', label: '路由到' },
          ],
        },
        {
          id: 'application-links',
          type: 'resource_links',
          links: [{ label: '打开项目空间', routeName: 'projects' }],
        },
      ],
    }],
  },
  form: {
    schemaVersion: 1,
    generationId: 'form-fixture',
    title: '创建访问入口',
    description: '补充域名、端口和 HTTPS 设置。',
    mode: 'interactive',
    template: 'form',
    cards: [{
      id: 'gateway-form',
      presentation: {
        variant: 'form',
        title: 'HTTP 访问入口',
        subtitle: 'luna-api · production',
        icon: { type: 'category', name: 'gateway', alt: '网关' },
      },
      blocks: [{ id: 'gateway-hint', type: 'callout', tone: 'neutral', content: '域名必须已解析到当前集群入口。' }],
      form: {
        sections: [{
          id: 'gateway-basic',
          title: '基础配置',
          fields: [
            { id: 'hostname', type: 'text', label: '域名', required: true, format: 'hostname', placeholder: 'api.example.com', minLength: 3, maxLength: 253 },
            { id: 'port', type: 'number', label: '容器端口', required: true, integer: true, min: 1, max: 65535, defaultValue: 8080 },
            { id: 'https', type: 'boolean', label: '启用 HTTPS', defaultValue: true },
          ],
        }],
      },
      actions: [{
        id: 'continue-gateway',
        type: 'send_message',
        label: '检查访问入口配置',
        message: '检查域名 {{hostname}}、端口 {{port}} 和 HTTPS={{https}} 的访问入口配置。',
        emphasis: 'primary',
      }],
    }],
  },
  wizard: {
    schemaVersion: 1,
    generationId: 'wizard-fixture',
    title: '绑定代码仓库',
    description: '先选择代码源，再按选择补充分支和构建目录。',
    mode: 'interactive',
    template: 'wizard',
    cards: [{
      id: 'repository-wizard',
      presentation: {
        variant: 'form',
        title: '代码仓库与构建入口',
        subtitle: '步骤 1 / 2',
        icon: { type: 'category', name: 'repository', alt: '代码仓库' },
      },
      form: {
        sections: [
          {
            id: 'repository-source',
            title: '代码来源',
            fields: [{
              id: 'provider',
              type: 'select',
              label: '代码源',
              required: true,
              display: 'segmented',
              options: [
                { value: 'github', label: 'GitHub' },
                { value: 'gitea', label: 'Gitea' },
              ],
            }],
          },
          {
            id: 'repository-build',
            title: '构建入口',
            fields: [
              { id: 'branch', type: 'text', label: '分支', required: true, defaultValue: 'main', visibleWhen: { fieldId: 'provider', operator: 'is_not_empty' } },
              { id: 'context', type: 'text', label: '构建目录', required: true, defaultValue: '.', visibleWhen: { fieldId: 'provider', operator: 'is_not_empty' } },
            ],
          },
        ],
      },
      actions: [{
        id: 'continue-repository',
        type: 'send_message',
        label: '继续选择仓库',
        message: '使用 {{provider}}，分支 {{branch}}，构建目录 {{context}}，继续选择仓库。',
        emphasis: 'primary',
      }],
    }],
  },
  diagnosis: {
    schemaVersion: 1,
    generationId: 'diagnosis-fixture',
    title: '构建失败诊断',
    description: '结论来自最近一次 BuildRun 和受控日志摘要。',
    mode: 'presentation',
    template: 'diagnosis',
    cards: [{
      id: 'build-diagnosis',
      presentation: {
        variant: 'finding',
        title: '镜像站认证失败',
        subtitle: 'BuildRun bldr_01 · push 阶段',
        icon: { type: 'category', name: 'build', alt: '构建' },
        badges: [{ label: '阻断构建', tone: 'error' }],
      },
      sourceRefs,
      blocks: [
        { id: 'diagnosis-summary', type: 'callout', tone: 'error', content: 'Builder 无法使用当前凭据向目标镜像站推送镜像。' },
        {
          id: 'diagnosis-evidence',
          type: 'status_list',
          title: '检查项',
          items: [
            { id: 'compile', label: '镜像构建完成', status: 'success' },
            { id: 'credential', label: '推送凭据被拒绝', detail: 'Registry 返回 401 Unauthorized。', status: 'error' },
            { id: 'network', label: '镜像站网络可达', status: 'success' },
          ],
        },
        { id: 'diagnosis-log', type: 'code', title: '关键日志', language: 'text', content: 'failed to push: unexpected status 401 Unauthorized', collapsible: true },
      ],
      actions: [{ id: 'inspect-registry', type: 'send_message', label: '继续检查镜像站凭据', message: '继续检查 BuildRun bldr_01 使用的镜像站凭据。', emphasis: 'primary' }],
    }],
  },
  plan: {
    schemaVersion: 1,
    generationId: 'plan-fixture',
    title: '生产发布计划',
    description: '执行前确认步骤、风险和验证点。',
    mode: 'interactive',
    template: 'plan',
    cards: [{
      id: 'release-plan',
      presentation: {
        variant: 'plan',
        title: '发布 luna-api:v2.4.0',
        subtitle: 'production-k3s · 3 个副本',
        icon: { type: 'category', name: 'deployment', alt: '发布' },
        badges: [{ label: '需要确认', tone: 'warning' }],
      },
      blocks: [
        {
          id: 'release-steps',
          type: 'timeline',
          items: [
            { id: 'precheck', title: '检查集群与镜像', detail: '确认目标镜像存在且集群健康。', status: 'pending' },
            { id: 'deploy', title: '创建 Release 并滚动更新', detail: '逐个替换 3 个副本。', status: 'pending' },
            { id: 'verify', title: '验证副本、入口与错误率', detail: '失败时停止并保留上一版本。', status: 'pending' },
          ],
        },
        { id: 'release-risk', type: 'callout', tone: 'warning', content: '这是生产环境发布，平台将在执行前要求二次确认。' },
      ],
      actions: [{ id: 'review-plan', type: 'send_message', label: '检查发布前条件', message: '按此计划检查生产发布前条件。', emphasis: 'primary' }],
    }],
  },
  progress: {
    schemaVersion: 1,
    generationId: 'progress-fixture',
    title: '正在发布应用',
    mode: 'presentation',
    template: 'progress',
    cards: [{
      id: 'release-progress',
      presentation: {
        variant: 'task',
        title: '发布 luna-api:v2.4.0',
        subtitle: 'Release rel_01',
        icon: { type: 'category', name: 'deployment', alt: '发布' },
        badges: [{ label: '进行中', tone: 'neutral' }],
      },
      blocks: [
        { id: 'release-percent', type: 'progress', mode: 'determinate', value: 67, label: '滚动更新', detail: '2 / 3 个副本已就绪' },
        {
          id: 'release-status',
          type: 'status_list',
          items: [
            { id: 'release-created', label: 'Release 已创建', status: 'success' },
            { id: 'workload-updated', label: '工作负载正在更新', status: 'running' },
            { id: 'route-check', label: '等待访问入口检查', status: 'pending' },
          ],
        },
      ],
      actions: [{ id: 'refresh-release', type: 'send_message', label: '刷新发布状态', message: '刷新 Release rel_01 的状态。', repeatable: true }],
    }],
  },
  result: {
    schemaVersion: 1,
    generationId: 'result-fixture',
    title: '发布完成',
    mode: 'presentation',
    template: 'result',
    cards: [{
      id: 'release-result',
      presentation: {
        variant: 'receipt',
        title: 'luna-api:v2.4.0 已发布',
        subtitle: 'Release rel_01',
        icon: { type: 'category', name: 'deployment', alt: '发布' },
        badges: [{ label: '成功', tone: 'success' }],
      },
      sourceRefs,
      blocks: [
        { id: 'result-summary', type: 'callout', tone: 'success', content: '3 个副本均已就绪，访问入口检查通过。' },
        {
          id: 'result-facts',
          type: 'key_value',
          items: [
            { label: 'Release', value: 'rel_01', format: 'code', copyable: true },
            { label: '镜像', value: 'registry.example/luna-api:v2.4.0', format: 'code' },
            { label: '耗时', value: '3 分 18 秒', format: 'duration' },
          ],
        },
        { id: 'result-links', type: 'resource_links', links: [{ label: '查看事件', routeName: 'events' }] },
      ],
    }],
  },
  dashboard: {
    schemaVersion: 1,
    generationId: 'dashboard-fixture',
    title: '项目空间健康概览',
    description: '最近 24 小时的构建、发布与访问入口状态。',
    mode: 'presentation',
    template: 'dashboard',
    cards: [{
      id: 'project-dashboard',
      presentation: {
        variant: 'summary',
        title: '轻雪项目空间 v2',
        subtitle: '4 个应用 · production',
        icon: { type: 'category', name: 'observability', alt: '概览' },
      },
      sourceRefs,
      blocks: [
        {
          id: 'dashboard-metrics',
          type: 'metrics',
          items: [
            { label: '构建成功率', value: '96.8%', change: '+2.1%', trend: 'up', tone: 'success' },
            { label: '健康副本', value: '11 / 12', change: '1 个待恢复', trend: 'down', tone: 'warning' },
            { label: '发布中', value: '1', change: '预计 2 分钟', trend: 'flat', tone: 'neutral' },
            { label: '入口异常', value: '0', change: '无变化', trend: 'flat', tone: 'success' },
          ],
        },
        {
          id: 'dashboard-chart',
          type: 'chart',
          title: '请求量',
          chartType: 'bar',
          xAxis: ['00:00', '04:00', '08:00', '12:00', '16:00', '20:00'],
          series: [{ name: '请求', values: [120, 80, 260, 410, 380, 290], unit: 'req/min' }],
        },
        {
          id: 'dashboard-status',
          type: 'status_list',
          items: [
            { id: 'api-health', label: 'luna-api 有 1 个副本未就绪', status: 'warning' },
            { id: 'gateway-health', label: '所有访问入口正常', status: 'success' },
          ],
        },
      ],
      actions: [{ id: 'inspect-unhealthy', type: 'send_message', label: '检查未就绪副本', message: '检查轻雪项目空间 v2 中未就绪的副本。', emphasis: 'primary' }],
    }],
  },
} satisfies Record<InteractionCardGroup['template'], InteractionCardGroup>

const extremeTitle = `极长资源名称-${'unbroken_'.repeat(10)}`
const extremeDescription = `极长内容-${'unbroken_'.repeat(36)}`

export const extremeInteractionCardFixture: InteractionCardGroup = {
  schemaVersion: 1,
  generationId: 'extreme-fixture',
  title: extremeTitle,
  description: `包含最大候选数、长文本、混合状态和宽内容。${extremeDescription}`,
  mode: 'presentation',
  template: 'catalog',
  display: { density: 'compact' },
  cards: Array.from({ length: 12 }, (_, index) => ({
    id: `extreme-${index + 1}`,
    presentation: {
      variant: index % 3 === 0 ? 'finding' : 'resource',
      title: `${index + 1}. ${extremeTitle}`,
      subtitle: `极端候选 ${index + 1} · ${'id_'.repeat(24)}`,
      description: `第 ${index + 1} 个极端候选。${extremeDescription}`,
      badges: [
        { label: index % 3 === 0 ? '异常' : '可用', tone: index % 3 === 0 ? 'error' : 'success' },
        { label: '极长内容', tone: 'warning' },
      ],
    },
    sourceRefs,
    blocks: [{
      id: `facts-${index + 1}`,
      type: 'key_value',
      items: [{ label: '不可断标识符', value: extremeDescription, format: 'code', copyable: true }],
    }],
    actions: [{ id: `choose-${index + 1}`, type: 'send_message', label: `选择第 ${index + 1} 个候选`, message: `选择 extreme-${index + 1}。` }],
  })),
}

export const templateSelectionInteractionCardFixture: InteractionCardGroup = {
  schemaVersion: 1,
  generationId: 'template-selection-fixture',
  title: '选择要部署的应用模板',
  description: '候选较多时使用选择字段，选中后继续配置。',
  mode: 'interactive',
  template: 'form',
  cards: [{
    id: 'template-selection',
    presentation: {
      variant: 'form',
      title: '应用模板',
      subtitle: '轻雪个人项目空间',
      description: '先选择一个模板，再进入该模板的参数配置。',
      icon: { type: 'category', name: 'application', alt: '应用模板' },
      badges: [{ label: '8 个候选', tone: 'neutral' }],
    },
    sourceRefs,
    form: {
      sections: [{
        id: 'template',
        fields: [{
          id: 'templateId',
          type: 'select',
          label: '应用模板',
          description: '候选来自当前应用市场搜索结果。',
          placeholder: '选择一个应用模板',
          required: true,
          options: [
            { value: 'postgresql', label: 'PostgreSQL', description: '关系型数据库' },
            { value: 'mysql', label: 'MySQL', description: '关系型数据库' },
            { value: 'mongodb', label: 'MongoDB', description: '文档数据库' },
            { value: 'redis', label: 'Redis', description: '内存数据存储与缓存' },
            { value: 'valkey', label: 'Valkey', description: 'Redis 兼容缓存' },
            { value: 'rabbitmq', label: 'RabbitMQ', description: '消息代理' },
            { value: 'meilisearch', label: 'Meilisearch', description: '全文搜索引擎' },
            { value: 'grafana', label: 'Grafana', description: '可观测性面板' },
          ],
        }],
      }],
    },
    actions: [{
      id: 'continue-template',
      type: 'send_message',
      label: '继续配置',
      message: '继续配置应用模板 {{templateId}}。',
      emphasis: 'primary',
    }],
  }],
}
