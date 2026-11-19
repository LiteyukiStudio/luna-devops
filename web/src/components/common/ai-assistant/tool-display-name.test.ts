import { describe, expect, it } from 'vitest'
import i18next from '@/i18n'
import { toolDisplayName } from './tool-display-name'

const ALL_166_FACADE_OPERATIONS = [
  'admin_adjust_user_wallet',
  'admin_configure_auth_policy',
  'admin_configure_global_build_env',
  'admin_configure_platform_mail',
  'admin_delete_ai_model',
  'admin_delete_git_provider',
  'admin_delete_platform_app_template',
  'admin_delete_platform_notification_resource',
  'admin_execute_data_retention_cleanup',
  'admin_get_global_build_env',
  'admin_get_system_configs',
  'admin_inspect_agent_observability',
  'admin_inspect_billing',
  'admin_install_system_component',
  'admin_list_ai_models',
  'admin_list_git_providers',
  'admin_list_platform_app_templates',
  'admin_list_platform_notifications',
  'admin_list_system_components',
  'admin_list_users_and_auth',
  'admin_preview_data_retention',
  'admin_save_ai_model',
  'admin_save_git_provider',
  'admin_save_global_notification_channel',
  'admin_save_global_notification_rule',
  'admin_save_notification_template',
  'admin_save_oidc_provider',
  'admin_save_platform_app_template',
  'admin_save_user',
  'admin_test_ai_integration',
  'admin_test_or_retry_global_notification',
  'admin_update_billing_rate_rules',
  'admin_update_system_configs',
  'build_bind_repository',
  'build_cancel_run',
  'build_configure_scoped_env',
  'build_configure_target_pipeline',
  'build_delete_git_account',
  'build_delete_registry',
  'build_delete_registry_credential',
  'build_delete_run',
  'build_delete_variable_set',
  'build_get_run',
  'build_get_scoped_env',
  'build_inspect_repository_code',
  'build_list_git_sources',
  'build_list_registries',
  'build_list_repository_bindings',
  'build_list_runs',
  'build_list_templates',
  'build_list_variable_sets',
  'build_register_container_image',
  'build_retry_run',
  'build_save_git_account',
  'build_save_registry',
  'build_save_registry_credential',
  'build_save_variable_set',
  'build_search_git_repositories',
  'build_search_registry_images',
  'build_trigger_run',
  'build_unbind_repository',
  'build_update_repository_binding',
  'delivery_cancel_release',
  'delivery_configure_config_set',
  'delivery_configure_target_config',
  'delivery_configure_target_volumes',
  'delivery_create_application',
  'delivery_create_config_set',
  'delivery_create_target',
  'delivery_delete_application',
  'delivery_delete_config_set',
  'delivery_delete_target',
  'delivery_deploy_target',
  'delivery_get_application',
  'delivery_get_config_set',
  'delivery_get_release',
  'delivery_get_target',
  'delivery_get_template',
  'delivery_install_template',
  'delivery_list_applications',
  'delivery_list_config_sets',
  'delivery_list_releases',
  'delivery_list_templates',
  'delivery_restart_target',
  'delivery_rollback_target',
  'delivery_scale_target',
  'delivery_update_application',
  'delivery_update_target_runtime',
  'network_check_gateway_domain',
  'network_create_gateway_route',
  'network_delete_gateway_route',
  'network_delete_service_binding',
  'network_delete_topology_edge',
  'network_get_topology',
  'network_list_gateway_routes',
  'network_list_service_bindings',
  'network_save_service_binding',
  'network_save_topology_edge',
  'network_update_gateway_route',
  'ops_archive_inbox_message',
  'ops_configure_my_notification_preferences',
  'ops_decide_inbox_action',
  'ops_delete_my_notification_channel',
  'ops_delete_project_notification_channel',
  'ops_delete_project_notification_rule',
  'ops_get_billing_summary',
  'ops_get_my_account_security',
  'ops_get_my_notifications',
  'ops_list_billing_records',
  'ops_list_events',
  'ops_list_inbox',
  'ops_list_project_notifications',
  'ops_mark_inbox_read',
  'ops_revoke_my_credential_or_grant',
  'ops_save_my_notification_channel',
  'ops_save_project_notification_channel',
  'ops_save_project_notification_rule',
  'ops_search_global',
  'ops_test_or_retry_my_notification',
  'ops_test_or_retry_project_notification',
  'ops_unbind_project_notification_channel',
  'ops_update_my_profile',
  'ops_web_fetch',
  'ops_web_search',
  'runtime_approve_cluster_network_share',
  'runtime_cancel_volume_transfer',
  'runtime_create_volume',
  'runtime_create_volume_export',
  'runtime_delete_cluster',
  'runtime_delete_cluster_egress_exception',
  'runtime_delete_cluster_k8s_resource',
  'runtime_delete_hook',
  'runtime_delete_volume',
  'runtime_exec_command',
  'runtime_get_pod_logs',
  'runtime_get_target_diagnostics',
  'runtime_get_volume',
  'runtime_inspect_cluster_governance',
  'runtime_list_clusters',
  'runtime_list_hook_runs',
  'runtime_list_hooks',
  'runtime_list_target_pods_and_events',
  'runtime_list_volumes',
  'runtime_retry_volume_operation',
  'runtime_save_cluster',
  'runtime_save_cluster_egress_exception',
  'runtime_save_hook',
  'runtime_test_cluster',
  'runtime_update_volume',
  'workspace_accept_network_share',
  'workspace_configure_network_policy',
  'workspace_create_network_share',
  'workspace_create_project',
  'workspace_delete_network_share',
  'workspace_delete_project',
  'workspace_get_network_policy',
  'workspace_get_project',
  'workspace_list_members',
  'workspace_list_network_shares',
  'workspace_list_projects',
  'workspace_pin_project',
  'workspace_remove_member',
  'workspace_reorder_my_dashboard_projects',
  'workspace_request_billing_transfer',
  'workspace_save_member',
  'workspace_update_project',
] as const

describe('ai tool display names', () => {
  it('localizes common platform and internal tools without exposing operation ids', async () => {
    await i18next.changeLanguage('zh-CN')

    expect(toolDisplayName(i18next.t, 'listApplications')).toBe('查询应用列表')
    expect(toolDisplayName(i18next.t, 'createRelease')).toBe('创建发布')
    expect(toolDisplayName(i18next.t, 'fetchWebPage')).toBe('读取网页')
    expect(toolDisplayName(i18next.t, 'present_card')).toBe('展示信息卡片')
    expect(toolDisplayName(i18next.t, 'request_input')).toBe('请求结构化输入')
    expect(toolDisplayName(i18next.t, 'request_choice')).toBe('请求选择')
    expect(toolDisplayName(i18next.t, 'load_skill')).toBe('加载领域技能')
    expect(toolDisplayName(i18next.t, 'delivery_deploy_target')).toBe('发布部署配置')
    expect(toolDisplayName(i18next.t, 'delivery_scale_target')).toBe('扩缩部署配置')
    expect(toolDisplayName(i18next.t, 'runtime_get_target_diagnostics')).toBe('查看部署诊断快照')
    expect(toolDisplayName(i18next.t, 'ops_web_search')).toBe('搜索互联网')
    expect(toolDisplayName(i18next.t, 'admin_execute_data_retention_cleanup')).toBe('清理数据保留策略')
  })

  it('resolves all 166 exported AI domain facade tools across all 5 locales without falling back to platformOperation', async () => {
    expect(ALL_166_FACADE_OPERATIONS).toHaveLength(166)
    for (const locale of ['zh-CN', 'zh-TW', 'en-US', 'ja-JP', 'ko-KR'] as const) {
      await i18next.changeLanguage(locale)
      const fallback = i18next.t('aiAssistant.tools.platformOperation')
      const unmapped = ALL_166_FACADE_OPERATIONS.filter((opId) => {
        const label = toolDisplayName(i18next.t, opId)
        return !label || label === fallback || label.includes('aiAssistant.tools.')
      })
      expect(unmapped, `unmapped facade tools in ${locale}`).toEqual([])
    }
  })

  it('uses a localized safe fallback for newly registered operations', async () => {
    await i18next.changeLanguage('zh-CN')
    expect(toolDisplayName(i18next.t, 'futureUnknownOperation')).toBe('平台操作')

    await i18next.changeLanguage('en-US')
    expect(toolDisplayName(i18next.t, 'futureUnknownOperation')).toBe('Platform operation')
  })
})
