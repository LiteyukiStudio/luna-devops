-- 目标镜像仓库按项目空间独占：触发构建和保存部署配置时按镜像站与仓库查找其它项目空间的占用。
CREATE INDEX IF NOT EXISTS idx_build_runs_target_registry_repository ON build_runs USING btree (target_registry_id, target_repository);
