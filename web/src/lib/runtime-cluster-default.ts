import type { RuntimeCluster } from '@/api'

/**
 * 尚未保存的部署配置选择“平台默认集群”时将落到的集群，仅用于编辑器展示与预填。
 * 规则与后端 runtimecluster.ResolveForTarget 一致：已标记默认的全局集群优先，
 * 否则取最早创建的全局集群；项目空间或个人集群从不作为隐式默认。
 * 已保存的部署配置必须使用后端返回的 effectiveClusterId，不得调用本函数推断。
 */
export function platformDefaultRuntimeCluster(clusters: RuntimeCluster[]) {
  const globalClusters = clusters.filter(cluster => cluster.scope === 'global' && (!cluster.deleteStatus || cluster.deleteStatus === 'active'))
  return globalClusters.find(cluster => cluster.isDefault)
    ?? [...globalClusters].sort((left, right) => left.createdAt.localeCompare(right.createdAt) || left.id.localeCompare(right.id))[0]
}
